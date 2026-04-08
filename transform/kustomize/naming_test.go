package kustomize

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetResourceTypeFilename(t *testing.T) {
	tests := []struct {
		name             string
		kind             string
		group            string
		expectedFilename string
	}{
		{
			name:             "core Service",
			kind:             "Service",
			group:            "",
			expectedFilename: "service.yaml",
		},
		{
			name:             "core ConfigMap",
			kind:             "ConfigMap",
			group:            "",
			expectedFilename: "configmap.yaml",
		},
		{
			name:             "apps Deployment",
			kind:             "Deployment",
			group:            "apps",
			expectedFilename: "deployment.apps.yaml",
		},
		{
			name:             "OpenShift Route",
			kind:             "Route",
			group:            "route.openshift.io",
			expectedFilename: "route.route.openshift.io.yaml",
		},
		{
			name:             "ImageStream",
			kind:             "ImageStream",
			group:            "image.openshift.io",
			expectedFilename: "imagestream.image.openshift.io.yaml",
		},
		{
			name:             "CRD - Certificate",
			kind:             "Certificate",
			group:            "cert-manager.io",
			expectedFilename: "certificate.cert-manager.io.yaml",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetResourceTypeFilename(tt.kind, tt.group)
			assert.Equal(t, tt.expectedFilename, result)
		})
	}
}

func TestParseResourceTypeFilename(t *testing.T) {
	tests := []struct {
		name          string
		filename      string
		expectedKind  string
		expectedGroup string
	}{
		{
			name:          "core service",
			filename:      "service.yaml",
			expectedKind:  "service",
			expectedGroup: "",
		},
		{
			name:          "apps deployment",
			filename:      "deployment.apps.yaml",
			expectedKind:  "deployment",
			expectedGroup: "apps",
		},
		{
			name:          "OpenShift route",
			filename:      "route.route.openshift.io.yaml",
			expectedKind:  "route",
			expectedGroup: "route.openshift.io",
		},
		{
			name:          "without .yaml extension",
			filename:      "service",
			expectedKind:  "service",
			expectedGroup: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			kind, group := ParseResourceTypeFilename(tt.filename)
			assert.Equal(t, tt.expectedKind, kind)
			assert.Equal(t, tt.expectedGroup, group)
		})
	}
}

func TestSanitizeFilename(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "clean filename",
			input:    "deployment.yaml",
			expected: "deployment.yaml",
		},
		{
			name:     "with slashes",
			input:    "my/file.yaml",
			expected: "my-file.yaml",
		},
		{
			name:     "with colons",
			input:    "ns:app.yaml",
			expected: "ns-app.yaml",
		},
		{
			name:     "with leading/trailing dots",
			input:    "..file.yaml..",
			expected: "file.yaml",
		},
		{
			name:     "with special characters",
			input:    "my*file?.yaml",
			expected: "my-file-.yaml",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := SanitizeFilename(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}
