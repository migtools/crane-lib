package transform

import (
	"bytes"
	"fmt"
	"os"
	"sort"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/yaml"
)

// GroupResourcesByType groups resources by their type (kind + group)
// Resources are grouped into ResourceGroup structures for multi-doc YAML file generation
func GroupResourcesByType(resources []unstructured.Unstructured) []ResourceGroup {
	// Use a map to collect resources by type key
	groupMap := make(map[string][]unstructured.Unstructured)

	// Group resources
	for _, resource := range resources {
		typeKey := GetResourceTypeKey(resource)
		groupMap[typeKey] = append(groupMap[typeKey], resource)
	}

	// Convert map to slice of ResourceGroup
	groups := make([]ResourceGroup, 0, len(groupMap))
	for typeKey, resources := range groupMap {
		groups = append(groups, ResourceGroup{
			TypeKey:   typeKey,
			Resources: resources,
		})
	}

	// Sort groups by TypeKey for deterministic output
	sort.Slice(groups, func(i, j int) bool {
		return groups[i].TypeKey < groups[j].TypeKey
	})

	return groups
}

// WriteResourceTypeFile writes a group of resources to a multi-doc YAML file
// Resources are separated by "---" separator as per YAML multi-doc format
func WriteResourceTypeFile(filename string, resources []unstructured.Unstructured) error {
	if len(resources) == 0 {
		// Don't create empty files
		return nil
	}

	var buf bytes.Buffer

	for i, resource := range resources {
		// Add YAML document separator before each resource (except the first)
		if i > 0 {
			buf.WriteString("---\n")
		}

		// Marshal resource to YAML
		yamlBytes, err := yaml.Marshal(resource.Object)
		if err != nil {
			return fmt.Errorf("failed to marshal resource %s/%s to YAML: %w",
				resource.GetNamespace(), resource.GetName(), err)
		}

		buf.Write(yamlBytes)
	}

	// Write to file
	if err := os.WriteFile(filename, buf.Bytes(), 0644); err != nil {
		return fmt.Errorf("failed to write resource type file %s: %w", filename, err)
	}

	return nil
}

// ReadResourceTypeFile reads a multi-doc YAML file and returns individual resources
// This is useful for reading output from previous stages in multi-stage pipeline
func ReadResourceTypeFile(filename string) ([]unstructured.Unstructured, error) {
	// Read file content
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to read file %s: %w", filename, err)
	}

	// Split by YAML document separator
	docs := splitYAMLDocuments(data)

	resources := make([]unstructured.Unstructured, 0, len(docs))

	for i, doc := range docs {
		// Skip empty documents
		if len(bytes.TrimSpace(doc)) == 0 {
			continue
		}

		// Parse YAML to unstructured
		var obj map[string]interface{}
		if err := yaml.Unmarshal(doc, &obj); err != nil {
			return nil, fmt.Errorf("failed to unmarshal document %d in %s: %w", i, filename, err)
		}

		// Skip nil or empty objects (e.g., YAML null or separator-only documents)
		if len(obj) == 0 {
			continue
		}

		resource := unstructured.Unstructured{Object: obj}
		resources = append(resources, resource)
	}

	return resources, nil
}

// splitYAMLDocuments splits a multi-doc YAML file into individual documents
func splitYAMLDocuments(data []byte) [][]byte {
	// Split by "---" separator
	separator := []byte("\n---\n")
	altSeparator := []byte("\n---")  // Handle end-of-file case
	altSeparator2 := []byte("---\n") // Handle start-of-file case

	var docs [][]byte
	remaining := data

	for len(remaining) > 0 {
		// Find next separator
		idx := bytes.Index(remaining, separator)
		matchedSeparator := separator

		if idx == -1 {
			// No more separators - check for alternative patterns
			idx = bytes.Index(remaining, altSeparator)
			if idx == -1 {
				// Check if it starts with separator
				if bytes.HasPrefix(remaining, altSeparator2) {
					remaining = remaining[4:] // Skip "---\n"
					continue
				}
				// This is the last document
				docs = append(docs, remaining)
				break
			}
			matchedSeparator = altSeparator
		}

		// Extract document before separator
		doc := remaining[:idx]
		if len(bytes.TrimSpace(doc)) > 0 {
			docs = append(docs, doc)
		}

		// Move past separator
		if idx+len(matchedSeparator) <= len(remaining) {
			remaining = remaining[idx+len(matchedSeparator):]
		} else {
			break
		}
	}

	return docs
}
