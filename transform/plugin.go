package transform

import (
	jsonpatch "github.com/evanphx/json-patch"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

type PluginRun interface {
	// Determine for a given resources what the plugin is deciding to do with this
	Run(*unstructured.Unstructured, map[string]string) (PluginResponse, error)
}

type Metadata interface {
	Metadata() PluginMetadata
}

type Plugin interface {
	PluginRun
	Metadata
}

type PluginResponse struct {
	Version    string          `json:"version,omitempty"`
	IsWhiteOut bool            `json:"isWhiteOut,omitempty"`
	Patches    jsonpatch.Patch `json:"patches,omitempty"`
}

type PluginMetadata struct {
	Name            string    `json:"name"`
	Version         string    `json:"version"`
	RequestVersion  []Version `json:"requestVersion"`
	ResponseVersion []Version `json:"responseVersion"`
	OptionalFields  []string  `json:"optionalFields,omitempty"`
}

type Version string

const (
	V1 Version = "v1"
)

const (
	// Metadata string is the constant string that will be used by the binary-pluigin helper and the cli helpers
	// To notice that
	MetadataString string = "METADATA"
)
