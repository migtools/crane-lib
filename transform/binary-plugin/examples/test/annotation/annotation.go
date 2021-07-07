package main

import (
	"fmt"

	jsonpatch "github.com/evanphx/json-patch"
	"github.com/konveyor/crane-lib/transform"
	"github.com/konveyor/crane-lib/transform/cli"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
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

func Run(u *unstructured.Unstructured, extras map[string]string) (transform.PluginResponse, error) {
	// plugin writers need to write custome code here.
	patch, err := AddAnnotation(*u, extras)

	if err != nil {
		return transform.PluginResponse{}, err
	}
	return transform.PluginResponse{
		Version:    "v1",
		IsWhiteOut: false,
		Patches:    patch,
	}, nil
}

func AddAnnotation(u unstructured.Unstructured, extras map[string]string) (jsonpatch.Patch, error) {
	val, ok := extras["annotation-value"]
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
