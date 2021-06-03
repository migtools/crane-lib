package transform

import (
	jsonpatch "github.com/evanphx/json-patch"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

type Plugin interface {
	// Determine for a given resources what the plugin is deciding to do with this
	Run(*unstructured.Unstructured) (PluginResponse, error)
}

type PluginResponse struct {
	Version    string          `json:"version,omitempty"`
	IsWhiteOut bool            `json:"isWhiteOut,omitempty"`
	Patches    jsonpatch.Patch `json:"patches,omitempty"`
}
