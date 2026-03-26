package transform

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestDeriveTargetFromResource(t *testing.T) {
	tests := []struct {
		name     string
		resource unstructured.Unstructured
		expected PatchTarget
	}{
		{
			name: "core resource - Service",
			resource: unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "v1",
					"kind":       "Service",
					"metadata": map[string]interface{}{
						"name":      "my-service",
						"namespace": "default",
					},
				},
			},
			expected: PatchTarget{
				Group:     "",
				Version:   "v1",
				Kind:      "Service",
				Name:      "my-service",
				Namespace: "default",
			},
		},
		{
			name: "apps/v1 resource - Deployment",
			resource: unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "apps/v1",
					"kind":       "Deployment",
					"metadata": map[string]interface{}{
						"name":      "my-deployment",
						"namespace": "app-ns",
					},
				},
			},
			expected: PatchTarget{
				Group:     "apps",
				Version:   "v1",
				Kind:      "Deployment",
				Name:      "my-deployment",
				Namespace: "app-ns",
			},
		},
		{
			name: "OpenShift Route",
			resource: unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "route.openshift.io/v1",
					"kind":       "Route",
					"metadata": map[string]interface{}{
						"name":      "my-route",
						"namespace": "openshift-ns",
					},
				},
			},
			expected: PatchTarget{
				Group:     "route.openshift.io",
				Version:   "v1",
				Kind:      "Route",
				Name:      "my-route",
				Namespace: "openshift-ns",
			},
		},
		{
			name: "cluster-scoped resource (no namespace)",
			resource: unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "v1",
					"kind":       "Namespace",
					"metadata": map[string]interface{}{
						"name": "my-namespace",
					},
				},
			},
			expected: PatchTarget{
				Group:     "",
				Version:   "v1",
				Kind:      "Namespace",
				Name:      "my-namespace",
				Namespace: "",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := DeriveTargetFromResource(tt.resource)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestParseAPIVersion(t *testing.T) {
	tests := []struct {
		name            string
		apiVersion      string
		expectedGroup   string
		expectedVersion string
	}{
		{
			name:            "core v1",
			apiVersion:      "v1",
			expectedGroup:   "",
			expectedVersion: "v1",
		},
		{
			name:            "apps/v1",
			apiVersion:      "apps/v1",
			expectedGroup:   "apps",
			expectedVersion: "v1",
		},
		{
			name:            "route.openshift.io/v1",
			apiVersion:      "route.openshift.io/v1",
			expectedGroup:   "route.openshift.io",
			expectedVersion: "v1",
		},
		{
			name:            "batch/v1beta1",
			apiVersion:      "batch/v1beta1",
			expectedGroup:   "batch",
			expectedVersion: "v1beta1",
		},
		{
			name:            "empty string",
			apiVersion:      "",
			expectedGroup:   "",
			expectedVersion: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			group, version := parseAPIVersion(tt.apiVersion)
			assert.Equal(t, tt.expectedGroup, group, "group mismatch")
			assert.Equal(t, tt.expectedVersion, version, "version mismatch")
		})
	}
}

func TestGetResourceTypeKey(t *testing.T) {
	tests := []struct {
		name        string
		resource    unstructured.Unstructured
		expectedKey string
	}{
		{
			name: "core Service",
			resource: unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "v1",
					"kind":       "Service",
				},
			},
			expectedKey: "Service",
		},
		{
			name: "apps Deployment",
			resource: unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "apps/v1",
					"kind":       "Deployment",
				},
			},
			expectedKey: "Deployment.apps",
		},
		{
			name: "OpenShift Route",
			resource: unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "route.openshift.io/v1",
					"kind":       "Route",
				},
			},
			expectedKey: "Route.route.openshift.io",
		},
		{
			name: "core ConfigMap",
			resource: unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "v1",
					"kind":       "ConfigMap",
				},
			},
			expectedKey: "ConfigMap",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetResourceTypeKey(tt.resource)
			assert.Equal(t, tt.expectedKey, result)
		})
	}
}
