package controllers

import (
	"context"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"
	"time"

	"github.com/go-logr/logr"
	apimanagementv1beta1 "github.com/upbound/provider-azure/apis/apimanagement/v1beta1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
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

// Reconcile function to reconcile SwaggerImport
func (r *SwaggerImportReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    log := r.Log.WithValues("swaggerimport", req.NamespacedName)

    // fetch the pod that triggered the reconcile
    var pod corev1.Pod
    if err := r.Get(ctx, req.NamespacedName, &pod); err != nil {
        if errors.IsNotFound(err) {
            // pod does not exist anymore
            log.Info("Pod not found, will not requeue", "name", req.NamespacedName)
            return ctrl.Result{}, nil // no requeue
        }
        log.Error(err, "Failed to get pod, requeuing")
        return ctrl.Result{RequeueAfter: 1 * time.Minute}, err // requeue still for other errors
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
        return ctrl.Result{RequeueAfter: 1 * time.Minute}, err
    }

    if len(apis.Items) == 0 {
        return ctrl.Result{RequeueAfter: 1 * time.Minute}, nil
    }

    // handle each version
    for _, api := range apis.Items {
		log.Info("Processing matching API", "API Name", api.Name, "Label Matched", appName)
		version := fmt.Sprintf("v%s.0", strings.Split(strings.Split(api.Name, "-v")[1], ".")[0])
		err := r.fetchAndSaveSwagger(ctx, pod.Namespace, api.Name, appName, version)
		if err != nil {
			log.Error(err, "Failed to fetch Swagger JSON", "apiName", api.Name)
			continue  // continue with other APIs if this one fails
		}
	}

    return ctrl.Result{RequeueAfter: 1 * time.Minute}, nil
}

func (r *SwaggerImportReconciler) getPorts(ctx context.Context, clientset *kubernetes.Clientset, namespace, appName string) ([]int32, error) {
    var ports []int32

    // fetch service based on label
    svc, err := clientset.CoreV1().Services(namespace).Get(ctx, appName, metav1.GetOptions{})
    if err != nil {
        return nil, fmt.Errorf("failed to get service: %s, error: %v", appName, err)
    }

    // add service ports to range
    for _, port := range svc.Spec.Ports {
        ports = append(ports, port.Port)
    }

    // try pod ports if service not available
    if len(ports) == 0 {
        pod, err := clientset.CoreV1().Pods(namespace).Get(ctx, appName, metav1.GetOptions{})
        if err != nil {
            return nil, fmt.Errorf("failed to get pod: %s, error: %v", appName, err)
        }
        for _, container := range pod.Spec.Containers {
            for _, containerPort := range container.Ports {
                ports = append(ports, containerPort.ContainerPort)
            }
        }
    }

    if len(ports) == 0 {
        return nil, fmt.Errorf("no available ports for service: %s", appName)
    }

    return ports, nil
}

func (r *SwaggerImportReconciler) needsUpdate(ctx context.Context, apiName, appName, newSwaggerJSON string) (bool, error) {
    api := &apimanagementv1beta1.API{}
    if err := r.Get(ctx, client.ObjectKey{Name: apiName, Namespace: appName}, api); err != nil {
        return false, err
    }

    // match swagger to imports
    if len(api.Spec.ForProvider.Import) > 0 {
        currentSwaggerJSON := api.Spec.ForProvider.Import[0].ContentValue
        return currentSwaggerJSON != nil && *currentSwaggerJSON != newSwaggerJSON, nil
    }
    return true, nil
}

func (r *SwaggerImportReconciler) fetchAndSaveSwagger(ctx context.Context, namespace, apiName, appName, version string) error {
    ports, err := r.getPorts(ctx, r.Clientset, namespace, appName)
    if err != nil {
        r.Log.Error(err, "Failed to get service ports", "appName", appName)
        return err
    }

    var lastError error
    for _, port := range ports {
        swaggerURL := fmt.Sprintf("http://%s.%s.svc.cluster.local:%d/swagger/%s/swagger.json", appName, namespace, port, version)
        resp, err := http.Get(swaggerURL)
        if err != nil {
            lastError = err
            continue // keep trying ports if one fails
        }
        defer resp.Body.Close()

        if resp.StatusCode == http.StatusOK {
            swaggerJSON, err := ioutil.ReadAll(resp.Body)
            if err != nil {
                lastError = err
                continue
            }

            r.Log.Info("Swagger JSON fetched successfully", "URL", swaggerURL)
            swaggerJSONString := string(swaggerJSON)

            // Check if update is necessary
            needsUpdate, err := r.needsUpdate(ctx, apiName, appName, swaggerJSONString)
            if err != nil {
                r.Log.Error(err, "Error checking if update is needed")
                lastError = err
                continue
            }

            if needsUpdate {
                if err := r.patchAPIResource(ctx, apiName, appName, swaggerJSONString); err != nil {
                    lastError = err
                    continue
                }
            } else {
                r.Log.Info("API is up to date; no update required", "APIName", apiName)
            }

            return nil
        } else {
            lastError = fmt.Errorf("swagger version not found or invalid: %s, HTTP status: %d", version, resp.StatusCode)
        }
    }

    return lastError // return error if all fails
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
			// check if the 'app' label is present
			_, found := obj.GetLabels()["app"]
			return found
		})).
		Complete(r)
}
