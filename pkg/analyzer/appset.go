package analyzer

import (
	v1 "buf.build/gen/go/k8sgpt-ai/k8sgpt/protocolbuffers/go/schema/v1"
	"context"
	"fmt"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// analyzeApplication analyzes individual application health and sync status
func (a *Handler) analyzeApplication(app *unstructured.Unstructured) []*v1.ErrorDetail {
	var errors []*v1.ErrorDetail

	// Check health status
	health, found, err := unstructured.NestedFieldNoCopy(app.Object, "status", "health", "status")
	if err == nil && found {
		if healthStr, ok := health.(string); ok && healthStr != "Healthy" {
			healthMessage, _, _ := unstructured.NestedString(app.Object, "status", "health", "message")
			errors = append(errors, &v1.ErrorDetail{
				Text: fmt.Sprintf("Application %s/%s is not healthy (status: %s): %s",
					app.GetNamespace(), app.GetName(), healthStr, healthMessage),
			})
		}
	}

	// Check sync status
	sync, found, err := unstructured.NestedFieldNoCopy(app.Object, "status", "sync", "status")
	if err == nil && found {
		if syncStr, ok := sync.(string); ok && syncStr != "Synced" {
			errors = append(errors, &v1.ErrorDetail{
				Text: fmt.Sprintf("Application %s/%s is not synced (status: %s)",
					app.GetNamespace(), app.GetName(), syncStr),
			})
		}
	}

	// Check for operation failures
	operationPhase, found, err := unstructured.NestedString(app.Object, "status", "operationState", "phase")
	if err == nil && found && operationPhase == "Failed" {
		operationMessage, _, _ := unstructured.NestedString(app.Object, "status", "operationState", "message")
		errors = append(errors, &v1.ErrorDetail{
			Text: fmt.Sprintf("Application %s/%s has failed operation: %s",
				app.GetNamespace(), app.GetName(), operationMessage),
		})
	}

	return errors
}

// getApplicationSetStatus extracts status information from ApplicationSet
func (a *Handler) getApplicationSetStatus(appSet *unstructured.Unstructured) []string {
	var statusDetails []string

	// Check conditions
	conditions, found, err := unstructured.NestedSlice(appSet.Object, "status", "conditions")
	if err == nil && found {
		for _, c := range conditions {
			condition, ok := c.(map[string]interface{})
			if !ok {
				continue
			}
			condType, _ := condition["type"].(string)
			condStatus, _ := condition["status"].(string)
			condMessage, _ := condition["message"].(string)
			statusDetails = append(statusDetails, fmt.Sprintf("Condition: %s = %s (%s)", condType, condStatus, condMessage))
		}
	}

	// Check applicationStatus
	appStatus, found, err := unstructured.NestedSlice(appSet.Object, "status", "applicationStatus")
	if err == nil && found {
		statusDetails = append(statusDetails, fmt.Sprintf("Generated Applications: %d", len(appStatus)))
		for _, app := range appStatus {
			appInfo, ok := app.(map[string]interface{})
			if !ok {
				continue
			}
			appName, _ := appInfo["application"].(string)
			health, _ := appInfo["health"].(string)
			sync, _ := appInfo["sync"].(string)
			if appName != "" {
				statusDetails = append(statusDetails, fmt.Sprintf("  App: %s (Health: %s, Sync: %s)", appName, health, sync))
			}
		}
	}

	return statusDetails
}

// analyzeApplicationSet performs detailed analysis of a single ApplicationSet
func (a *Handler) analyzeApplicationSet(ctx context.Context, appSet *unstructured.Unstructured) []*v1.ErrorDetail {
	var errors []*v1.ErrorDetail

	// Check 1: ApplicationSet conditions
	conditionErrors := a.checkConditions(appSet)
	errors = append(errors, conditionErrors...)

	// Check 2: Progressing state
	progressingErrors := a.checkProgressingState(appSet)
	errors = append(errors, progressingErrors...)

	// Check 3: Generator issues
	generatorErrors := a.analyzeGenerators(appSet)
	errors = append(errors, generatorErrors...)

	// Check 4: Generated applications status
	appErrors := a.analyzeGeneratedApplications(ctx, appSet)
	errors = append(errors, appErrors...)

	return errors
}

// checkConditions analyzes ApplicationSet conditions
func (a *Handler) checkConditions(appSet *unstructured.Unstructured) []*v1.ErrorDetail {
	var errors []*v1.ErrorDetail

	conditions, found, err := unstructured.NestedSlice(appSet.Object, "status", "conditions")
	if err != nil || !found {
		return errors
	}

	for _, c := range conditions {
		condition, ok := c.(map[string]interface{})
		if !ok {
			continue
		}

		condType, _ := condition["type"].(string)
		condStatus, _ := condition["status"].(string)
		condMessage, _ := condition["message"].(string)

		// Check for various error conditions
		switch condType {
		case "ErrorOccurred":
			if condStatus == "True" {
				errors = append(errors, &v1.ErrorDetail{
					Text: fmt.Sprintf("ApplicationSet %s/%s has error condition: %s",
						appSet.GetNamespace(), appSet.GetName(), condMessage),
				})
			}
		case "ParametersGenerated":
			if condStatus == "False" {
				errors = append(errors, &v1.ErrorDetail{
					Text: fmt.Sprintf("ApplicationSet %s/%s failed to generate parameters: %s",
						appSet.GetNamespace(), appSet.GetName(), condMessage),
				})
			}
		case "ResourcesUpToDate":
			if condStatus == "False" {
				errors = append(errors, &v1.ErrorDetail{
					Text: fmt.Sprintf("ApplicationSet %s/%s resources are not up to date: %s",
						appSet.GetNamespace(), appSet.GetName(), condMessage),
				})
			}
		}
	}

	return errors
}

// checkProgressingState checks if ApplicationSet is in progressing state
func (a *Handler) checkProgressingState(appSet *unstructured.Unstructured) []*v1.ErrorDetail {
	var errors []*v1.ErrorDetail

	conditions, found, err := unstructured.NestedSlice(appSet.Object, "status", "conditions")
	if err != nil || !found {
		return errors
	}

	for _, c := range conditions {
		condition, ok := c.(map[string]interface{})
		if !ok {
			continue
		}

		condType, _ := condition["type"].(string)
		condStatus, _ := condition["status"].(string)
		condMessage, _ := condition["message"].(string)

		if condType == "Progressing" && condStatus == "True" {
			errors = append(errors, &v1.ErrorDetail{
				Text: fmt.Sprintf("ApplicationSet %s/%s is in progressing state: %s",
					appSet.GetNamespace(), appSet.GetName(), condMessage),
			})
		}
	}

	return errors
}

// analyzeGenerators checks for issues in ApplicationSet generators
func (a *Handler) analyzeGenerators(appSet *unstructured.Unstructured) []*v1.ErrorDetail {
	var errors []*v1.ErrorDetail

	generators, found, err := unstructured.NestedSlice(appSet.Object, "spec", "generators")
	if err != nil {
		errors = append(errors, &v1.ErrorDetail{
			Text: fmt.Sprintf("ApplicationSet %s/%s has invalid generators configuration: %v",
				appSet.GetNamespace(), appSet.GetName(), err),
		})
		return errors
	}

	if !found || len(generators) == 0 {
		errors = append(errors, &v1.ErrorDetail{
			Text: fmt.Sprintf("ApplicationSet %s/%s has no generators defined",
				appSet.GetNamespace(), appSet.GetName()),
		})
		return errors
	}

	// Check each generator
	for i, gen := range generators {
		generator, ok := gen.(map[string]interface{})
		if !ok {
			errors = append(errors, &v1.ErrorDetail{
				Text: fmt.Sprintf("ApplicationSet %s/%s has invalid generator at index %d",
					appSet.GetNamespace(), appSet.GetName(), i),
			})
			continue
		}

		// Check if generator is empty
		if len(generator) == 0 {
			errors = append(errors, &v1.ErrorDetail{
				Text: fmt.Sprintf("ApplicationSet %s/%s has empty generator at index %d",
					appSet.GetNamespace(), appSet.GetName(), i),
			})
			continue
		}

		// Check specific generator types
		genErrors := a.validateGeneratorType(appSet, generator, i)
		errors = append(errors, genErrors...)
	}

	return errors
}

// validateGeneratorType validates specific generator types
func (a *Handler) validateGeneratorType(appSet *unstructured.Unstructured, generator map[string]interface{}, index int) []*v1.ErrorDetail {
	var errors []*v1.ErrorDetail

	// Check Git generator
	if gitGen, found := generator["git"]; found {
		if gitMap, ok := gitGen.(map[string]interface{}); ok {
			if repoURL, exists := gitMap["repoURL"]; !exists || repoURL == "" {
				errors = append(errors, &v1.ErrorDetail{
					Text: fmt.Sprintf("ApplicationSet %s/%s Git generator at index %d has empty repoURL",
						appSet.GetNamespace(), appSet.GetName(), index),
				})
			}
		}
	}

	// Check List generator
	if listGen, found := generator["list"]; found {
		if listMap, ok := listGen.(map[string]interface{}); ok {
			elements, hasElements := listMap["elements"]
			_, hasElementsYaml := listMap["elementsYaml"]

			if !hasElements && !hasElementsYaml {
				errors = append(errors, &v1.ErrorDetail{
					Text: fmt.Sprintf("ApplicationSet %s/%s List generator at index %d has no elements or elementsYaml",
						appSet.GetNamespace(), appSet.GetName(), index),
				})
			} else if hasElements {
				if elemSlice, ok := elements.([]interface{}); ok && len(elemSlice) == 0 {
					errors = append(errors, &v1.ErrorDetail{
						Text: fmt.Sprintf("ApplicationSet %s/%s List generator at index %d has empty elements array",
							appSet.GetNamespace(), appSet.GetName(), index),
					})
				}
			}
		}
	}

	// Check Cluster generator
	if clusterGen, found := generator["clusters"]; found {
		if clusterMap, ok := clusterGen.(map[string]interface{}); ok {
			_, hasSelector := clusterMap["selector"]
			values, hasValues := clusterMap["values"]

			if !hasSelector && !hasValues {
				errors = append(errors, &v1.ErrorDetail{
					Text: fmt.Sprintf("ApplicationSet %s/%s Cluster generator at index %d has no selector or values",
						appSet.GetNamespace(), appSet.GetName(), index),
				})
			} else if hasValues {
				if valuesMap, ok := values.(map[string]interface{}); ok && len(valuesMap) == 0 {
					errors = append(errors, &v1.ErrorDetail{
						Text: fmt.Sprintf("ApplicationSet %s/%s Cluster generator at index %d has empty values",
							appSet.GetNamespace(), appSet.GetName(), index),
					})
				}
			}
		}
	}

	return errors
}

// analyzeGeneratedApplications checks the status of applications generated by the ApplicationSet
func (a *Handler) analyzeGeneratedApplications(ctx context.Context, appSet *unstructured.Unstructured) []*v1.ErrorDetail {
	var errors []*v1.ErrorDetail

	// First, check the applicationStatus in the ApplicationSet status
	appStatus, found, err := unstructured.NestedSlice(appSet.Object, "status", "applicationStatus")
	if err == nil && found {
		for _, app := range appStatus {
			appInfo, ok := app.(map[string]interface{})
			if !ok {
				continue
			}

			appName, _ := appInfo["application"].(string)
			health, _ := appInfo["health"].(string)
			sync, _ := appInfo["sync"].(string)
			message, _ := appInfo["message"].(string)

			// Check for unhealthy applications
			if health != "" && health != "Healthy" {
				errors = append(errors, &v1.ErrorDetail{
					Text: fmt.Sprintf("Generated Application %s is not healthy (status: %s): %s",
						appName, health, message),
				})
			}

			// Check for unsynced applications
			if sync != "" && sync != "Synced" {
				errors = append(errors, &v1.ErrorDetail{
					Text: fmt.Sprintf("Generated Application %s is not synced (status: %s)",
						appName, sync),
				})
			}
		}
	}

	// Also try to list actual Application resources to get more detailed status
	appLabelSelector := fmt.Sprintf("argocd.argoproj.io/application-set-name=%s", appSet.GetName())
	applications, err := a.dynamicClient.Resource(applicationGVR).Namespace(appSet.GetNamespace()).List(ctx, metav1.ListOptions{
		LabelSelector: appLabelSelector,
	})

	if err != nil {
		// Don't fail if we can't list applications - the applicationStatus check above should be sufficient
		return errors
	}

	if len(applications.Items) == 0 && len(appStatus) == 0 {
		errors = append(errors, &v1.ErrorDetail{
			Text: fmt.Sprintf("ApplicationSet %s/%s has no generated applications",
				appSet.GetNamespace(), appSet.GetName()),
		})
	}

	// Analyze individual applications for more detailed issues
	for _, app := range applications.Items {
		appErrors := a.analyzeApplication(&app)
		errors = append(errors, appErrors...)
	}

	return errors
}
