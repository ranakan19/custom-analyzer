package analyzer

import (
	"context"
	"testing"

	v1 "buf.build/gen/go/k8sgpt-ai/k8sgpt/protocolbuffers/go/schema/v1"
	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/dynamic/fake"
)

func TestAnalyzer_Run_BasicFunctionality(t *testing.T) {
	// Create a fake dynamic client
	scheme := runtime.NewScheme()
	listKinds := map[schema.GroupVersionResource]string{
		applicationSetGVR: "ApplicationSetList",
		applicationGVR:    "ApplicationList",
	}
	client := fake.NewSimpleDynamicClientWithCustomListKinds(scheme, listKinds)

	// Create test ApplicationSet with basic issues
	appSet1 := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "argoproj.io/v1alpha1",
			"kind":       "ApplicationSet",
			"metadata": map[string]interface{}{
				"name":      "test-appset-1",
				"namespace": "default",
			},
			"spec": map[string]interface{}{
				"generators": []interface{}{
					map[string]interface{}{
						"list": map[string]interface{}{
							"elements": []interface{}{
								map[string]interface{}{"cluster": "dev"},
								map[string]interface{}{"cluster": "prod"},
							},
						},
					},
				},
			},
			"status": map[string]interface{}{
				"conditions": []interface{}{
					map[string]interface{}{
						"type":    "ErrorOccurred",
						"status":  "True",
						"message": "Test error message",
					},
				},
			},
		},
	}

	// Create ApplicationSet with no generators
	appSet2 := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "argoproj.io/v1alpha1",
			"kind":       "ApplicationSet",
			"metadata": map[string]interface{}{
				"name":      "test-appset-2",
				"namespace": "default",
			},
			"spec": map[string]interface{}{
				"generators": []interface{}{},
			},
		},
	}

	// Create the test objects in the fake client
	_, err := client.Resource(applicationSetGVR).Namespace("default").Create(context.TODO(), appSet1, metav1.CreateOptions{})
	assert.NoError(t, err)
	_, err = client.Resource(applicationSetGVR).Namespace("default").Create(context.TODO(), appSet2, metav1.CreateOptions{})
	assert.NoError(t, err)

	// Create analyzer with fake client
	analyzer := NewAnalyzer().WithDynamicClient(client)

	// Run the analyzer
	response, err := analyzer.Handler.Run(context.TODO(), &v1.RunRequest{})
	assert.NoError(t, err)
	assert.NotNil(t, response)
	assert.NotNil(t, response.Result)

	// Verify the response
	assert.Equal(t, "applicationset-analyzer", response.Result.Name)
	assert.Contains(t, response.Result.Details, "Found 2 ApplicationSet(s) in the cluster")

	// Check that errors were detected
	foundErrorCondition := false
	foundNoGenerators := false
	for _, err := range response.Result.Error {
		if err.Text == "ApplicationSet default/test-appset-1 has error condition: Test error message" {
			foundErrorCondition = true
		}
		if err.Text == "ApplicationSet default/test-appset-2 has no generators defined" {
			foundNoGenerators = true
		}
	}
	assert.True(t, foundErrorCondition, "Should detect error condition")
	assert.True(t, foundNoGenerators, "Should detect missing generators")
}

func TestAnalyzer_Run_ProgressingState(t *testing.T) {
	scheme := runtime.NewScheme()
	listKinds := map[schema.GroupVersionResource]string{
		applicationSetGVR: "ApplicationSetList",
		applicationGVR:    "ApplicationList",
	}
	client := fake.NewSimpleDynamicClientWithCustomListKinds(scheme, listKinds)

	// ApplicationSet in progressing state
	appSetProgressing := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "argoproj.io/v1alpha1",
			"kind":       "ApplicationSet",
			"metadata": map[string]interface{}{
				"name":      "progressing-appset",
				"namespace": "default",
			},
			"spec": map[string]interface{}{
				"generators": []interface{}{
					map[string]interface{}{
						"git": map[string]interface{}{
							"repoURL": "https://github.com/example/repo",
						},
					},
				},
			},
			"status": map[string]interface{}{
				"conditions": []interface{}{
					map[string]interface{}{
						"type":    "Progressing",
						"status":  "True",
						"message": "ApplicationSet is progressing",
					},
					map[string]interface{}{
						"type":    "ParametersGenerated",
						"status":  "False",
						"message": "Failed to generate parameters",
					},
				},
			},
		},
	}

	_, err := client.Resource(applicationSetGVR).Namespace("default").Create(context.TODO(), appSetProgressing, metav1.CreateOptions{})
	assert.NoError(t, err)

	analyzer := NewAnalyzer().WithDynamicClient(client)
	response, err := analyzer.Handler.Run(context.TODO(), &v1.RunRequest{})

	assert.NoError(t, err)
	assert.NotNil(t, response.Result)

	// Check for progressing state detection
	foundProgressing := false
	foundParameterGeneration := false
	for _, err := range response.Result.Error {
		if err.Text == "ApplicationSet default/progressing-appset is in progressing state: ApplicationSet is progressing" {
			foundProgressing = true
		}
		if err.Text == "ApplicationSet default/progressing-appset failed to generate parameters: Failed to generate parameters" {
			foundParameterGeneration = true
		}
	}
	assert.True(t, foundProgressing, "Should detect progressing state")
	assert.True(t, foundParameterGeneration, "Should detect parameter generation failure")
}

