# ApplicationSet Custom Analyzer for K8sGPT

This is a custom analyzer for K8sGPT that analyzes ArgoCD ApplicationSets in your Kubernetes cluster. It identifies issues with ApplicationSets, their generators, and the applications they generate.

## Features

The analyzer performs comprehensive checks on ApplicationSets:

1. **Discovery**: Finds all ApplicationSets in the cluster
2. **Status Analysis**: Checks the overall status of each ApplicationSet
3. **Condition Monitoring**: Identifies ApplicationSets in "progressing" state or with error conditions
4. **Generator Validation**: Analyzes generator configurations for issues
5. **Generated Application Health**: Monitors the health and sync status of applications created by ApplicationSets

## Prerequisites

- K8sGPT CLI installed
- Go 1.22 or higher
- Access to a Kubernetes cluster with ArgoCD and ApplicationSets
- Proper kubeconfig configuration or in-cluster permissions

## Installation and Setup

1. Clone or download this project
2. Install dependencies:
   ```bash
   go mod tidy
   ```

3. Build the analyzer:
   ```bash
   go build -o applicationset-analyzer main.go
   ```

## Usage

### 1. Start the Custom Analyzer

Run the analyzer server:
```bash
go run main.go
```

The server will start on port 8085 and display:
```
Starting ApplicationSet Analyzer!
ApplicationSet Analyzer server listening on :8085
```

### 2. Register with K8sGPT

Add the custom analyzer to K8sGPT:
```bash
k8sgpt custom-analyzer add -n applicationset-analyzer
```

Verify it's registered:
```bash
k8sgpt custom-analyzer list
```

### 3. Run Analysis

Execute the analysis:
```bash
k8sgpt analyze --custom-analysis
```

## What the Analyzer Checks

### ApplicationSet Status
- Overall health and conditions
- Progressing state detection
- Error conditions
- Parameter generation failures
- Resource update status

### Generator Issues
- Empty or misconfigured generators
- Git generator validation (repository URLs)
- Cluster generator validation (selectors and values)
- List generator validation (elements)
- Support for Matrix, Merge, SCMProvider, ClusterDecisionResource, and PullRequest generators

### Generated Applications
- Application health status
- Sync status
- Operation failures
- Resource synchronization issues

## Example Output

```
ApplicationSet Analyzer Results:
Found 2 ApplicationSet(s) in the cluster
ApplicationSet: argocd/guestbook-apps
  Condition: ParametersGenerated = True (Successfully generated parameters)
  Condition: ResourcesUpToDate = False (Applications need update)

Issues Found:
- ApplicationSet argocd/guestbook-apps resources are not up to date: Applications require synchronization
- Generated Application argocd/guestbook-dev is not synced (status: OutOfSync)
- Generated Application argocd/guestbook-prod has failed operation: Sync operation failed
```

## Architecture

The analyzer uses:
- **GRPC**: For communication with K8sGPT
- **Kubernetes Dynamic Client**: For querying ApplicationSets and Applications
- **ArgoCD API Types**: For proper type handling of ArgoCD resources

## Troubleshooting

### Connection Issues
If you see "Could not connect to Kubernetes cluster":
- Ensure your kubeconfig is properly configured
- Verify cluster access with `kubectl cluster-info`
- Check if running in-cluster with proper RBAC permissions

### No ApplicationSets Found
- Verify ArgoCD is installed with ApplicationSet controller
- Check if ApplicationSets exist: `kubectl get applicationsets -A`
- Ensure proper namespace permissions

### Permission Errors
The analyzer needs permissions to:
- List and get ApplicationSets (`argoproj.io/v1alpha1`)
- List and get Applications (`argoproj.io/v1alpha1`)

Example RBAC for in-cluster deployment:
```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: applicationset-analyzer
rules:
- apiGroups: ["argoproj.io"]
  resources: ["applicationsets", "applications"]
  verbs: ["get", "list"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: applicationset-analyzer
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: applicationset-analyzer
subjects:
- kind: ServiceAccount
  name: applicationset-analyzer
  namespace: default
```

## Contributing

Feel free to submit issues and enhancement requests! 