package transform

import (
	jsonpatch "github.com/evanphx/json-patch"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// TransformArtifact represents the transformation result for a single resource
// including patches, whiteout status, and metadata for Kustomize generation
type TransformArtifact struct {
	// Resource is the original Kubernetes resource
	Resource unstructured.Unstructured

	// HaveWhiteOut indicates if this resource should be excluded from output
	HaveWhiteOut bool

	// Patches contains all JSONPatch operations to be applied
	Patches jsonpatch.Patch

	// IgnoredOps contains operations that were ignored due to conflicts
	IgnoredOps []IgnoredOperation

	// Target contains the Kustomize patch target metadata
	Target PatchTarget

	// PluginName is the name of the plugin that generated this artifact (for multi-stage)
	PluginName string
}

// PatchTarget contains Kustomize patch target selector metadata
// derived from the resource's apiVersion, kind, and metadata
type PatchTarget struct {
	// Group is the API group (empty string for core resources)
	Group string

	// Version is the API version (e.g., "v1", "v1beta1")
	Version string

	// Kind is the resource kind (e.g., "Deployment", "Service")
	Kind string

	// Name is the resource name from metadata.name
	Name string

	// Namespace is the resource namespace from metadata.namespace (optional)
	Namespace string
}

// IgnoredOperation represents a JSONPatch operation that was discarded
// due to conflicts with higher priority plugins
type IgnoredOperation struct {
	// Operation is the JSONPatch operation that was ignored
	Operation jsonpatch.Operation

	// Plugin is the name of the plugin that generated this operation
	Plugin string

	// Reason describes why the operation was ignored (e.g., "path-conflict-priority")
	Reason string

	// WinnerPlugin is the name of the plugin whose operation was selected instead
	WinnerPlugin string
}

// ResourceGroup represents resources grouped by type (kind + group)
// for multi-doc YAML file generation
type ResourceGroup struct {
	// TypeKey is the unique identifier for this resource type
	// Format: "<kind>" for core resources, "<kind>.<group>" for others
	// Examples: "deployment", "service", "route.route.openshift.io"
	TypeKey string

	// Resources contains all resources of this type
	Resources []unstructured.Unstructured
}

// DeriveTargetFromResource extracts PatchTarget metadata from a Kubernetes resource
func DeriveTargetFromResource(resource unstructured.Unstructured) PatchTarget {
	apiVersion := resource.GetAPIVersion()
	group, version := parseAPIVersion(apiVersion)

	return PatchTarget{
		Group:     group,
		Version:   version,
		Kind:      resource.GetKind(),
		Name:      resource.GetName(),
		Namespace: resource.GetNamespace(),
	}
}

// parseAPIVersion splits apiVersion into group and version components
// Examples:
//   "v1" -> ("", "v1")
//   "apps/v1" -> ("apps", "v1")
//   "route.openshift.io/v1" -> ("route.openshift.io", "v1")
func parseAPIVersion(apiVersion string) (group, version string) {
	// Core API resources have no group (e.g., "v1")
	if apiVersion == "" {
		return "", ""
	}

	// Split by "/" to separate group from version
	parts := splitAPIVersion(apiVersion)
	if len(parts) == 1 {
		// Core resource (e.g., "v1")
		return "", parts[0]
	}

	// Non-core resource (e.g., "apps/v1")
	return parts[0], parts[1]
}

// splitAPIVersion is a helper to split apiVersion string
func splitAPIVersion(apiVersion string) []string {
	result := []string{}
	current := ""

	for i, char := range apiVersion {
		if char == '/' {
			if current != "" {
				result = append(result, current)
				current = ""
			}
		} else {
			current += string(char)
		}

		// Add the last part
		if i == len(apiVersion)-1 && current != "" {
			result = append(result, current)
		}
	}

	return result
}

// GetResourceTypeKey returns the unique type key for grouping resources
// Format: "<kind>" for core resources, "<kind>.<group>" for non-core
func GetResourceTypeKey(resource unstructured.Unstructured) string {
	kind := resource.GetKind()
	apiVersion := resource.GetAPIVersion()
	group, _ := parseAPIVersion(apiVersion)

	if group == "" {
		// Core resource - use kind only
		return kind
	}

	// Non-core resource - use kind.group
	return kind + "." + group
}
