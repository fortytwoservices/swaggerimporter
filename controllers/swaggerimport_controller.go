package controllers

import (
	"context"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/go-logr/logr"
	apimanagementv1beta1 "github.com/upbound/provider-azure/apis/apimanagement/v1beta1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/portforward"
	"k8s.io/client-go/transport/spdy"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

// SwaggerImportReconciler reconciles a SwaggerImport object
type SwaggerImportReconciler struct {
	client.Client
	Scheme    *runtime.Scheme
	Log       logr.Logger
	Clientset *kubernetes.Clientset
	Config    *rest.Config
}

//+kubebuilder:rbac:groups="",resources=pods,verbs=get;list;watch

// get in cluster config for the controller
func (r *SwaggerImportReconciler) getConfig() (*rest.Config, error) {
    config, err := rest.InClusterConfig()
    if err != nil {
        return nil, fmt.Errorf("failed to get in-cluster config: %w", err)
    }
    return config, nil
}

// Reconcile function to reconcile SwaggerImport
func (r *SwaggerImportReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    log := r.Log.WithValues("swaggerimport", req.NamespacedName)

    // fetch the pod that triggered the reconcile
    var pod corev1.Pod
    if err := r.Get(ctx, req.NamespacedName, &pod); err != nil {
        log.Error(err, "Failed to get pod")
        return ctrl.Result{}, client.IgnoreNotFound(err)
    }

    // extract the 'app' label from the pod or skip
    appName, found := pod.Labels["app"]
    if !found {
        log.Info("Pod does not have 'app' label", "podName", pod.Name)
        return ctrl.Result{}, nil
    }

    // fetch API resources that match the extracted 'app' label
    var apis apimanagementv1beta1.APIList
    apiLabelSelector := client.MatchingLabels{"application": appName}
    if err := r.List(ctx, &apis, client.MatchingLabels(apiLabelSelector)); err != nil {
        log.Error(err, "Failed to list API resources", "appName", appName)
        return ctrl.Result{}, err
    }

    if len(apis.Items) == 0 {
        return ctrl.Result{}, nil  // no matches, skip
    }

    // handle each version
    for _, api := range apis.Items {
        log.Info("Processing matching API", "API Name", api.Name, "Label Matched", appName)
        // assuming swagger file available on /swagger/<version>/swagger.json
        err := r.portForwardAndFetch(ctx, pod.Name, pod.Namespace, api.Name, appName)
        if err != nil {
            log.Error(err, "Failed to port-forward and fetch Swagger JSON", "podName", pod.Name, "apiName", api.Name)
            continue  // continue with other APIs if this one fails
        }
    }

    return ctrl.Result{}, nil
}

func (r *SwaggerImportReconciler) getPorts(ctx context.Context, clientset *kubernetes.Clientset, namespace, podName, appName string) ([]int32, error) {
	var ports []int32

	// attempt to fetch the service to find the ports
	svc, err := clientset.CoreV1().Services(namespace).Get(ctx, appName, metav1.GetOptions{})
	if err == nil && len(svc.Spec.Ports) > 0 {
		for _, port := range svc.Spec.Ports {
			if port.Port != 80 { // Skip port 80
				ports = append(ports, port.Port)
			}
		}
	} else {
		// fallback to pod ports if service ports are not found
		pod, err := clientset.CoreV1().Pods(namespace).Get(ctx, podName, metav1.GetOptions{})
		if err != nil {
			return nil, err
		}
		if len(pod.Spec.Containers) > 0 {
			for _, containerPort := range pod.Spec.Containers[0].Ports {
				if containerPort.ContainerPort != 80 { // skip 80 as pf wont work
					ports = append(ports, containerPort.ContainerPort)
				}
			}
		}
	}

	if len(ports) == 0 {
		return nil, fmt.Errorf("no available ports for service: %s", appName)
	}

	return ports, nil
}

func (r *SwaggerImportReconciler) portForwardAndFetch(ctx context.Context, podName, namespace, apiName, appName string) error {
	config, err := r.getConfig()
	if err != nil {
		r.Log.Error(err, "Failed to get configuration")
		return err
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		r.Log.Error(err, "Failed to create clientset")
		return err
	}

	ports, err := r.getPorts(ctx, clientset, namespace, podName, appName)
	if err != nil {
		r.Log.Error(err, "Failed to get ports", "ServiceName", appName)
		return err
	}

	localPort := "10000"
	var success bool // check if successful
	var finalErr error // store finall error if all fails

	for _, remotePort := range ports {
		if r.portForwardAndFetchOnPort(ctx, namespace, podName, apiName, appName, localPort, strconv.Itoa(int(remotePort))) {
			success = true
			break
		}
	}

	if !success {
		r.Log.Error(finalErr, "Failed to port-forward on any ports for service", "ServiceName", appName)
		return fmt.Errorf("failed to port-forward on any ports for service: %s", appName)
	}

	return nil
}

func (r *SwaggerImportReconciler) portForwardAndFetchOnPort(ctx context.Context, namespace, podName, apiName, appName, localPort, remotePortStr string) bool {
	r.Log.Info("Attempting port-forward", "PodName", podName, "APIName", apiName, "LocalPort", localPort, "RemotePort", remotePortStr)

	// setup port-forward on the discoverd port
	stopChan, readyChan, errChan := make(chan struct{}), make(chan struct{}), make(chan error)
	go func() {
		err := r.portForward(namespace, podName, localPort, remotePortStr, stopChan, readyChan, errChan)
		if err != nil {
			errChan <- err
		}
	}()

	select {
	case <-readyChan:
		r.Log.Info("Port-forward successful", "RemotePort", remotePortStr)
		version := fmt.Sprintf("v%s.0", strings.Split(strings.Split(apiName, "-v")[1], ".")[0])
		if err := r.fetchAndSaveSwagger(ctx, localPort, apiName, appName, version); err == nil {
			close(stopChan)
			return true
		}
	case err := <-errChan:
		r.Log.Error(err, "Port-forward error", "RemotePort", remotePortStr)
	case <-time.After(10 * time.Second):
		r.Log.Error(nil, "Port-forward timeout", "RemotePort", remotePortStr)
	}
	close(stopChan)
	return false
}

func (r *SwaggerImportReconciler) fetchAndSaveSwagger(ctx context.Context, localPort, apiName, appName, version string) error {
    swaggerURL := fmt.Sprintf("http://localhost:%s/swagger/%s/swagger.json", localPort, version)
    resp, err := http.Get(swaggerURL)
    if err != nil {
        return err
    }
    defer resp.Body.Close()

    if resp.StatusCode != http.StatusOK {
        r.Log.Info("Swagger version not found or invalid", "version", version)
        return fmt.Errorf("swagger version not found or invalid: %s", version)
    }

    swaggerJSON, err := ioutil.ReadAll(resp.Body)
    if err != nil {
        r.Log.Error(err, "Failed to read swagger.json", "URL", swaggerURL)
        return err
    }

    r.Log.Info("Swagger JSON fetched successfully")

    // convert swaggerJSON to a string
    swaggerJSONString := string(swaggerJSON)

    if err := r.patchAPIResource(ctx, apiName, appName, swaggerJSONString); err != nil {
        r.Log.Error(err, "Failed to patch API resource", "APIName", apiName)
        return err
    }

    return nil
}

func (r *SwaggerImportReconciler) patchAPIResource(ctx context.Context, apiName string, appName string, swaggerJSON string) error {
    api := &apimanagementv1beta1.API{}
    if err := r.Get(ctx, client.ObjectKey{Name: apiName, Namespace: appName}, api); err != nil {
        return err
    }

    contentFormat := "openapi+json"

	// patch swagger into API resource spec.forProvider.import
    importSpec := apimanagementv1beta1.ImportParameters{
        ContentFormat: &contentFormat,
        ContentValue:  &swaggerJSON,
    }

    api.Spec.ForProvider.Import = []apimanagementv1beta1.ImportParameters{importSpec}

    if err := r.Update(ctx, api); err != nil {
        return err
    }

    r.Log.Info("API resource patched successfully", "APIName", apiName)
    return nil
}

func (r *SwaggerImportReconciler) portForward(namespace, podName, localPort, remotePort string, stopChan, readyChan chan struct{}, errChan chan error) error {
    path := fmt.Sprintf("/api/v1/namespaces/%s/pods/%s/portforward", namespace, podName)
    hostIP := strings.TrimLeft(r.Config.Host, "htps:/")
    serverURL := url.URL{Scheme: "https", Path: path, Host: hostIP}

    transport, upgrader, err := spdy.RoundTripperFor(r.Config)
    if err != nil {
        return fmt.Errorf("failed to create roundtripper: %w", err)
    }

    dialer := spdy.NewDialer(upgrader, &http.Client{Transport: transport}, http.MethodPost, &serverURL)

    ports := []string{fmt.Sprintf("%s:%s", localPort, remotePort)}
    pf, err := portforward.New(dialer, ports, stopChan, readyChan, nil, nil)
    if err != nil {
        return fmt.Errorf("failed to create portforward: %w", err)
    }

    go func() {
        err := pf.ForwardPorts()
        if err != nil {
            errChan <- err
        }
    }()

    return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *SwaggerImportReconciler) SetupWithManager(mgr ctrl.Manager) error {
	clientset, err := kubernetes.NewForConfig(mgr.GetConfig())
	if err != nil {
		return err
	}
	r.Clientset = clientset
	r.Config = mgr.GetConfig()

	return ctrl.NewControllerManagedBy(mgr).
		For(&corev1.Pod{}).
		WithEventFilter(predicate.NewPredicateFuncs(func(obj client.Object) bool {
			// Check if the 'app' label is present
			_, found := obj.GetLabels()["app"]
			return found
		})).
		Complete(r)
}
