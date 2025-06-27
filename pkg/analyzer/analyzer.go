package analyzer

import (
	"context"
	"fmt"
	"strings"

	v1 "github.com/ranakan19/custom-analyzer/proto/schema/v1"
	argov1 "github.com/argoproj/argo-cd/v2/pkg/apis/application/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

type Handler struct {
	v1.CustomAnalyzerServiceServer
	dynamicClient dynamic.Interface
}

type Analyzer struct {
	Handler *Handler
}

// NewAnalyzer creates a new ApplicationSet analyzer
func NewAnalyzer() *Analyzer {
	handler := &Handler{}
	
	// Initialize Kubernetes client
	config, err := rest.InClusterConfig()
	if err != nil {
		// Fallback to kubeconfig if not in cluster
		config, err = clientcmd.BuildConfigFromFlags("", clientcmd.RecommendedHomeFile)
		if err != nil {
			// We'll handle this error in the Run method
		}
	}
	
	if config != nil {
		dynamicClient, err := dynamic.NewForConfig(config)
		if err == nil {
			handler.dynamicClient = dynamicClient
		}
	}
	
	return &Analyzer{
		Handler: handler,
	}
}

// Run implements the analyzer logic for ApplicationSets
func (a *Handler) Run(ctx context.Context, req *v1.RunRequest) (*v1.RunResponse, error) {
	fmt.Println("Running ApplicationSet analyzer")
	
	if a.dynamicClient == nil {
		return &v1.RunResponse{
			Result: &v1.Result{
				Name:    "applicationset-analyzer",
				Details: "Failed to initialize Kubernetes client",
				Error: []*v1.ErrorDetail{
					{
						Text: "Could not connect to Kubernetes cluster. Ensure kubeconfig is properly configured or running in-cluster.",
					},
				},
			},
		}, nil
	}

	// Define the ApplicationSet GVR
	applicationSetGVR := schema.GroupVersionResource{
		Group:    "argoproj.io",
		Version:  "v1alpha1",
		Resource: "applicationsets",
	}

	// List all ApplicationSets in all namespaces
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
		}, nil
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
	for _, item := range applicationSets.Items {
		appSet := &argov1.ApplicationSet{}
		err := runtime.DefaultUnstructuredConverter.FromUnstructured(item.Object, appSet)
		if err != nil {
			errors = append(errors, &v1.ErrorDetail{
				Text: fmt.Sprintf("Failed to convert ApplicationSet %s/%s: %v", item.GetNamespace(), item.GetName(), err),
			})
			continue
		}

		// Analyze the ApplicationSet
		appSetErrors := a.analyzeApplicationSet(ctx, appSet)
		errors = append(errors, appSetErrors...)
		
		// Add details about the ApplicationSet
		details = append(details, fmt.Sprintf("ApplicationSet: %s/%s", appSet.Namespace, appSet.Name))
		if appSet.Status.Conditions != nil {
			for _, condition := range appSet.Status.Conditions {
				details = append(details, fmt.Sprintf("  Condition: %s = %s (%s)", condition.Type, condition.Status, condition.Message))
			}
		}
	}

	result := &v1.Result{
		Name:    "applicationset-analyzer",
		Details: strings.Join(details, "\n"),
		Error:   errors,
	}

	return &v1.RunResponse{Result: result}, nil
}

// analyzeApplicationSet performs detailed analysis of a single ApplicationSet
func (a *Handler) analyzeApplicationSet(ctx context.Context, appSet *argov1.ApplicationSet) []*v1.ErrorDetail {
	var errors []*v1.ErrorDetail

	// Check 1: ApplicationSet in progressing state
	if a.isApplicationSetProgressing(appSet) {
		errors = append(errors, &v1.ErrorDetail{
			Text: fmt.Sprintf("ApplicationSet %s/%s is in progressing state", appSet.Namespace, appSet.Name),
		})
	}

	// Check 2: ApplicationSet has error conditions
	for _, condition := range appSet.Status.Conditions {
		if condition.Type == argov1.ApplicationSetConditionErrorOccurred && condition.Status == metav1.ConditionTrue {
			errors = append(errors, &v1.ErrorDetail{
				Text: fmt.Sprintf("ApplicationSet %s/%s has error condition: %s", appSet.Namespace, appSet.Name, condition.Message),
			})
		}
		if condition.Type == argov1.ApplicationSetConditionParametersGenerated && condition.Status == metav1.ConditionFalse {
			errors = append(errors, &v1.ErrorDetail{
				Text: fmt.Sprintf("ApplicationSet %s/%s failed to generate parameters: %s", appSet.Namespace, appSet.Name, condition.Message),
			})
		}
		if condition.Type == argov1.ApplicationSetConditionResourcesUpToDate && condition.Status == metav1.ConditionFalse {
			errors = append(errors, &v1.ErrorDetail{
				Text: fmt.Sprintf("ApplicationSet %s/%s resources are not up to date: %s", appSet.Namespace, appSet.Name, condition.Message),
			})
		}
	}

	// Check 3: Generator issues
	generatorErrors := a.analyzeGenerators(appSet)
	errors = append(errors, generatorErrors...)

	// Check 4: Generated Applications status
	generatedAppErrors := a.analyzeGeneratedApplications(ctx, appSet)
	errors = append(errors, generatedAppErrors...)

	return errors
}

