package kustomize

import (
	"strings"
)

// GetResourceTypeFilename generates a deterministic filename for a resource type
// Format:
//   - Core types: <kind>.yaml (e.g., "deployment.yaml", "service.yaml")
//   - Non-core types: <kind>.<group>.yaml (e.g., "route.route.openshift.io.yaml")
//
// Note: Version is NOT included in the filename - it's in the resource YAML itself
func GetResourceTypeFilename(kind, group string) string {
	// Lowercase kind for filename
	kindLower := strings.ToLower(kind)

	if group == "" {
		// Core API resource
		return kindLower + ".yaml"
	}

	// Non-core API resource
	return kindLower + "." + group + ".yaml"
}

// SanitizeFilename sanitizes a filename for filesystem safety
// This is primarily for edge cases with unusual characters
func SanitizeFilename(filename string) string {
	// Replace problematic characters
	filename = strings.ReplaceAll(filename, "/", "-")
	filename = strings.ReplaceAll(filename, "\\", "-")
	filename = strings.ReplaceAll(filename, ":", "-")
	filename = strings.ReplaceAll(filename, "*", "-")
	filename = strings.ReplaceAll(filename, "?", "-")
	filename = strings.ReplaceAll(filename, "\"", "-")
	filename = strings.ReplaceAll(filename, "<", "-")
	filename = strings.ReplaceAll(filename, ">", "-")
	filename = strings.ReplaceAll(filename, "|", "-")

	// Remove leading/trailing dots and spaces
	filename = strings.Trim(filename, ". ")

	return filename
}

// ParseResourceTypeFilename parses a resource type filename back into kind and group
// This is the inverse of GetResourceTypeFilename
func ParseResourceTypeFilename(filename string) (kind, group string) {
	// Remove .yaml extension
	if strings.HasSuffix(filename, ".yaml") {
		filename = strings.TrimSuffix(filename, ".yaml")
	}

	// Split by first dot
	parts := strings.SplitN(filename, ".", 2)

	if len(parts) == 1 {
		// Core resource
		return parts[0], ""
	}

	// Non-core resource
	return parts[0], parts[1]
}
