package controller

import (
	"context"
	"testing"

	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	modulev1alpha1 "github.com/opendatahub-io/mlflow-operator/api/components/v1alpha1"
	mlflowv1 "github.com/opendatahub-io/mlflow-operator/api/v1"
	"github.com/opendatahub-io/mlflow-operator/internal/config"
)

func TestResolveOperatorConfigFromBaseKeepsLegacyPathWhenToggleDisabled(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := modulev1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("add MLflowOperator scheme: %v", err)
	}

	module := &modulev1alpha1.MLflowOperator{}
	module.Name = modulev1alpha1.MLflowOperatorInstanceName
	module.Spec.Gateway = &modulev1alpha1.GatewaySpec{Domain: "ignored.apps.example.com"}
	module.Spec.GatewayName = "ignored-gateway"
	module.Spec.SectionTitle = "Ignored"

	client := fake.NewClientBuilder().WithScheme(scheme).WithObjects(module).Build()
	reconciler := &MLflowReconciler{Client: client, Scheme: scheme, Namespace: "legacy-ns"}

	resolved, err := reconciler.resolveOperatorConfigFromBase(context.Background(), &config.OperatorConfig{
		ApplicationsNamespace:                "legacy-ns",
		EnableMLflowOperatorModuleController: false,
		GatewayName:                          "legacy-gateway",
		MLflowURL:                            "https://legacy.example.com",
		MLflowURLConfigured:                  true,
		SectionTitle:                         "Legacy",
	})
	if err != nil {
		t.Fatalf("resolve config: %v", err)
	}

	if resolved.ApplicationsNamespace != "legacy-ns" {
		t.Fatalf("expected legacy namespace, got %q", resolved.ApplicationsNamespace)
	}
	if resolved.MLflowURL != "https://legacy.example.com" {
		t.Fatalf("expected legacy URL, got %q", resolved.MLflowURL)
	}
	if resolved.GatewayName != "legacy-gateway" {
		t.Fatalf("expected legacy gateway name, got %q", resolved.GatewayName)
	}
	if resolved.SectionTitle != "Legacy" {
		t.Fatalf("expected legacy section title, got %q", resolved.SectionTitle)
	}
}

func TestResolveOperatorConfigFromBaseOverlaysModuleSpecWhenToggleEnabled(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := modulev1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("add MLflowOperator scheme: %v", err)
	}

	module := &modulev1alpha1.MLflowOperator{}
	module.Name = modulev1alpha1.MLflowOperatorInstanceName
	module.Spec.Gateway = &modulev1alpha1.GatewaySpec{Domain: "gateway.apps.example.com"}
	module.Spec.GatewayName = "modular-gateway"
	module.Spec.SectionTitle = "OpenShift Self Managed Services"

	client := fake.NewClientBuilder().WithScheme(scheme).WithObjects(module).Build()
	reconciler := &MLflowReconciler{Client: client, Scheme: scheme, Namespace: "legacy-ns"}

	resolved, err := reconciler.resolveOperatorConfigFromBase(context.Background(), &config.OperatorConfig{
		ApplicationsNamespace:                "legacy-ns",
		EnableMLflowOperatorModuleController: true,
		GatewayName:                          "legacy-gateway",
		MLflowURL:                            "https://legacy.example.com",
		MLflowURLConfigured:                  true,
		SectionTitle:                         "Legacy",
	})
	if err != nil {
		t.Fatalf("resolve config: %v", err)
	}

	if resolved.ApplicationsNamespace != "legacy-ns" {
		t.Fatalf("expected deployment namespace to remain authoritative, got %q", resolved.ApplicationsNamespace)
	}
	if resolved.MLflowURL != "https://gateway.apps.example.com" {
		t.Fatalf("expected module gateway URL, got %q", resolved.MLflowURL)
	}
	if !resolved.MLflowURLConfigured {
		t.Fatalf("expected module URL to count as configured")
	}
	if resolved.GatewayName != "modular-gateway" {
		t.Fatalf("expected module gateway name, got %q", resolved.GatewayName)
	}
	if resolved.SectionTitle != "OpenShift Self Managed Services" {
		t.Fatalf("expected module section title, got %q", resolved.SectionTitle)
	}
}

func TestGatewayDomainToURL(t *testing.T) {
	tests := map[string]string{
		"gateway.apps.example.com":                "https://gateway.apps.example.com",
		"https://gateway.apps.example.com":        "https://gateway.apps.example.com",
		"https://gateway.apps.example.com/":       "https://gateway.apps.example.com",
		"   https://gateway.apps.example.com/   ": "https://gateway.apps.example.com",
		"   /   ": "",
	}

	for input, expected := range tests {
		if got := gatewayDomainToURL(input); got != expected {
			t.Fatalf("gatewayDomainToURL(%q) = %q, want %q", input, got, expected)
		}
	}
}

