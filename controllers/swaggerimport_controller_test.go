package controllers

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	namespacedapimanagement "github.com/upbound/provider-azure/v2/apis/namespaced/apimanagement/v1beta1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

func TestControllers(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Controllers Suite")
}

var _ = Describe("SwaggerImportReconciler", func() {
	var (
		reconciler      *SwaggerImportReconciler
		fakeClient      client.Client
		scheme          *runtime.Scheme
		ctx             context.Context
		mockSwaggerJSON string
	)

	BeforeEach(func() {
		ctx = context.Background()
		scheme = runtime.NewScheme()
		_ = corev1.AddToScheme(scheme)
		_ = namespacedapimanagement.AddToScheme(scheme)

		mockSwaggerJSON = `{"swagger": "2.0", "info": {"title": "Mock API", "version": "1.0.0"}}`
	})

	Context("When a Pod with swaggerimporter label exists", func() {
		It("should fetch swagger and update API resource", func() {
			// Setup resources
			podName := "test-pod"
			namespacePod := "services"
			appName := "test-app"
			apiName := "test-app-v1"
			namespaceApi := "services"

			pod := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      podName,
					Namespace: namespacePod,
					Labels: map[string]string{
						"swaggerimporter": "true",
						"app":             appName,
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Ports: []corev1.ContainerPort{
								{ContainerPort: 8080},
							},
						},
					},
				},
			}

			api := &namespacedapimanagement.API{
				ObjectMeta: metav1.ObjectMeta{
					Name:      apiName,
					Namespace: namespaceApi,
					Labels: map[string]string{
						"application": appName,
					},
				},
				Spec: namespacedapimanagement.APISpec{
					ForProvider: namespacedapimanagement.APIParameters{
						// Initialize with empty or default values if needed
					},
				},
			}

			service := &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      appName,
					Namespace: namespacePod,
				},
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{
						{Port: 8080},
					},
				},
			}

			fakeClient = fake.NewClientBuilder().WithScheme(scheme).WithObjects(pod, api, service).Build()

			reconciler = &SwaggerImportReconciler{
				Client: fakeClient,
				Scheme: scheme,
				Log:    zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)),
				HTTPGet: func(url string) (*http.Response, error) {
					return &http.Response{
						StatusCode: http.StatusOK,
						Body:       io.NopCloser(bytes.NewBufferString(mockSwaggerJSON)),
					}, nil
				},
			}

			req := ctrl.Request{
				NamespacedName: types.NamespacedName{
					Name:      podName,
					Namespace: namespacePod,
				},
			}

			_, err := reconciler.Reconcile(ctx, req)
			Expect(err).NotTo(HaveOccurred())

			// Verify API was updated
			updatedAPI := &namespacedapimanagement.API{}
			err = fakeClient.Get(ctx, types.NamespacedName{Name: apiName, Namespace: namespaceApi}, updatedAPI)
			Expect(err).NotTo(HaveOccurred())
			Expect(updatedAPI.Spec.ForProvider.Import).NotTo(BeNil())
			Expect(updatedAPI.Spec.ForProvider.Import.ContentValue).NotTo(BeNil())
			Expect(*updatedAPI.Spec.ForProvider.Import.ContentValue).To(Equal(mockSwaggerJSON))
		})
	})
})
