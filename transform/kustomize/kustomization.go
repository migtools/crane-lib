package kustomize

import (
	"fmt"
	"sort"

	"sigs.k8s.io/yaml"
)

// KustomizationFile represents the structure of a kustomization.yaml file
type KustomizationFile struct {
	APIVersion string   `json:"apiVersion,omitempty" yaml:"apiVersion,omitempty"`
	Kind       string   `json:"kind,omitempty" yaml:"kind,omitempty"`
	Resources  []string `json:"resources,omitempty" yaml:"resources,omitempty"`
	Patches    []Patch  `json:"patches,omitempty" yaml:"patches,omitempty"`
}

// Patch represents a Kustomize patch with target selector
type Patch struct {
	Path   string      `json:"path" yaml:"path"`
	Target PatchTarget `json:"target" yaml:"target"`
}

// PatchTarget represents the Kustomize patch target selector
type PatchTarget struct {
	Group     string `json:"group,omitempty" yaml:"group,omitempty"`
	Version   string `json:"version,omitempty" yaml:"version,omitempty"`
	Kind      string `json:"kind,omitempty" yaml:"kind,omitempty"`
	Name      string `json:"name,omitempty" yaml:"name,omitempty"`
	Namespace string `json:"namespace,omitempty" yaml:"namespace,omitempty"`
}

// GenerateKustomization creates a kustomization.yaml file content
// with deterministic ordering for stable Git diffs
func GenerateKustomization(resources []string, patches []Patch) ([]byte, error) {
	// Sort resources lexically for determinism
	sortedResources := make([]string, len(resources))
	copy(sortedResources, resources)
	sort.Strings(sortedResources)

	// Sort patches lexically by path for determinism
	sortedPatches := make([]Patch, len(patches))
	copy(sortedPatches, patches)
	sort.Slice(sortedPatches, func(i, j int) bool {
		return sortedPatches[i].Path < sortedPatches[j].Path
	})

	kustomization := KustomizationFile{
		APIVersion: "kustomize.config.k8s.io/v1beta1",
		Kind:       "Kustomization",
		Resources:  sortedResources,
		Patches:    sortedPatches,
	}

	// Marshal to YAML
	yamlBytes, err := yaml.Marshal(kustomization)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal kustomization.yaml: %w", err)
	}

	return yamlBytes, nil
}

// NewPatch creates a new Patch with target selector
func NewPatch(path, group, version, kind, name, namespace string) Patch {
	return Patch{
		Path: path,
		Target: PatchTarget{
			Group:     group,
			Version:   version,
			Kind:      kind,
			Name:      name,
			Namespace: namespace,
		},
	}
}
