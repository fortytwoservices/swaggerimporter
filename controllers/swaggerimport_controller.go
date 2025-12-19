package controllers

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/go-logr/logr"
	clusterapimanagement "github.com/upbound/provider-azure/v2/apis/cluster/apimanagement/v1beta2"
	namespacedapimanagement "github.com/upbound/provider-azure/v2/apis/namespaced/apimanagement/v1beta1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

// SwaggerImportReconciler reconciles a SwaggerImport object
type SwaggerImportReconciler struct {
	client.Client
	Scheme  *runtime.Scheme
	Log     logr.Logger
	HTTPGet func(url string) (*http.Response, error)
}

//+kubebuilder:rbac:groups="",resources=pods,verbs=get;list;watch
//+kubebuilder:rbac:groups="",resources=services,verbs=get;list;watch
//+kubebuilder:rbac:groups=apimanagement.azure.upbound.io,resources=apis,verbs=get;list;watch;update;patch
//+kubebuilder:rbac:groups=apimanagement.azure.m.upbound.io,resources=apis,verbs=get;list;watch;update;patch

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

	apiLabelSelector := client.MatchingLabels{"application": appName}

	// fetch API resources that match the extracted 'app' label
	var apis namespacedapimanagement.APIList
	if err := r.List(ctx, &apis, client.MatchingLabels(apiLabelSelector)); err != nil {
		log.Error(err, "Failed to list API resources", "appName", appName)
		return ctrl.Result{RequeueAfter: 1 * time.Minute}, err
	}

	// fetch clustered api reosurces as well
	var clusterAPIs clusterapimanagement.APIList
	if err := r.List(ctx, &clusterAPIs, client.MatchingLabels(apiLabelSelector), client.InNamespace("")); err != nil {
		log.Error(err, "Failed to list clustered API resources", "appName", appName)
		return ctrl.Result{RequeueAfter: 1 * time.Minute}, err
	}
	// apis.Items = append(apis.Items, clusterAPIs.Items...)

	if len(apis.Items) == 0 && len(clusterAPIs.Items) == 0 {
		return ctrl.Result{RequeueAfter: 1 * time.Minute}, nil
	}

	// handle each version
	for _, api := range apis.Items {
		log.Info("Processing matching API", "API Name", api.Name, "Label Matched", appName)
		version, err := parseVersion(api.Name)
		if err != nil {
			log.Error(err, "Failed to parse version from API name", "apiName", api.Name)
			continue // skip this API if version parsing fails
		}
		err = r.fetchAndSaveSwagger(ctx, pod.Namespace, api.Name, api.Namespace, appName, version)
		if err != nil {
			log.Error(err, "Failed to fetch Swagger JSON", "apiName", api.Name)
			continue // continue with other APIs if this one fails
		}
	}

	// handle each version
	for _, api := range clusterAPIs.Items {
		log.Info("Processing matching API", "API Name", api.Name, "Label Matched", appName)
		version, err := parseVersion(api.Name)
		if err != nil {
			log.Error(err, "Failed to parse version from API name", "apiName", api.Name)
			continue // skip this API if version parsing fails
		}
		err = r.fetchAndSaveSwagger(ctx, pod.Namespace, api.Name, api.Namespace, appName, version)
		if err != nil {
			log.Error(err, "Failed to fetch Swagger JSON", "apiName", api.Name)
			continue // continue with other APIs if this one fails
		}
	}

	return ctrl.Result{RequeueAfter: 1 * time.Minute}, nil
}

// parseVersion extracts version from API name in the format "name-v{major}.{minor}"
// Returns the version string in the format "v{major}.0" or an error if the format is invalid
func parseVersion(apiName string) (string, error) {
	// Check if the name contains "-v"
	parts := strings.Split(apiName, "-v")
	if len(parts) < 2 {
		return "", fmt.Errorf("API name does not contain version marker '-v': %s", apiName)
	}

	// Get the version part (everything after the last "-v")
	versionPart := parts[len(parts)-1]
	
	// Split by "." to get major version
	versionComponents := strings.Split(versionPart, ".")
	if len(versionComponents) == 0 || versionComponents[0] == "" {
		return "", fmt.Errorf("invalid version format in API name: %s", apiName)
	}

	// Return version in the format "v{major}.0"
	return fmt.Sprintf("v%s.0", versionComponents[0]), nil
}

