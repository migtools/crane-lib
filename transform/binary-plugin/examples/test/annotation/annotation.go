package main

import (
	"fmt"
	jsonpatch "github.com/evanphx/json-patch"
	"github.com/konveyor/crane-lib/transform"
	"github.com/konveyor/crane-lib/transform/cli"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func main() {
	u, err := cli.Unstructured(cli.ObjectReaderOrDie())
	if err != nil {
		cli.WriterErrorAndExit(fmt.Errorf("error getting unstructured object: %#v", err))
	}

	cli.RunAndExit(cli.NewCustomPlugin("WhiteoutPlugin", Run), u)
}

func Run(u *unstructured.Unstructured) (transform.PluginResponse, error) {
	// plugin writers need to write custome code here.
	patch, err := AddAnnotation(*u)

	if err != nil {
		return transform.PluginResponse{}, err
	}
	return transform.PluginResponse{
		Version:    "v1",
		IsWhiteOut: false,
		Patches:    patch,
	}, nil
}

func AddAnnotation(u unstructured.Unstructured) (jsonpatch.Patch, error) {
	patchJSON := fmt.Sprintf(`[
{ "op": "add", "path": "/metadata/annotations/test-crane-annotation", "value":"crane"}
]`)

	patch, err := jsonpatch.DecodePatch([]byte(patchJSON))
	if err != nil {
		return nil, err
	}
	return patch, nil
}
