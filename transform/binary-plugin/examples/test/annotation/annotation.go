package main

import (
	"fmt"

	jsonpatch "github.com/evanphx/json-patch"
	"github.com/konveyor/crane-lib/transform"
	"github.com/konveyor/crane-lib/transform/cli"
)

func main() {
	fields := []transform.OptionalFields{
		{
			FlagName: "annotation-value",
			Help:     "The value to set for the annotation test-crane-annotation",
			Example:  "foo",
		},
	}
	cli.RunAndExit(cli.NewCustomPlugin("AnnotationPlugin", "v1", fields, Run))
}

func Run(request transform.PluginRequest) (transform.PluginResponse, error) {
	// plugin writers need to write custome code here.
	patch, err := AddAnnotation(request)

	if err != nil {
		return transform.PluginResponse{}, err
	}
	return transform.PluginResponse{
		Version:    "v1",
		IsWhiteOut: false,
		Patches:    patch,
	}, nil
}

func AddAnnotation(request transform.PluginRequest) (jsonpatch.Patch, error) {
	val, ok := request.Extras["annotation-value"]
	if !ok {
		val = "crane"
	}
	patchJSON := fmt.Sprintf(`[
{ "op": "add", "path": "/metadata/annotations/test-crane-annotation", "value":"%v"}
]`, val)

	patch, err := jsonpatch.DecodePatch([]byte(patchJSON))
	if err != nil {
		return nil, err
	}
	return patch, nil
}