func TestAnalyzer_Run_GeneratorValidation(t *testing.T) {
	scheme := runtime.NewScheme()
	listKinds := map[schema.GroupVersionResource]string{
		applicationSetGVR: "ApplicationSetList",
		applicationGVR:    "ApplicationList",
	}
	client := fake.NewSimpleDynamicClientWithCustomListKinds(scheme, listKinds)

	// ApplicationSet with invalid generators
	appSetBadGenerators := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "argoproj.io/v1alpha1",
			"kind":       "ApplicationSet",
			"metadata": map[string]interface{}{
				"name":      "bad-generators-appset",
				"namespace": "default",
			},
			"spec": map[string]interface{}{
				"generators": []interface{}{
					// Empty generator
					map[string]interface{}{},
					// Git generator with empty repoURL
					map[string]interface{}{
						"git": map[string]interface{}{
							"repoURL": "",
						},
					},
					// List generator with no elements
					map[string]interface{}{
						"list": map[string]interface{}{
							"elements": []interface{}{},
						},
					},
					// Cluster generator with no selector or values
					map[string]interface{}{
						"clusters": map[string]interface{}{},
					},
				},
			},
		},
	}

	_, err := client.Resource(applicationSetGVR).Namespace("default").Create(context.TODO(), appSetBadGenerators, metav1.CreateOptions{})
	assert.NoError(t, err)

	analyzer := NewAnalyzer().WithDynamicClient(client)
	response, err := analyzer.Handler.Run(context.TODO(), &v1.RunRequest{})

	assert.NoError(t, err)
	assert.NotNil(t, response.Result)

	// Check for various generator issues
	foundEmptyGenerator := false
	foundEmptyRepoURL := false
	foundEmptyElements := false
	foundNoSelectorOrValues := false

	for _, err := range response.Result.Error {
		switch err.Text {
		case "ApplicationSet default/bad-generators-appset has empty generator at index 0":
			foundEmptyGenerator = true
		case "ApplicationSet default/bad-generators-appset Git generator at index 1 has empty repoURL":
			foundEmptyRepoURL = true
		case "ApplicationSet default/bad-generators-appset List generator at index 2 has empty elements array":
			foundEmptyElements = true
		case "ApplicationSet default/bad-generators-appset Cluster generator at index 3 has no selector or values":
			foundNoSelectorOrValues = true
		}
	}

	assert.True(t, foundEmptyGenerator, "Should detect empty generator")
	assert.True(t, foundEmptyRepoURL, "Should detect empty repoURL")
	assert.True(t, foundEmptyElements, "Should detect empty elements array")
	assert.True(t, foundNoSelectorOrValues, "Should detect missing selector or values")
}

