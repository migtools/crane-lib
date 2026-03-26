package transform

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
)

// WhiteoutReport represents a resource that was excluded from the output
// due to plugin whiteout decision
type WhiteoutReport struct {
	APIVersion  string   `json:"apiVersion"`
	Kind        string   `json:"kind"`
	Name        string   `json:"name"`
	Namespace   string   `json:"namespace,omitempty"`
	RequestedBy []string `json:"requestedBy"`
}

// IgnoredPatchReport represents a patch operation that was discarded
// due to conflicts with higher priority plugins
type IgnoredPatchReport struct {
	Resource       ResourceIdentity `json:"resource"`
	Path           string           `json:"path"`
	Operation      string           `json:"operation"`
	SelectedPlugin string           `json:"selectedPlugin"`
	IgnoredPlugin  string           `json:"ignoredPlugin"`
	Reason         string           `json:"reason"`
}

// ResourceIdentity uniquely identifies a Kubernetes resource
type ResourceIdentity struct {
	APIVersion string `json:"apiVersion"`
	Kind       string `json:"kind"`
	Name       string `json:"name"`
	Namespace  string `json:"namespace,omitempty"`
}

// GenerateWhiteoutReport creates a JSON report of whiteouted resources
func GenerateWhiteoutReport(whiteouts []WhiteoutReport) ([]byte, error) {
	// Sort for determinism
	SortWhiteouts(whiteouts)

	// Marshal to JSON with indentation for readability
	jsonBytes, err := json.MarshalIndent(whiteouts, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to marshal whiteout report: %w", err)
	}

	return jsonBytes, nil
}

// GenerateIgnoredPatchReport creates a JSON report of ignored patches
func GenerateIgnoredPatchReport(ignored []IgnoredPatchReport) ([]byte, error) {
	// Sort for determinism
	SortIgnoredPatches(ignored)

	// Marshal to JSON with indentation for readability
	jsonBytes, err := json.MarshalIndent(ignored, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to marshal ignored patch report: %w", err)
	}

	return jsonBytes, nil
}

// SortWhiteouts sorts whiteout reports for deterministic output
// Sort order: namespace -> kind -> name
func SortWhiteouts(whiteouts []WhiteoutReport) {
	sort.Slice(whiteouts, func(i, j int) bool {
		// First by namespace
		if whiteouts[i].Namespace != whiteouts[j].Namespace {
			return whiteouts[i].Namespace < whiteouts[j].Namespace
		}

		// Then by kind
		if whiteouts[i].Kind != whiteouts[j].Kind {
			return whiteouts[i].Kind < whiteouts[j].Kind
		}

		// Finally by name
		return whiteouts[i].Name < whiteouts[j].Name
	})
}

// SortIgnoredPatches sorts ignored patch reports for deterministic output
// Sort order: resource namespace -> resource kind -> resource name -> path
func SortIgnoredPatches(reports []IgnoredPatchReport) {
	sort.Slice(reports, func(i, j int) bool {
		// First by resource namespace
		if reports[i].Resource.Namespace != reports[j].Resource.Namespace {
			return reports[i].Resource.Namespace < reports[j].Resource.Namespace
		}

		// Then by resource kind
		if reports[i].Resource.Kind != reports[j].Resource.Kind {
			return reports[i].Resource.Kind < reports[j].Resource.Kind
		}

		// Then by resource name
		if reports[i].Resource.Name != reports[j].Resource.Name {
			return reports[i].Resource.Name < reports[j].Resource.Name
		}

		// Finally by path
		return reports[i].Path < reports[j].Path
	})
}

// WriteWhiteoutReport writes whiteout report to a file
func WriteWhiteoutReport(filename string, whiteouts []WhiteoutReport) error {
	jsonBytes, err := GenerateWhiteoutReport(whiteouts)
	if err != nil {
		return err
	}

	if err := os.WriteFile(filename, jsonBytes, 0644); err != nil {
		return fmt.Errorf("failed to write whiteout report to %s: %w", filename, err)
	}

	return nil
}

// WriteIgnoredPatchReport writes ignored patch report to a file
func WriteIgnoredPatchReport(filename string, ignored []IgnoredPatchReport) error {
	jsonBytes, err := GenerateIgnoredPatchReport(ignored)
	if err != nil {
		return err
	}

	if err := os.WriteFile(filename, jsonBytes, 0644); err != nil {
		return fmt.Errorf("failed to write ignored patch report to %s: %w", filename, err)
	}

	return nil
}

// ReadWhiteoutReport reads whiteout report from a file
func ReadWhiteoutReport(filename string) ([]WhiteoutReport, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to read whiteout report from %s: %w", filename, err)
	}

	var whiteouts []WhiteoutReport
	if err := json.Unmarshal(data, &whiteouts); err != nil {
		return nil, fmt.Errorf("failed to unmarshal whiteout report: %w", err)
	}

	return whiteouts, nil
}

// ReadIgnoredPatchReport reads ignored patch report from a file
func ReadIgnoredPatchReport(filename string) ([]IgnoredPatchReport, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to read ignored patch report from %s: %w", filename, err)
	}

	var ignored []IgnoredPatchReport
	if err := json.Unmarshal(data, &ignored); err != nil {
		return nil, fmt.Errorf("failed to unmarshal ignored patch report: %w", err)
	}

	return ignored, nil
}
