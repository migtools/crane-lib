package kustomize

import (
	"fmt"
	"strings"

	jsonpatch "github.com/evanphx/json-patch"
	"sigs.k8s.io/yaml"
)

// SerializePatchToYAML converts a JSONPatch patch to Kustomize-compatible YAML format
// The output is a YAML array of RFC 6902 JSON Patch operations
func SerializePatchToYAML(ops jsonpatch.Patch) ([]byte, error) {
	if len(ops) == 0 {
		return []byte("[]\n"), nil
	}

	// Convert each operation to a map for YAML serialization
	operations := make([]map[string]interface{}, 0, len(ops))

	for _, op := range ops {
		opMap := make(map[string]interface{})

		// Get operation kind (add, remove, replace, etc.)
		opMap["op"] = op.Kind()

		// Get path
		path, err := op.Path()
		if err != nil {
			return nil, fmt.Errorf("failed to get operation path: %w", err)
		}
		opMap["path"] = path

		// Get value if present (not for "remove" operations)
		if op.Kind() != "remove" {
			val, err := op.ValueInterface()
			if err == nil {
				opMap["value"] = val
			}
			// For "remove" operations or when value is missing, we don't include it
		}

		// Handle "from" field for move/copy operations
		if op.Kind() == "move" || op.Kind() == "copy" {
			from, err := op.From()
			if err == nil {
				opMap["from"] = from
			}
		}

		operations = append(operations, opMap)
	}

	// Marshal to YAML
	yamlBytes, err := yaml.Marshal(operations)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal patch to YAML: %w", err)
	}

	return yamlBytes, nil
}

// GeneratePatchFilename creates a deterministic filename for a patch file
// Format: <namespace>--<group>-<version>--<kind>--<name>.patch.yaml
// For cluster-scoped resources, namespace is omitted: <group>-<version>--<kind>--<name>.patch.yaml
func GeneratePatchFilename(group, version, kind, name, namespace string) string {
	// Sanitize components
	sanitizedKind := sanitizeComponent(kind)
	sanitizedName := sanitizeComponent(name)

	var filename string

	if namespace != "" {
		// Namespaced resource
		sanitizedNamespace := sanitizeComponent(namespace)
		sanitizedGroup := sanitizeComponent(group)

		if group == "" {
			// Core resource: <namespace>--<version>--<kind>--<name>.patch.yaml
			filename = fmt.Sprintf("%s--%s--%s--%s.patch.yaml",
				sanitizedNamespace,
				version,
				sanitizedKind,
				sanitizedName)
		} else {
			// Non-core resource: <namespace>--<group>-<version>--<kind>--<name>.patch.yaml
			filename = fmt.Sprintf("%s--%s-%s--%s--%s.patch.yaml",
				sanitizedNamespace,
				sanitizedGroup,
				version,
				sanitizedKind,
				sanitizedName)
		}
	} else {
		// Cluster-scoped resource
		sanitizedGroup := sanitizeComponent(group)

		if group == "" {
			// Core cluster-scoped: <version>--<kind>--<name>.patch.yaml
			filename = fmt.Sprintf("%s--%s--%s.patch.yaml",
				version,
				sanitizedKind,
				sanitizedName)
		} else {
			// Non-core cluster-scoped: <group>-<version>--<kind>--<name>.patch.yaml
			filename = fmt.Sprintf("%s-%s--%s--%s.patch.yaml",
				sanitizedGroup,
				version,
				sanitizedKind,
				sanitizedName)
		}
	}

	return filename
}

// sanitizeComponent sanitizes a single component for use in filename
// Replaces characters that are problematic in filenames with safe alternatives
func sanitizeComponent(s string) string {
	// Replace problematic characters
	s = strings.ReplaceAll(s, "/", "-")
	s = strings.ReplaceAll(s, "\\", "-")
	s = strings.ReplaceAll(s, ":", "-")
	s = strings.ReplaceAll(s, "*", "-")
	s = strings.ReplaceAll(s, "?", "-")
	s = strings.ReplaceAll(s, "\"", "-")
	s = strings.ReplaceAll(s, "<", "-")
	s = strings.ReplaceAll(s, ">", "-")
	s = strings.ReplaceAll(s, "|", "-")
	s = strings.ReplaceAll(s, " ", "-")

	return s
}
