package transform

import (
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

type Plugin interface {
	// Determine for a given resources what the plugin is deciding to do with this
	RunTransform(unstructured.Unstructured) (PluginResponse, error)
}

type PluginResponse struct {
	Version    string `json:"version,omitempty"`
	IsWhiteOut bool   `json:"is_white_out,omitempty"`
	Patches    []byte `json:"patches,omitempty"`
}
