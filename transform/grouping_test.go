package transform

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestGroupResourcesByType(t *testing.T) {
	resources := []unstructured.Unstructured{
		{
			Object: map[string]interface{}{
				"apiVersion": "v1",
				"kind":       "Service",
				"metadata": map[string]interface{}{
					"name": "service-1",
				},
			},
		},
		{
			Object: map[string]interface{}{
				"apiVersion": "apps/v1",
				"kind":       "Deployment",
				"metadata": map[string]interface{}{
					"name": "deployment-1",
				},
			},
		},
		{
			Object: map[string]interface{}{
				"apiVersion": "v1",
				"kind":       "Service",
				"metadata": map[string]interface{}{
					"name": "service-2",
				},
			},
		},
		{
			Object: map[string]interface{}{
				"apiVersion": "route.openshift.io/v1",
				"kind":       "Route",
				"metadata": map[string]interface{}{
					"name": "route-1",
				},
			},
		},
	}

	groups := GroupResourcesByType(resources)

	// Should have 3 groups: Service, Deployment.apps, Route.route.openshift.io
	assert.Len(t, groups, 3)

	// Check that each group has the correct number of resources
	groupCounts := make(map[string]int)
	for _, group := range groups {
		groupCounts[group.TypeKey] = len(group.Resources)
	}

	assert.Equal(t, 2, groupCounts["Service"], "Service group should have 2 resources")
	assert.Equal(t, 1, groupCounts["Deployment.apps"], "Deployment group should have 1 resource")
	assert.Equal(t, 1, groupCounts["Route.route.openshift.io"], "Route group should have 1 resource")
}

func TestWriteResourceTypeFile(t *testing.T) {
	// Create a temporary directory for test files
	tempDir := t.TempDir()

	resources := []unstructured.Unstructured{
		{
			Object: map[string]interface{}{
				"apiVersion": "v1",
				"kind":       "Service",
				"metadata": map[string]interface{}{
					"name":      "service-1",
					"namespace": "default",
				},
				"spec": map[string]interface{}{
					"type": "ClusterIP",
				},
			},
		},
		{
			Object: map[string]interface{}{
				"apiVersion": "v1",
				"kind":       "Service",
				"metadata": map[string]interface{}{
					"name":      "service-2",
					"namespace": "default",
				},
				"spec": map[string]interface{}{
					"type": "NodePort",
				},
			},
		},
	}

	filename := filepath.Join(tempDir, "service.yaml")

	// Write file
	err := WriteResourceTypeFile(filename, resources)
	require.NoError(t, err)

	// Verify file exists
	_, err = os.Stat(filename)
	require.NoError(t, err)

	// Read file content
	content, err := os.ReadFile(filename)
	require.NoError(t, err)

	// Verify content contains both resources separated by ---
	contentStr := string(content)
	assert.Contains(t, contentStr, "service-1")
	assert.Contains(t, contentStr, "service-2")
	assert.Contains(t, contentStr, "---")
}

func TestWriteResourceTypeFileEmpty(t *testing.T) {
	// Create a temporary directory for test files
	tempDir := t.TempDir()

	filename := filepath.Join(tempDir, "empty.yaml")

	// Write empty resource list
	err := WriteResourceTypeFile(filename, []unstructured.Unstructured{})
	require.NoError(t, err)

	// Verify file was NOT created (we don't create empty files)
	_, err = os.Stat(filename)
	assert.True(t, os.IsNotExist(err), "empty resource file should not be created")
}

func TestReadResourceTypeFile(t *testing.T) {
	// Create a temporary directory for test files
	tempDir := t.TempDir()

	// Create a multi-doc YAML file
	content := `apiVersion: v1
kind: Service
metadata:
  name: service-1
  namespace: default
spec:
  type: ClusterIP
---
apiVersion: v1
kind: Service
metadata:
  name: service-2
  namespace: default
spec:
  type: NodePort
`

	filename := filepath.Join(tempDir, "service.yaml")
	err := os.WriteFile(filename, []byte(content), 0644)
	require.NoError(t, err)

	// Read resources
	resources, err := ReadResourceTypeFile(filename)
	require.NoError(t, err)

	// Verify we got 2 resources
	assert.Len(t, resources, 2)

	// Verify first resource
	assert.Equal(t, "Service", resources[0].GetKind())
	assert.Equal(t, "service-1", resources[0].GetName())
	assert.Equal(t, "default", resources[0].GetNamespace())

	// Verify second resource
	assert.Equal(t, "Service", resources[1].GetKind())
	assert.Equal(t, "service-2", resources[1].GetName())
	assert.Equal(t, "default", resources[1].GetNamespace())
}

func TestSplitYAMLDocuments(t *testing.T) {
	tests := []struct {
		name          string
		input         string
		expectedCount int
	}{
		{
			name: "two documents with separator",
			input: `apiVersion: v1
kind: Service
---
apiVersion: v1
kind: ConfigMap`,
			expectedCount: 2,
		},
		{
			name: "single document",
			input: `apiVersion: v1
kind: Service`,
			expectedCount: 1,
		},
		{
			name: "three documents",
			input: `apiVersion: v1
kind: Service
---
apiVersion: v1
kind: ConfigMap
---
apiVersion: apps/v1
kind: Deployment`,
			expectedCount: 3,
		},
		{
			name:          "empty input",
			input:         "",
			expectedCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			docs := splitYAMLDocuments([]byte(tt.input))

			// Filter out empty documents
			nonEmptyDocs := 0
			for _, doc := range docs {
				if len(doc) > 0 {
					nonEmptyDocs++
				}
			}

			assert.Equal(t, tt.expectedCount, nonEmptyDocs)
		})
	}
}