// isApplicationSetProgressing checks if the ApplicationSet is in a progressing state
func (a *Handler) isApplicationSetProgressing(appSet *argov1.ApplicationSet) bool {
	for _, condition := range appSet.Status.Conditions {
		if condition.Type == argov1.ApplicationSetConditionProgressing && condition.Status == metav1.ConditionTrue {
			return true
		}
	}
	return false
}

// analyzeGenerators checks for issues in ApplicationSet generators
func (a *Handler) analyzeGenerators(appSet *argov1.ApplicationSet) []*v1.ErrorDetail {
	var errors []*v1.ErrorDetail

	if len(appSet.Spec.Generators) == 0 {
		errors = append(errors, &v1.ErrorDetail{
			Text: fmt.Sprintf("ApplicationSet %s/%s has no generators defined", appSet.Namespace, appSet.Name),
		})
		return errors
	}

	for i, generator := range appSet.Spec.Generators {
		// Check for empty generators
		if a.isGeneratorEmpty(generator) {
			errors = append(errors, &v1.ErrorDetail{
				Text: fmt.Sprintf("ApplicationSet %s/%s has empty generator at index %d", appSet.Namespace, appSet.Name, i),
			})
		}

		// Check Git generator specific issues
		if generator.Git != nil {
			if generator.Git.RepoURL == "" {
				errors = append(errors, &v1.ErrorDetail{
					Text: fmt.Sprintf("ApplicationSet %s/%s Git generator at index %d has empty repoURL", appSet.Namespace, appSet.Name, i),
				})
			}
		}

		// Check Cluster generator specific issues
		if generator.Clusters != nil {
			if generator.Clusters.Selector == nil && len(generator.Clusters.Values) == 0 {
				errors = append(errors, &v1.ErrorDetail{
					Text: fmt.Sprintf("ApplicationSet %s/%s Cluster generator at index %d has no selector or values", appSet.Namespace, appSet.Name, i),
				})
			}
		}

		// Check List generator specific issues
		if generator.List != nil {
			if len(generator.List.Elements) == 0 && generator.List.ElementsYaml == "" {
				errors = append(errors, &v1.ErrorDetail{
					Text: fmt.Sprintf("ApplicationSet %s/%s List generator at index %d has no elements", appSet.Namespace, appSet.Name, i),
				})
			}
		}
	}

	return errors
}

// isGeneratorEmpty checks if a generator is effectively empty
func (a *Handler) isGeneratorEmpty(generator argov1.ApplicationSetGenerator) bool {
	return generator.Git == nil && 
		   generator.Clusters == nil && 
		   generator.List == nil && 
		   generator.Matrix == nil && 
		   generator.Merge == nil &&
		   generator.SCMProvider == nil &&
		   generator.ClusterDecisionResource == nil &&
		   generator.PullRequest == nil
}

// analyzeGeneratedApplications checks the status of applications generated by the ApplicationSet
func (a *Handler) analyzeGeneratedApplications(ctx context.Context, appSet *argov1.ApplicationSet) []*v1.ErrorDetail {
	var errors []*v1.ErrorDetail

	// Define the Application GVR
	applicationGVR := schema.GroupVersionResource{
		Group:    "argoproj.io",
		Version:  "v1alpha1",
		Resource: "applications",
	}

	// List applications in the same namespace as the ApplicationSet
	applications, err := a.dynamicClient.Resource(applicationGVR).Namespace(appSet.Namespace).List(ctx, metav1.ListOptions{
		LabelSelector: fmt.Sprintf("argocd.argoproj.io/application-set-name=%s", appSet.Name),
	})
	
	if err != nil {
		errors = append(errors, &v1.ErrorDetail{
			Text: fmt.Sprintf("Failed to list applications for ApplicationSet %s/%s: %v", appSet.Namespace, appSet.Name, err),
		})
		return errors
	}

	if len(applications.Items) == 0 {
		errors = append(errors, &v1.ErrorDetail{
			Text: fmt.Sprintf("ApplicationSet %s/%s has no generated applications", appSet.Namespace, appSet.Name),
		})
		return errors
	}

	// Analyze each generated application
	for _, item := range applications.Items {
		app := &argov1.Application{}
		err := runtime.DefaultUnstructuredConverter.FromUnstructured(item.Object, app)
		if err != nil {
			errors = append(errors, &v1.ErrorDetail{
				Text: fmt.Sprintf("Failed to convert Application %s/%s: %v", item.GetNamespace(), item.GetName(), err),
			})
			continue
		}

		// Check application health
		if app.Status.Health.Status != argov1.HealthStatusHealthy {
			errors = append(errors, &v1.ErrorDetail{
				Text: fmt.Sprintf("Generated Application %s/%s is not healthy (status: %s): %s", 
					app.Namespace, app.Name, app.Status.Health.Status, app.Status.Health.Message),
			})
		}

		// Check sync status
		if app.Status.Sync.Status != argov1.SyncStatusCodeSynced {
			errors = append(errors, &v1.ErrorDetail{
				Text: fmt.Sprintf("Generated Application %s/%s is not synced (status: %s)", 
					app.Namespace, app.Name, app.Status.Sync.Status),
			})
		}

		// Check for operation errors
		if app.Status.OperationState != nil && app.Status.OperationState.Phase == argov1.OperationFailed {
			errors = append(errors, &v1.ErrorDetail{
				Text: fmt.Sprintf("Generated Application %s/%s has failed operation: %s", 
					app.Namespace, app.Name, app.Status.OperationState.Message),
			})
		}
	}

	return errors
} 