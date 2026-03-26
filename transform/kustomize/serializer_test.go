package kustomize

import (
	"testing"

	jsonpatch "github.com/evanphx/json-patch"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSerializePatchToYAML(t *testing.T) {
	tests := []struct {
		name          string
		patchJSON     string
		expectedYAML  string
		expectedError bool
	}{
		{
			name:      "empty patch",
			patchJSON: `[]`,
			expectedYAML: `[]
`,
			expectedError: false,
		},
		{
			name: "single remove operation",
			patchJSON: `[
				{"op": "remove", "path": "/spec/clusterIP"}
			]`,
			expectedYAML: `- op: remove
  path: /spec/clusterIP
`,
			expectedError: false,
		},
		{
			name: "single replace operation",
			patchJSON: `[
				{"op": "replace", "path": "/spec/type", "value": "NodePort"}
			]`,
			expectedYAML: `- op: replace
  path: /spec/type
  value: NodePort
`,
			expectedError: false,
		},
		{
			name: "multiple operations",
			patchJSON: `[
				{"op": "remove", "path": "/spec/clusterIP"},
				{"op": "replace", "path": "/spec/type", "value": "NodePort"}
			]`,
			expectedYAML: `- op: remove
  path: /spec/clusterIP
- op: replace
  path: /spec/type
  value: NodePort
`,
			expectedError: false,
		},
		{
			name: "add operation with object value",
			patchJSON: `[
				{"op": "add", "path": "/metadata/labels", "value": {"app": "test"}}
			]`,
			expectedYAML: `- op: add
  path: /metadata/labels
  value:
    app: test
`,
			expectedError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Parse JSON patch
			patch, err := jsonpatch.DecodePatch([]byte(tt.patchJSON))
			require.NoError(t, err, "failed to decode test patch")

			// Serialize to YAML
			yamlBytes, err := SerializePatchToYAML(patch)

			if tt.expectedError {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expectedYAML, string(yamlBytes))
			}
		})
	}
}

func TestGeneratePatchFilename(t *testing.T) {
	tests := []struct {
		name             string
		group            string
		version          string
		kind             string
		resourceName     string
		namespace        string
		expectedFilename string
	}{
		{
			name:             "core namespaced resource",
			group:            "",
			version:          "v1",
			kind:             "Service",
			resourceName:     "my-service",
			namespace:        "default",
			expectedFilename: "default--v1--Service--my-service.patch.yaml",
		},
		{
			name:             "apps/v1 Deployment",
			group:            "apps",
			version:          "v1",
			kind:             "Deployment",
			resourceName:     "nginx",
			namespace:        "app-ns",
			expectedFilename: "app-ns--apps-v1--Deployment--nginx.patch.yaml",
		},
		{
			name:             "OpenShift Route",
			group:            "route.openshift.io",
			version:          "v1",
			kind:             "Route",
			resourceName:     "frontend",
			namespace:        "openshift",
			expectedFilename: "openshift--route.openshift.io-v1--Route--frontend.patch.yaml",
		},
		{
			name:             "cluster-scoped core resource",
			group:            "",
			version:          "v1",
			kind:             "Namespace",
			resourceName:     "my-namespace",
			namespace:        "",
			expectedFilename: "v1--Namespace--my-namespace.patch.yaml",
		},
		{
			name:             "cluster-scoped non-core resource",
			group:            "rbac.authorization.k8s.io",
			version:          "v1",
			kind:             "ClusterRole",
			resourceName:     "admin",
			namespace:        "",
			expectedFilename: "rbac.authorization.k8s.io-v1--ClusterRole--admin.patch.yaml",
		},
		{
			name:             "resource with special characters in name",
			group:            "apps",
			version:          "v1",
			kind:             "Deployment",
			resourceName:     "my:app/test",
			namespace:        "ns:special",
			expectedFilename: "ns-special--apps-v1--Deployment--my-app-test.patch.yaml",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GeneratePatchFilename(tt.group, tt.version, tt.kind, tt.resourceName, tt.namespace)
			assert.Equal(t, tt.expectedFilename, result)
		})
	}
}

func TestSanitizeComponent(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "clean string",
			input:    "my-app",
			expected: "my-app",
		},
		{
			name:     "with slashes",
			input:    "my/app/test",
			expected: "my-app-test",
		},
		{
			name:     "with colons",
			input:    "ns:app",
			expected: "ns-app",
		},
		{
			name:     "with spaces",
			input:    "my app",
			expected: "my-app",
		},
		{
			name:     "with multiple special chars",
			input:    "my:app/test*file",
			expected: "my-app-test-file",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := sanitizeComponent(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}
