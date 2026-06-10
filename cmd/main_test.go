package main

import (
	"testing"

	"github.com/opendatahub-io/mlflow-operator/internal/config"
)

func TestResolveManagerNamespace(t *testing.T) {
	tests := []struct {
		name              string
		namespace         string
		operatorConfig    *config.OperatorConfig
		expectedNamespace string
	}{
		{
			name:      "keeps legacy namespace when toggle disabled",
			namespace: "opendatahub",
			operatorConfig: &config.OperatorConfig{
				ApplicationsNamespace:                "redhat-ods-applications",
				EnableMLflowOperatorModuleController: false,
			},
			expectedNamespace: "opendatahub",
		},
		{
			name:      "uses applications namespace when toggle enabled",
			namespace: "opendatahub",
			operatorConfig: &config.OperatorConfig{
				ApplicationsNamespace:                "redhat-ods-applications",
				EnableMLflowOperatorModuleController: true,
			},
			expectedNamespace: "redhat-ods-applications",
		},
		{
			name:      "falls back when applications namespace empty",
			namespace: "opendatahub",
			operatorConfig: &config.OperatorConfig{
				EnableMLflowOperatorModuleController: true,
			},
			expectedNamespace: "opendatahub",
		},
		{
			name:              "falls back when config missing",
			namespace:         "opendatahub",
			operatorConfig:    nil,
			expectedNamespace: "opendatahub",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := resolveManagerNamespace(tt.namespace, tt.operatorConfig); got != tt.expectedNamespace {
				t.Fatalf("resolveManagerNamespace() = %q, want %q", got, tt.expectedNamespace)
			}
		})
	}
}