func TestResolveOperatorConfigFromBaseSkipsBlankGatewayDomain(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := modulev1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("add MLflowOperator scheme: %v", err)
	}

	module := &modulev1alpha1.MLflowOperator{}
	module.Name = modulev1alpha1.MLflowOperatorInstanceName
	module.Spec.Gateway = &modulev1alpha1.GatewaySpec{Domain: "   /   "}

	client := fake.NewClientBuilder().WithScheme(scheme).WithObjects(module).Build()
	reconciler := &MLflowReconciler{Client: client, Scheme: scheme, Namespace: "legacy-ns"}

	resolved, err := reconciler.resolveOperatorConfigFromBase(context.Background(), &config.OperatorConfig{
		ApplicationsNamespace:                "legacy-ns",
		EnableMLflowOperatorModuleController: true,
		MLflowURL:                            "https://legacy.example.com",
		MLflowURLConfigured:                  true,
	})
	if err != nil {
		t.Fatalf("resolve config: %v", err)
	}

	if resolved.MLflowURL != "https://legacy.example.com" {
		t.Fatalf("expected legacy URL to remain when gateway domain is blank, got %q", resolved.MLflowURL)
	}
	if !resolved.MLflowURLConfigured {
		t.Fatalf("expected MLflowURLConfigured to remain true when blank gateway domain is ignored")
	}
}

func TestEnsureMLflowOperatorReadyBlocksWhenModuleMissing(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := modulev1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("add MLflowOperator scheme: %v", err)
	}
	if err := mlflowv1.AddToScheme(scheme); err != nil {
		t.Fatalf("add MLflow scheme: %v", err)
	}

	mlflow := &mlflowv1.MLflow{}
	mlflow.Name = "mlflow"

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&mlflowv1.MLflow{}).
		WithObjects(mlflow).
		Build()

	reconciler := &MLflowReconciler{Client: client, Scheme: scheme, Namespace: "opendatahub"}
	result, handled, err := reconciler.ensureMLflowOperatorReady(context.Background(), mlflow, &config.OperatorConfig{
		EnableMLflowOperatorModuleController: true,
	})
	if err != nil {
		t.Fatalf("ensure module ready: %v", err)
	}
	if !handled {
		t.Fatalf("expected missing MLflowOperator to block MLflow reconciliation")
	}
	if result.RequeueAfter <= 0 {
		t.Fatalf("expected requeue after dependency block, got %v", result.RequeueAfter)
	}

	updated := &mlflowv1.MLflow{}
	if err := client.Get(context.Background(), types.NamespacedName{Name: mlflow.Name}, updated); err != nil {
		t.Fatalf("get updated MLflow: %v", err)
	}
	available := apimeta.FindStatusCondition(updated.Status.Conditions, "Available")
	if available == nil || available.Reason != "MLflowOperatorMissing" {
		t.Fatalf("expected Available condition with MLflowOperatorMissing, got %#v", available)
	}
	dependency := apimeta.FindStatusCondition(updated.Status.Conditions, mlflowOperatorReadyConditionType)
	if dependency == nil || dependency.Status != metav1.ConditionFalse || dependency.Reason != "MLflowOperatorMissing" {
		t.Fatalf("expected MLflowOperatorReady=False with MLflowOperatorMissing, got %#v", dependency)
	}
}

func TestEnsureMLflowOperatorReadyPassesWhenModuleReady(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := modulev1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("add MLflowOperator scheme: %v", err)
	}
	if err := mlflowv1.AddToScheme(scheme); err != nil {
		t.Fatalf("add MLflow scheme: %v", err)
	}

	module := &modulev1alpha1.MLflowOperator{}
	module.Name = modulev1alpha1.MLflowOperatorInstanceName
	module.Generation = 2
	module.Status.ObservedGeneration = 2
	module.Status.Conditions = []metav1.Condition{{
		Type:   readyConditionType,
		Status: metav1.ConditionTrue,
		Reason: readyReason,
	}}

	mlflow := &mlflowv1.MLflow{}
	mlflow.Name = "mlflow"

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&mlflowv1.MLflow{}).
		WithObjects(module, mlflow).
		Build()

	reconciler := &MLflowReconciler{Client: client, Scheme: scheme, Namespace: "opendatahub"}
	result, handled, err := reconciler.ensureMLflowOperatorReady(context.Background(), mlflow, &config.OperatorConfig{
		EnableMLflowOperatorModuleController: true,
	})
	if err != nil {
		t.Fatalf("ensure module ready: %v", err)
	}
	if handled {
		t.Fatalf("expected ready MLflowOperator to allow MLflow reconciliation, got result=%#v", result)
	}
	dependency := apimeta.FindStatusCondition(mlflow.Status.Conditions, mlflowOperatorReadyConditionType)
	if dependency == nil || dependency.Status != metav1.ConditionTrue {
		t.Fatalf("expected MLflowOperatorReady=True when module is ready, got %#v", dependency)
	}
}
