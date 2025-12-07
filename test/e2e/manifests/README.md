# E2E Test Manifests

This directory contains Kubernetes manifest files used by the e2e integration tests.

## Files

### mlflow-minimal.yaml

A minimal MLflow custom resource for testing purposes.

**Features:**
- kube-rbac-proxy disabled for direct access
- Single replica
- Minimal resource requests (100m CPU, 256Mi memory)
- SQLite backend storage
- File-based artifact storage with 5Gi PVC
- Debug logging enabled

**Usage:**

This manifest is automatically loaded by the e2e test suite. It can also be used for manual testing:

```bash
kubectl apply -f mlflow-minimal.yaml
```

**When to modify:**
- Update this file when testing new MLflow CR features
- Keep it minimal - only include settings necessary for basic functionality
- Ensure it can run in resource-constrained environments (CI/CD)

### test-pod.yaml.tmpl

Template for the test pod that validates MLflow API functionality.

**Template Variables:**
- `{{ .Namespace }}`: Target namespace for the test pod
- `{{ .TrackingURI }}`: MLflow tracking server URI

**Features:**
- Uses Python 3.11 slim image
- Installs MLflow and requests packages
- Mounts test script from ConfigMap
- Runs test_mlflow_api.py against deployed MLflow

**Usage:**

This template is rendered by the e2e test with dynamic values. For manual use:

```bash
# Example rendered values
sed -e 's/{{ .Namespace }}/opendatahub/g' \
    -e 's|{{ .TrackingURI }}|http://mlflow.opendatahub.svc.cluster.local:5000|g' \
    test-pod.yaml.tmpl > test-pod.yaml

kubectl apply -f test-pod.yaml
```

**When to modify:**
- Update Python version if needed
- Add additional test dependencies
- Modify test execution logic

## How These Files Are Used

The e2e test (`e2e_test.go`) uses these manifests as follows:

1. **Reading MLflow manifest:**
   ```go
   mlflowManifestPath := filepath.Join(projectDir, "test/e2e/manifests/mlflow-minimal.yaml")
   mlflowManifest, err := os.ReadFile(mlflowManifestPath)
   ```

2. **Rendering test pod template:**
   ```go
   testPodTemplatePath := filepath.Join(projectDir, "test/e2e/manifests/test-pod.yaml.tmpl")
   testPodTemplateContent, err := os.ReadFile(testPodTemplatePath)

   tmpl, err := template.New("test-pod").Parse(string(testPodTemplateContent))
   templateData := map[string]string{
       "Namespace":   targetNamespace,
       "TrackingURI": mlflowURL,
   }
   err = tmpl.Execute(&testPodManifest, templateData)
   ```

## Benefits of External Manifests

Using external manifest files instead of hardcoded strings provides:

1. **Maintainability**: Easier to edit and review YAML without Go string escaping
2. **Reusability**: Manifests can be used independently for manual testing
3. **Validation**: Standard YAML tools can validate syntax
4. **Version Control**: Better diff visualization in Git
5. **Documentation**: Self-documenting through comments in YAML

## Testing Changes

After modifying these manifests, verify the e2e tests still work:

```bash
# Compile tests
go test -c github.com/opendatahub-io/mlflow-operator/test/e2e

# Run tests (requires Kubernetes cluster)
cd ../..  # Return to project root
make test-e2e
```

## Adding New Manifests

When adding new test manifests:

1. Create the manifest file in this directory
2. Add `.tmpl` extension if it requires templating
3. Document template variables if applicable
4. Update this README with file description
5. Update e2e tests to load the new manifest
