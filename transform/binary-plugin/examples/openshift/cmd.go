package main

import (
	"fmt"

	jsonpatch "github.com/evanphx/json-patch"
	"github.com/konveyor/crane-lib/transform"
	"github.com/konveyor/crane-lib/transform/cli"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func main() {
	// TODO: add plumbing for logger in the cli-library and instantiate here
	// TODO: add plumbing for passing flags in the cli-library
	u, err := cli.Unstructured(cli.ObjectReaderOrDie())
	if err != nil {
		cli.WriterErrorAndExit(fmt.Errorf("error getting unstructured object: %s", err))
	}

	cli.RunAndExit(cli.NewCustomPlugin("OpenshiftCustomPlugin", Run), u)
}

func Run(u *unstructured.Unstructured) (transform.PluginResponse, error) {
	// plugin writers need to write custom code here.
	var patch jsonpatch.Patch
	var err error
	switch u.GetKind() {
	case "Pod":
		patch, err = UpdateDefaultPullSecrets(*u)
	case "Route":
		patch, err = UpdateRoute(*u)
	case "ServiceAccount":
		patch, err = UpdateServiceAccount(*u)
	}
	if err != nil {
		return transform.PluginResponse{}, err
	}
	return transform.PluginResponse{
		Version:    "v1",
		IsWhiteOut: false,
		Patches:    patch,
	}, nil
}
