package kustomize

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGenerateKustomization(t *testing.T) {
	tests := []struct {
		name         string
		resources    []string
		patches      []Patch
		expectedYAML string
	}{
		{
			name:      "empty kustomization",
			resources: []string{},
			patches:   []Patch{},
			expectedYAML: `apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization
`,
		},
		{
			name: "resources only",
			resources: []string{
				"resources/deployment.yaml",
				"resources/service.yaml",
			},
			patches: []Patch{},
			expectedYAML: `apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization
resources:
- resources/deployment.yaml
- resources/service.yaml
`,
		},
		{
			name:      "resources with patches",
			resources: []string{
				"resources/deployment.yaml",
				"resources/service.yaml",
			},
			patches: []Patch{
				{
					Path: "patches/default--apps-v1--Deployment--nginx.patch.yaml",
					Target: PatchTarget{
						Group:     "apps",
						Version:   "v1",
						Kind:      "Deployment",
						Name:      "nginx",
						Namespace: "default",
					},
				},
			},
			expectedYAML: `apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization
patches:
- path: patches/default--apps-v1--Deployment--nginx.patch.yaml
  target:
    group: apps
    kind: Deployment
    name: nginx
    namespace: default
    version: v1
resources:
- resources/deployment.yaml
- resources/service.yaml
`,
		},
		{
			name: "deterministic ordering - unsorted input",
			resources: []string{
				"resources/service.yaml",
				"resources/configmap.yaml",
				"resources/deployment.yaml",
			},
			patches: []Patch{
				{
					Path: "patches/z-patch.yaml",
					Target: PatchTarget{
						Kind: "Service",
						Name: "z",
					},
				},
				{
					Path: "patches/a-patch.yaml",
					Target: PatchTarget{
						Kind: "Deployment",
						Name: "a",
					},
				},
			},
			expectedYAML: `apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization
patches:
- path: patches/a-patch.yaml
  target:
    kind: Deployment
    name: a
- path: patches/z-patch.yaml
  target:
    kind: Service
    name: z
resources:
- resources/configmap.yaml
- resources/deployment.yaml
- resources/service.yaml
`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			yamlBytes, err := GenerateKustomization(tt.resources, tt.patches)
			require.NoError(t, err)
			assert.Equal(t, tt.expectedYAML, string(yamlBytes))
		})
	}
}

func TestNewPatch(t *testing.T) {
	patch := NewPatch(
		"patches/test.patch.yaml",
		"apps",
		"v1",
		"Deployment",
		"nginx",
		"default",
	)

	assert.Equal(t, "patches/test.patch.yaml", patch.Path)
	assert.Equal(t, "apps", patch.Target.Group)
	assert.Equal(t, "v1", patch.Target.Version)
	assert.Equal(t, "Deployment", patch.Target.Kind)
	assert.Equal(t, "nginx", patch.Target.Name)
	assert.Equal(t, "default", patch.Target.Namespace)
}
