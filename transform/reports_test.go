package transform

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGenerateWhiteoutReport(t *testing.T) {
	whiteouts := []WhiteoutReport{
		{
			APIVersion:  "route.openshift.io/v1",
			Kind:        "Route",
			Name:        "frontend",
			Namespace:   "ns1",
			RequestedBy: []string{"OpenShiftPlugin"},
		},
		{
			APIVersion:  "v1",
			Kind:        "Service",
			Name:        "backend",
			Namespace:   "ns1",
			RequestedBy: []string{"KubernetesPlugin"},
		},
	}

	jsonBytes, err := GenerateWhiteoutReport(whiteouts)
	require.NoError(t, err)

	// Verify it's valid JSON
	assert.Contains(t, string(jsonBytes), "frontend")
	assert.Contains(t, string(jsonBytes), "backend")
	assert.Contains(t, string(jsonBytes), "OpenShiftPlugin")
}

func TestGenerateIgnoredPatchReport(t *testing.T) {
	ignored := []IgnoredPatchReport{
		{
			Resource: ResourceIdentity{
				APIVersion: "apps/v1",
				Kind:       "Deployment",
				Name:       "myapp",
				Namespace:  "ns1",
			},
			Path:           "/spec/template/spec/containers/0/image",
			Operation:      "replace",
			SelectedPlugin: "OpenShiftPlugin",
			IgnoredPlugin:  "ImageStreamPlugin",
			Reason:         "path-conflict-priority",
		},
	}

	jsonBytes, err := GenerateIgnoredPatchReport(ignored)
	require.NoError(t, err)

	// Verify it's valid JSON
	assert.Contains(t, string(jsonBytes), "myapp")
	assert.Contains(t, string(jsonBytes), "OpenShiftPlugin")
	assert.Contains(t, string(jsonBytes), "path-conflict-priority")
}

func TestSortWhiteouts(t *testing.T) {
	whiteouts := []WhiteoutReport{
		{Kind: "Service", Name: "z-service", Namespace: "ns2"},
		{Kind: "Deployment", Name: "a-deployment", Namespace: "ns1"},
		{Kind: "Service", Name: "a-service", Namespace: "ns1"},
		{Kind: "Deployment", Name: "z-deployment", Namespace: "ns2"},
	}

	SortWhiteouts(whiteouts)

	// Expected order: ns1/Deployment/a-deployment, ns1/Service/a-service, ns2/Deployment/z-deployment, ns2/Service/z-service
	assert.Equal(t, "ns1", whiteouts[0].Namespace)
	assert.Equal(t, "Deployment", whiteouts[0].Kind)
	assert.Equal(t, "a-deployment", whiteouts[0].Name)

	assert.Equal(t, "ns1", whiteouts[1].Namespace)
	assert.Equal(t, "Service", whiteouts[1].Kind)
	assert.Equal(t, "a-service", whiteouts[1].Name)

	assert.Equal(t, "ns2", whiteouts[2].Namespace)
	assert.Equal(t, "Deployment", whiteouts[2].Kind)
	assert.Equal(t, "z-deployment", whiteouts[2].Name)

	assert.Equal(t, "ns2", whiteouts[3].Namespace)
	assert.Equal(t, "Service", whiteouts[3].Kind)
	assert.Equal(t, "z-service", whiteouts[3].Name)
}

func TestSortIgnoredPatches(t *testing.T) {
	reports := []IgnoredPatchReport{
		{
			Resource: ResourceIdentity{Namespace: "ns2", Kind: "Service", Name: "svc"},
			Path:     "/spec/type",
		},
		{
			Resource: ResourceIdentity{Namespace: "ns1", Kind: "Deployment", Name: "deploy"},
			Path:     "/spec/replicas",
		},
		{
			Resource: ResourceIdentity{Namespace: "ns1", Kind: "Deployment", Name: "deploy"},
			Path:     "/spec/template",
		},
	}

	SortIgnoredPatches(reports)

	// Expected order: ns1/Deployment/deploy/spec/replicas, ns1/Deployment/deploy/spec/template, ns2/Service/svc/spec/type
	assert.Equal(t, "ns1", reports[0].Resource.Namespace)
	assert.Equal(t, "/spec/replicas", reports[0].Path)

	assert.Equal(t, "ns1", reports[1].Resource.Namespace)
	assert.Equal(t, "/spec/template", reports[1].Path)

	assert.Equal(t, "ns2", reports[2].Resource.Namespace)
}

func TestWriteAndReadWhiteoutReport(t *testing.T) {
	tempDir := t.TempDir()
	filename := filepath.Join(tempDir, "whiteouts.json")

	whiteouts := []WhiteoutReport{
		{
			APIVersion:  "v1",
			Kind:        "Service",
			Name:        "test-service",
			Namespace:   "default",
			RequestedBy: []string{"TestPlugin"},
		},
	}

	// Write report
	err := WriteWhiteoutReport(filename, whiteouts)
	require.NoError(t, err)

	// Verify file exists
	_, err = os.Stat(filename)
	require.NoError(t, err)

	// Read report back
	readWhiteouts, err := ReadWhiteoutReport(filename)
	require.NoError(t, err)

	// Verify content
	require.Len(t, readWhiteouts, 1)
	assert.Equal(t, "Service", readWhiteouts[0].Kind)
	assert.Equal(t, "test-service", readWhiteouts[0].Name)
	assert.Equal(t, "default", readWhiteouts[0].Namespace)
}

func TestWriteAndReadIgnoredPatchReport(t *testing.T) {
	tempDir := t.TempDir()
	filename := filepath.Join(tempDir, "ignored-patches.json")

	ignored := []IgnoredPatchReport{
		{
			Resource: ResourceIdentity{
				APIVersion: "apps/v1",
				Kind:       "Deployment",
				Name:       "test-deployment",
				Namespace:  "default",
			},
			Path:           "/spec/replicas",
			Operation:      "replace",
			SelectedPlugin: "PluginA",
			IgnoredPlugin:  "PluginB",
			Reason:         "priority",
		},
	}

	// Write report
	err := WriteIgnoredPatchReport(filename, ignored)
	require.NoError(t, err)

	// Verify file exists
	_, err = os.Stat(filename)
	require.NoError(t, err)

	// Read report back
	readIgnored, err := ReadIgnoredPatchReport(filename)
	require.NoError(t, err)

	// Verify content
	require.Len(t, readIgnored, 1)
	assert.Equal(t, "Deployment", readIgnored[0].Resource.Kind)
	assert.Equal(t, "test-deployment", readIgnored[0].Resource.Name)
	assert.Equal(t, "/spec/replicas", readIgnored[0].Path)
	assert.Equal(t, "PluginA", readIgnored[0].SelectedPlugin)
}