func (r *SwaggerImportReconciler) getPorts(ctx context.Context, namespace, appName string) ([]int32, error) {
	var ports []int32

	// fetch service based on name
	svc := &corev1.Service{}
	err := r.Get(ctx, client.ObjectKey{Name: appName, Namespace: namespace}, svc)
	if err != nil {
		return nil, fmt.Errorf("failed to get service: %s, error: %v", appName, err)
	}

	// add service ports to range
	for _, port := range svc.Spec.Ports {
		ports = append(ports, port.Port)
	}

	// try pod ports if service not available
	if len(ports) == 0 {
		pod := &corev1.Pod{}
		err := r.Get(ctx, client.ObjectKey{Name: appName, Namespace: namespace}, pod)
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

func (r *SwaggerImportReconciler) needsUpdate(ctx context.Context, apiName, namespaceApi, newSwaggerJSON string) (bool, error) {
	if namespaceApi == "" {
		api := &clusterapimanagement.API{}
		if err := r.Get(ctx, client.ObjectKey{Name: apiName}, api); err != nil {
			return false, err
		}

		// match swagger to imports
		if api.Spec.ForProvider.Import != nil {
			currentSwaggerJSON := api.Spec.ForProvider.Import.ContentValue
			return currentSwaggerJSON != nil && *currentSwaggerJSON != newSwaggerJSON, nil
		}
		return true, nil
	}

	api := &namespacedapimanagement.API{}
	if err := r.Get(ctx, client.ObjectKey{Name: apiName, Namespace: namespaceApi}, api); err != nil {
		return false, err
	}

	// match swagger to imports
	if api.Spec.ForProvider.Import != nil {
		currentSwaggerJSON := api.Spec.ForProvider.Import.ContentValue
		return currentSwaggerJSON != nil && *currentSwaggerJSON != newSwaggerJSON, nil
	}
	return true, nil
}

func (r *SwaggerImportReconciler) fetchAndSaveSwagger(ctx context.Context, namespace, apiName, namespaceApi, appName, version string) error {
	ports, err := r.getPorts(ctx, namespace, appName)
	if err != nil {
		r.Log.Error(err, "Failed to get service ports", "appName", appName)
		return err
	}

	var lastError error
	for _, port := range ports {
		swaggerURL := fmt.Sprintf("http://%s.%s.svc.cluster.local:%d/swagger/%s/swagger.json", appName, namespace, port, version)
		resp, err := r.HTTPGet(swaggerURL)
		if err != nil {
			lastError = err
			continue // keep trying ports if one fails
		}
		defer resp.Body.Close()

		if resp.StatusCode == http.StatusOK {
			swaggerJSON, err := io.ReadAll(resp.Body)
			if err != nil {
				lastError = err
				continue
			}

			r.Log.Info("Swagger JSON fetched successfully", "URL", swaggerURL)
			swaggerJSONString := string(swaggerJSON)

			// Check if update is necessary
			needsUpdate, err := r.needsUpdate(ctx, apiName, namespaceApi, swaggerJSONString)
			if err != nil {
				r.Log.Error(err, "Error checking if update is needed")
				lastError = err
				continue
			}

			if needsUpdate {
				if err := r.patchAPIResource(ctx, apiName, namespaceApi, swaggerJSONString); err != nil {
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

func (r *SwaggerImportReconciler) patchAPIResource(ctx context.Context, apiName string, namespaceApi string, swaggerJSON string) error {
	contentFormat := "openapi+json"

	if namespaceApi == "" {
		api := &clusterapimanagement.API{}
		if err := r.Get(ctx, client.ObjectKey{Name: apiName}, api); err != nil {
			return err
		}

		// patch swagger into API resource spec.forProvider.import
		importSpec := clusterapimanagement.ImportParameters{
			ContentFormat: &contentFormat,
			ContentValue:  &swaggerJSON,
		}

		api.Spec.ForProvider.Import = &importSpec

		if err := r.Update(ctx, api); err != nil {
			return err
		}

		r.Log.Info("Cluster API resource patched successfully", "APIName", apiName)
		return nil
	}

	api := &namespacedapimanagement.API{}
	if err := r.Get(ctx, client.ObjectKey{Name: apiName, Namespace: namespaceApi}, api); err != nil {
		return err
	}

	// patch swagger into API resource spec.forProvider.import
	importSpec := namespacedapimanagement.ImportParameters{
		ContentFormat: &contentFormat,
		ContentValue:  &swaggerJSON,
	}

	api.Spec.ForProvider.Import = &importSpec

	if err := r.Update(ctx, api); err != nil {
		return err
	}

	r.Log.Info("API resource patched successfully", "APIName", apiName, "ApiNamespace", namespaceApi)
	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *SwaggerImportReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&corev1.Pod{}).
		WithEventFilter(predicate.NewPredicateFuncs(func(obj client.Object) bool {
			// only applications with label swaggerimporter = true will trigger reconcile
			return obj.GetLabels()["swaggerimporter"] == "true"
		})).
		Complete(r)
}
