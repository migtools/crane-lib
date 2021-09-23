package transform

import (
	"strings"

	jsonpatch "github.com/evanphx/json-patch"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

type PluginRun interface {
	// Determine for a given resources what the plugin is deciding to do with this
	Run(PluginRequest) (PluginResponse, error)
}

type Metadata interface {
	Metadata() PluginMetadata
}

type Plugin interface {
	PluginRun
	Metadata
}

type PluginRequest struct {
	unstructured.Unstructured `json:",inline"`
	Extras map[string]string  `json:"extras,omitempty"`
}

type PluginResponse struct {
	Version    string          `json:"version,omitempty"`
	IsWhiteOut bool            `json:"isWhiteOut,omitempty"`
	Patches    jsonpatch.Patch `json:"patches,omitempty"`
}

type PluginMetadata struct {
	Name            string           `json:"name"`
	Version         string           `json:"version"`
	RequestVersion  []Version        `json:"requestVersion"`
	ResponseVersion []Version        `json:"responseVersion"`
	OptionalFields  []OptionalFields `json:"optionalFields,omitempty"`
}

type Version string

type OptionalFields struct {
	FlagName string `json:"flagName"`
	Help     string `json:"help"`
	Example  string `json:"example"`
}

const (
	V1 Version = "v1"
)

const (
	RequestVersion  = V1
	ResponseVersion = V1
)

const (
	// Metadata string is the constant string that will be used by the binary-pluigin helper and the cli helpers
	// To notice that
	MetadataString string = "METADATA"
)

func ParseOptionalFieldSliceVal(sliceVal string) []string {
	return strings.Split(sliceVal,",")
}

func ParseOptionalFieldMapVal(sliceVal string) map[string]string {
	mapVal := make(map[string]string)
	kvPairs := strings.Split(sliceVal,",")
	for _, kvPair := range kvPairs {
		kvSlice := strings.Split(kvPair,"=")
		if len(kvSlice) == 1 {
			mapVal[kvSlice[0]] = ""
		} else {
			mapVal[kvSlice[0]] = kvSlice[1]
		}
	}
	return mapVal
}
