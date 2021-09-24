package main

import (
	"strconv"

	jsonpatch "github.com/evanphx/json-patch"
	"github.com/konveyor/crane-lib/transform"
	"github.com/konveyor/crane-lib/transform/cli"
	"github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

var logger logrus.FieldLogger

func main() {
	logger = logrus.New()
	// TODO: add plumbing for logger in the cli-library and instantiate here
	fields := []transform.OptionalFields{
                {
                        FlagName: "StripDefaultPullSecrets",
                        Help:     "Whether to strip Pod and BuildConfig default pull secrets (beginning with builder/default/deployer-dockercfg-) that aren't replaced by the map param PullSecretReplacement",
                        Example:  "true",
                },
                {
                        FlagName: "PullSecretReplacement",
                        Help:     "Map of pull secrets to replace in Pods and BuildConfigs while transforming in format secret1=destsecret1,secret2=destsecret2[...]",
			Example:  "default-dockercfg-h4n7g=default-dockercfg-12345,builder-dockercfg-abcde=builder-dockercfg-12345",
                },
		{
			FlagName: "RegistryReplacement",
			Help:     "Map of image registry paths to swap on transform, in the format original-registry1=target-registry1,original-registry2=target-registry2...",
			Example:  "docker-registry.default.svc:5000=image-registry.openshift-image-registry.svc:5000,docker.io/foo=quay.io/bar",
		},
        }
	cli.RunAndExit(cli.NewCustomPlugin("OpenshiftPlugin", "v1", fields, Run))
}

type openshiftOptionalFields struct {
	StripDefaultPullSecrets bool
	PullSecretReplacement   map[string]string
	RegistryReplacement     map[string]string
}

func getOptionalFields(extras map[string]string) (openshiftOptionalFields, error) {
	var fields openshiftOptionalFields
	var err error
	if len(extras["StripDefaultPullSecrets"]) > 0 {
		fields.StripDefaultPullSecrets, err = strconv.ParseBool(extras["StripDefaultPullSecrets"])
		if err != nil {
			return fields, err
		}
	}
	if len(extras["PullSecretReplacement"]) > 0 {
		fields.PullSecretReplacement = transform.ParseOptionalFieldMapVal(extras["PullSecretReplacement"])
	}
	if len(extras["RegistryReplacement"]) > 0 {
		fields.RegistryReplacement = transform.ParseOptionalFieldMapVal(extras["RegistryReplacement"])
	}
	return fields, nil
}

func Run(u *unstructured.Unstructured, extras map[string]string) (transform.PluginResponse, error) {
	var patch jsonpatch.Patch
	whiteOut := false
	inputFields, err := getOptionalFields(extras)
	if err != nil {
		return transform.PluginResponse{}, err
	}

	switch u.GetKind() {
	case "Build":
		logger.Info("found build, adding to whiteout")
		whiteOut = true
	case "BuildConfig":
		logger.Info("found build config, processing")
		patch, err = UpdateBuildConfig(*u, inputFields)
	case "Pod":
		logger.Info("found pod, processing update default pull secret")
		patch, err = UpdateDefaultPullSecrets(*u, inputFields)
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
		IsWhiteOut: whiteOut,
		Patches:    patch,
	}, nil
}
