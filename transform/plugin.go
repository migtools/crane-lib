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
	Metadata() (PluginMetadata, error)
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
	Name            string
	Version         string
	RequestVersion  []Version
	ResponseVersion []Version
	OptionalFields  []string
}

type Version string

const (
	V1 Version = "v1"
)

const (
	MetadataStdIn string = "METADATA"
)
