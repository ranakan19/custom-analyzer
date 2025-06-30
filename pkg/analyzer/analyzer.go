package analyzer

import (
	"context"
	"fmt"
	"strings"

	rpc "buf.build/gen/go/k8sgpt-ai/k8sgpt/grpc/go/schema/v1/schemav1grpc"
	v1 "buf.build/gen/go/k8sgpt-ai/k8sgpt/protocolbuffers/go/schema/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

type Handler struct {
	rpc.CustomAnalyzerServiceServer
	dynamicClient dynamic.Interface
}

type Analyzer struct {
	Handler *Handler
}

// GVRs for ApplicationSet and Application resources
var (
	applicationSetGVR = schema.GroupVersionResource{
		Group:    "argoproj.io",
		Version:  "v1alpha1",
		Resource: "applicationsets",
	}
	applicationGVR = schema.GroupVersionResource{
		Group:    "argoproj.io",
		Version:  "v1alpha1",
		Resource: "applications",
	}
)

// NewAnalyzer creates a new ApplicationSet analyzer
func NewAnalyzer() *Analyzer {
	handler := &Handler{}
	return &Analyzer{
		Handler: handler,
	}
}

// WithDynamicClient sets the dynamic client for testing
func (a *Analyzer) WithDynamicClient(client dynamic.Interface) *Analyzer {
	a.Handler.dynamicClient = client
	return a
}

// initializeClient initializes the Kubernetes client
func (a *Handler) initializeClient() error {
	if a.dynamicClient != nil {
		return nil
	}

	config, err := rest.InClusterConfig()
	if err != nil {
		config, err = clientcmd.BuildConfigFromFlags("", clientcmd.RecommendedHomeFile)
		if err != nil {
			return fmt.Errorf("failed to get kubeconfig: %v", err)
		}
	}

	dynamicClient, err := dynamic.NewForConfig(config)
	if err != nil {
		return fmt.Errorf("failed to create dynamic client: %v", err)
	}

	a.dynamicClient = dynamicClient
	return nil
}

// Run implements the analyzer logic for ApplicationSets
func (a *Handler) Run(ctx context.Context, req *v1.RunRequest) (*v1.RunResponse, error) {
	if err := a.initializeClient(); err != nil {
		return &v1.RunResponse{
			Result: &v1.Result{
				Name:    "applicationset-analyzer",
				Details: "Failed to initialize Kubernetes client",
				Error: []*v1.ErrorDetail{
					{
						Text: fmt.Sprintf("Could not connect to Kubernetes cluster: %v", err),
					},
				},
			},
		}, err
	}

	// List all ApplicationSets across all namespaces
	applicationSets, err := a.dynamicClient.Resource(applicationSetGVR).List(ctx, metav1.ListOptions{})
	if err != nil {
		return &v1.RunResponse{
			Result: &v1.Result{
				Name:    "applicationset-analyzer",
				Details: fmt.Sprintf("Failed to list ApplicationSets: %v", err),
				Error: []*v1.ErrorDetail{
					{
						Text: fmt.Sprintf("Error listing ApplicationSets: %v", err),
					},
				},
			},
		}, err
	}

	if len(applicationSets.Items) == 0 {
		return &v1.RunResponse{
			Result: &v1.Result{
				Name:    "applicationset-analyzer",
				Details: "No ApplicationSets found in the cluster",
				Error:   []*v1.ErrorDetail{},
			},
		}, nil
	}

	var errors []*v1.ErrorDetail
	var details []string

	details = append(details, fmt.Sprintf("Found %d ApplicationSet(s) in the cluster", len(applicationSets.Items)))

	// Analyze each ApplicationSet
	for _, appSet := range applicationSets.Items {
		appSetErrors := a.analyzeApplicationSet(ctx, &appSet)
		errors = append(errors, appSetErrors...)

		// Add basic information about the ApplicationSet
		details = append(details, fmt.Sprintf("ApplicationSet: %s/%s", appSet.GetNamespace(), appSet.GetName()))

		// Get and display status information
		status := a.getApplicationSetStatus(&appSet)
		for _, statusDetail := range status {
			details = append(details, fmt.Sprintf("  %s", statusDetail))
		}
	}

	result := &v1.Result{
		Name:    "applicationset-analyzer",
		Details: strings.Join(details, "\n"),
		Error:   errors,
	}

	return &v1.RunResponse{Result: result}, nil
}