func TestAnalyzer_Run_GeneratedApplicationsStatus(t *testing.T) {
	scheme := runtime.NewScheme()
	listKinds := map[schema.GroupVersionResource]string{
		applicationSetGVR: "ApplicationSetList",
		applicationGVR:    "ApplicationList",
	}
	client := fake.NewSimpleDynamicClientWithCustomListKinds(scheme, listKinds)

	// ApplicationSet with application status
	appSetWithApps := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "argoproj.io/v1alpha1",
			"kind":       "ApplicationSet",
			"metadata": map[string]interface{}{
				"name":      "appset-with-apps",
				"namespace": "default",
			},
			"spec": map[string]interface{}{
				"generators": []interface{}{
					map[string]interface{}{
						"list": map[string]interface{}{
							"elements": []interface{}{
								map[string]interface{}{"env": "dev"},
							},
						},
					},
				},
			},
			"status": map[string]interface{}{
				"applicationStatus": []interface{}{
					map[string]interface{}{
						"application": "test-app-dev",
						"health":      "Degraded",
						"sync":        "OutOfSync",
						"message":     "Application is unhealthy",
					},
					map[string]interface{}{
						"application": "test-app-prod",
						"health":      "Healthy",
						"sync":        "OutOfSync",
					},
				},
			},
		},
	}

	// Create a generated Application
	generatedApp := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "argoproj.io/v1alpha1",
			"kind":       "Application",
			"metadata": map[string]interface{}{
				"name":      "test-app-dev",
				"namespace": "default",
				"labels": map[string]interface{}{
					"argocd.argoproj.io/application-set-name": "appset-with-apps",
				},
			},
			"status": map[string]interface{}{
				"health": map[string]interface{}{
					"status":  "Degraded",
					"message": "Pod is failing",
				},
				"sync": map[string]interface{}{
					"status": "OutOfSync",
				},
				"operationState": map[string]interface{}{
					"phase":   "Failed",
					"message": "Sync operation failed",
				},
			},
		},
	}

	_, err := client.Resource(applicationSetGVR).Namespace("default").Create(context.TODO(), appSetWithApps, metav1.CreateOptions{})
	assert.NoError(t, err)
	_, err = client.Resource(applicationGVR).Namespace("default").Create(context.TODO(), generatedApp, metav1.CreateOptions{})
	assert.NoError(t, err)

	analyzer := NewAnalyzer().WithDynamicClient(client)
	response, err := analyzer.Handler.Run(context.TODO(), &v1.RunRequest{})

	assert.NoError(t, err)
	assert.NotNil(t, response.Result)

	// Check for application status issues
	foundUnhealthyApp := false
	foundOutOfSyncApp1 := false
	foundOutOfSyncApp2 := false
	foundFailedOperation := false

	for _, err := range response.Result.Error {
		switch {
		case err.Text == "Generated Application test-app-dev is not healthy (status: Degraded): Application is unhealthy":
			foundUnhealthyApp = true
		case err.Text == "Generated Application test-app-dev is not synced (status: OutOfSync)":
			foundOutOfSyncApp1 = true
		case err.Text == "Generated Application test-app-prod is not synced (status: OutOfSync)":
			foundOutOfSyncApp2 = true
		case err.Text == "Application default/test-app-dev has failed operation: Sync operation failed":
			foundFailedOperation = true
		}
	}

	assert.True(t, foundUnhealthyApp, "Should detect unhealthy application")
	assert.True(t, foundOutOfSyncApp1, "Should detect out of sync application (from status)")
	assert.True(t, foundOutOfSyncApp2, "Should detect out of sync application (second app)")
	assert.True(t, foundFailedOperation, "Should detect failed operation")

	// Check that status details are included
	assert.Contains(t, response.Result.Details, "Generated Applications: 2")
	assert.Contains(t, response.Result.Details, "App: test-app-dev (Health: Degraded, Sync: OutOfSync)")
}

func TestAnalyzer_Run_NoApplicationSets(t *testing.T) {
	scheme := runtime.NewScheme()
	listKinds := map[schema.GroupVersionResource]string{
		applicationSetGVR: "ApplicationSetList",
		applicationGVR:    "ApplicationList",
	}
	client := fake.NewSimpleDynamicClientWithCustomListKinds(scheme, listKinds)

	analyzer := NewAnalyzer().WithDynamicClient(client)
	response, err := analyzer.Handler.Run(context.TODO(), &v1.RunRequest{})

	assert.NoError(t, err)
	assert.NotNil(t, response.Result)
	assert.Equal(t, "applicationset-analyzer", response.Result.Name)
	assert.Equal(t, "No ApplicationSets found in the cluster", response.Result.Details)
	assert.Empty(t, response.Result.Error)
}

func TestAnalyzer_Run_HealthyApplicationSet(t *testing.T) {
	scheme := runtime.NewScheme()
	listKinds := map[schema.GroupVersionResource]string{
		applicationSetGVR: "ApplicationSetList",
		applicationGVR:    "ApplicationList",
	}
	client := fake.NewSimpleDynamicClientWithCustomListKinds(scheme, listKinds)

	// Healthy ApplicationSet
	healthyAppSet := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "argoproj.io/v1alpha1",
			"kind":       "ApplicationSet",
			"metadata": map[string]interface{}{
				"name":      "healthy-appset",
				"namespace": "default",
			},
			"spec": map[string]interface{}{
				"generators": []interface{}{
					map[string]interface{}{
						"list": map[string]interface{}{
							"elements": []interface{}{
								map[string]interface{}{"env": "prod"},
							},
						},
					},
				},
			},
			"status": map[string]interface{}{
				"conditions": []interface{}{
					map[string]interface{}{
						"type":    "ResourcesUpToDate",
						"status":  "True",
						"message": "All resources are up to date",
					},
				},
				"applicationStatus": []interface{}{
					map[string]interface{}{
						"application": "healthy-app",
						"health":      "Healthy",
						"sync":        "Synced",
					},
				},
			},
		},
	}

	_, err := client.Resource(applicationSetGVR).Namespace("default").Create(context.TODO(), healthyAppSet, metav1.CreateOptions{})
	assert.NoError(t, err)

	analyzer := NewAnalyzer().WithDynamicClient(client)
	response, err := analyzer.Handler.Run(context.TODO(), &v1.RunRequest{})

	assert.NoError(t, err)
	assert.NotNil(t, response.Result)
	assert.Contains(t, response.Result.Details, "Found 1 ApplicationSet(s) in the cluster")
	assert.Contains(t, response.Result.Details, "ApplicationSet: default/healthy-appset")

	// Should have no errors for healthy ApplicationSet
	assert.Empty(t, response.Result.Error, "Healthy ApplicationSet should have no errors")
}
