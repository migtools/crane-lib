package main

import (
	jsonpatch "github.com/evanphx/json-patch"
	"github.com/konveyor/crane-lib/transform"
	"github.com/konveyor/crane-lib/transform/cli"
	"github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

var logger logrus.FieldLogger

func main() {
	// TODO: add plumbing for logger in the cli-library and instantiate here
	// TODO: add plumbing for passing flags in the cli-library
	cli.RunAndExit(cli.NewCustomPlugin("OpenshiftCustomPlugin", "V1", nil, Run))
}

func Run(u *unstructured.Unstructured, extras map[string]string) (transform.PluginResponse, error) {
	// plugin writers need to write custom code here.
	var patch jsonpatch.Patch
	var err error
	switch u.GetKind() {
	case "Pod":
		logger.Info("found pod, processing update default pull secret")
		patch, err = UpdateDefaultPullSecrets(*u)
	case "Route":
		logger.Info("found route, processing")
		patch, err = UpdateRoute(*u)
	case "ServiceAccount":
		logger.Info("found service account, processing")
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
