package main

import (
	"encoding/json"
	"fmt"
	"strings"

	jsonpatch "github.com/evanphx/json-patch"
	"github.com/konveyor/crane-lib/transform/internal/image"
	buildv1API "github.com/openshift/api/build/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

const (
	podReplaceImagePullSecret = `%v%v
{ "op": "replace", "path": "/spec/imagePullSecrets/%v/name", "value": "%v"}`
	podRemoveImagePullSecret = `%v%v
{ "op": "remove", "path": "/spec/imagePullSecrets/%v"}`
	serviceAccountRemoveSecret = `%v%v
{ "op": "remove", "path": "/secrets/%v"}`
	serviceAccountRemoveImagePullSecret = `%v%v
{ "op": "remove", "path": "/imagePullSecrets/%v"}`
	replaceSecretOp = `[
{ "op": "replace", "path": "%v", "value": "%v"}
]`
	removeSecretOp = `[
{ "op": "remove", "path": "%v"}
]`
	buildConfigOutputPushSecret         = "/spec/output/pushSecret"
	buildConfigOutputTo                 = "/spec/output/to/name"
	buildConfigSourceStrategyPullSecret = "/spec/strategy/sourceStrategy/pullSecret"
	buildConfigSourceStrategyFrom       = "/spec/strategy/sourceStrategy/from/name"
	buildConfigDockerStrategyPullSecret = "/spec/strategy/dockerStrategy/pullSecret"
	buildConfigDockerStrategyFrom       = "/spec/strategy/dockerStrategy/from/name"
	buildConfigCustomStrategyPullSecret = "/spec/strategy/customStrategy/pullSecret"
	buildConfigCustomStrategyFrom       = "/spec/strategy/customStrategy/from/name"
	buildConfigSourceImagesPullSecret   = "/spec/source/images/%v/pullSecret"
	buildConfigSourceImagesFrom         = "/spec/source/images/%v/from/name"

)

var defaultPullSecrets = []string{"builder-dockercfg-", "default-dockercfg-", "deployer-dockercfg-"}

func updateBuildConfigImageReference(
	imgRef v1.ObjectReference,
	imgPath string,
	fields openshiftOptionalFields,
) (jsonpatch.Patch, error) {
	patch := jsonpatch.Patch{}
	var err error
	if imgRef.Kind != "DockerImage" {
		return patch, nil
	}
	if fields.RegistryReplacement == nil || len(fields.RegistryReplacement) == 0 {
		return patch, nil
	}
	updatedImageRef, update := image.UpdateImageRegistry(fields.RegistryReplacement, imgRef.Name)
	if update {
		patch, err = image.UpdateImage(imgPath, updatedImageRef)
		if err != nil {
			return nil, err
		}
	}
	return patch, nil
}

func UpdateDefaultPullSecrets(u unstructured.Unstructured, fields openshiftOptionalFields) (jsonpatch.Patch, error) {
	return updateSecretsForSlice(getPullSecrets(u), podReplaceImagePullSecret, podRemoveImagePullSecret, fields)
}

func updateSecretsForSlice(
	pullSecrets []v1.LocalObjectReference,
	replaceOp string,
	removeOp string,
	fields openshiftOptionalFields) (jsonpatch.Patch, error) {
	var err error

	replacePatch := jsonpatch.Patch{}
	removePatch := jsonpatch.Patch{}
	replaceJSON := `[`
	removeJSON := `[`

	// iterate in reverse order since jsonpatch array element removal shifts
	// later elements up, which would break later remove patch entries otherwise
	var priorReplace, priorRemove bool
	for i := len(pullSecrets)-1; i >= 0; i-- {
		newSecret, ok := fields.PullSecretReplacement[pullSecrets[i].Name]
		// replacement found
		if ok {
			replaceJSON = fmt.Sprintf(
				replaceOp,
				replaceJSON,
				nonInitialDelimiter(priorReplace),
				i,
				newSecret,
			)
			priorReplace = true
		} else if fields.StripDefaultPullSecrets && isDefault(pullSecrets[i].Name) {
			removeJSON = fmt.Sprintf(
				removeOp,
				removeJSON,
				nonInitialDelimiter(priorRemove),
				i,
			)
			priorRemove = true
		}
	}
	replaceJSON = fmt.Sprintf("%v]", replaceJSON)
	removeJSON = fmt.Sprintf("%v]", removeJSON)


	if priorReplace {
		replacePatch, err = jsonpatch.DecodePatch([]byte(replaceJSON))
		if err != nil {
			return nil, err
		}
	}
	if priorRemove {
		removePatch, err = jsonpatch.DecodePatch([]byte(removeJSON))
		if err != nil {
			return nil, err
		}
	}
	return append(replacePatch, removePatch...), nil
}

func updateSecret(
	pullSecret *v1.LocalObjectReference,
	secretPath string,
	fields openshiftOptionalFields) (jsonpatch.Patch, error) {
	var err error

	patch := jsonpatch.Patch{}
	var patchJSON string

	if pullSecret == nil || len(pullSecret.Name) == 0  {
		return patch, nil
	}

	newSecret, ok := fields.PullSecretReplacement[pullSecret.Name]
	// replacement found
	if ok {
		patchJSON = fmt.Sprintf(replaceSecretOp, secretPath+"/name", newSecret)
	} else if fields.StripDefaultPullSecrets && isDefault(pullSecret.Name) {
		patchJSON = fmt.Sprintf(removeSecretOp, secretPath)
	}


	if len(patchJSON) > 0 {
		patch, err = jsonpatch.DecodePatch([]byte(patchJSON))
		if err != nil {
			return nil, err
		}
	}
	return patch, nil
}

func nonInitialDelimiter(priorEntries bool) string {
    if priorEntries {
        return ","
    } else {
        return ""
    }
}
func UpdateServiceAccount(u unstructured.Unstructured) (jsonpatch.Patch, error) {
	jsonPatch := jsonpatch.Patch{}
	check := u.GetName() + "-dockercfg-"
	var err error

	pullSecrets := getPullSecretReferencesServiceAccount(u)
	pullSecretsPatch := jsonpatch.Patch{}
	pullSecretsJSON := `[`
	var priorPullSecret bool
	for i := len(pullSecrets)-1; i >= 0; i-- {
		if strings.HasPrefix(pullSecrets[i].Name, check) {
			pullSecretsJSON = fmt.Sprintf(
				serviceAccountRemoveImagePullSecret,
				pullSecretsJSON,
				nonInitialDelimiter(priorPullSecret),
				i,
			)
			priorPullSecret = true
		}
	}
	pullSecretsJSON = fmt.Sprintf("%v]", pullSecretsJSON)
	if priorPullSecret {
		pullSecretsPatch, err = jsonpatch.DecodePatch([]byte(pullSecretsJSON))
		if err != nil {
			return jsonPatch, err
		}
		jsonPatch = append(jsonPatch, pullSecretsPatch...)
	}

	secrets := getSecretReferencesServiceAccount(u)
	secretsPatch := jsonpatch.Patch{}
	secretsJSON := `[`
	var priorSecret bool
	for i := len(secrets)-1; i >= 0; i-- {
		if strings.HasPrefix(secrets[i].Name, check) {
			secretsJSON = fmt.Sprintf(
				serviceAccountRemoveSecret,
				secretsJSON,
				nonInitialDelimiter(priorSecret),
				i,
			)
			priorSecret = true
		}
	}
	secretsJSON = fmt.Sprintf("%v]", secretsJSON)
	if priorSecret {
		secretsPatch, err = jsonpatch.DecodePatch([]byte(secretsJSON))
		if err != nil {
			return jsonPatch, err
		}
		jsonPatch = append(jsonPatch, secretsPatch...)
	}

	return jsonPatch, nil
}

func UpdateRoute(u unstructured.Unstructured) (jsonpatch.Patch, error) {
	var patch jsonpatch.Patch
	var err error
	annotations := u.GetAnnotations()
	if annotations != nil && annotations["openshift.io/host.generated"] == "true" {
		patchJSON := fmt.Sprintf(`[
{ "op": "remove", "path": "/spec/host"}
]`)

		patch, err = jsonpatch.DecodePatch([]byte(patchJSON))
		if err != nil {
			return nil, err
		}
	}
	return patch, nil
}

func isDefault(name string) bool {
	for _, d := range defaultPullSecrets {
		if strings.Contains(name, d) {
			return true
		}
	}
	return false
}

func UpdateBuildConfig(u unstructured.Unstructured, fields openshiftOptionalFields) (jsonpatch.Patch, error) {
	jsonPatch := jsonpatch.Patch{}
	js, err := u.MarshalJSON()
	if err != nil {
		return jsonPatch, err
	}

	buildConfig := &buildv1API.BuildConfig{}

	err = json.Unmarshal(js, buildConfig)
	if err != nil {
		return jsonPatch, err
	}
	patch, err := updateSecret(buildConfig.Spec.Output.PushSecret, buildConfigOutputPushSecret, fields)
	if err != nil {
		return nil, err
	}
	jsonPatch = append(jsonPatch, patch...)
	if buildConfig.Spec.Output.To != nil {
		patch, err := updateBuildConfigImageReference(*buildConfig.Spec.Output.To, buildConfigOutputTo, fields)
		if err != nil {
			return jsonPatch, err
		}
		jsonPatch = append(jsonPatch, patch...)
	}

	if buildConfig.Spec.Strategy.SourceStrategy != nil {
		patch, err := updateSecret(buildConfig.Spec.Strategy.SourceStrategy.PullSecret, buildConfigSourceStrategyPullSecret, fields)
		if err != nil {
			return nil, err
		}
		jsonPatch = append(jsonPatch, patch...)
		patch, err = updateBuildConfigImageReference(buildConfig.Spec.Strategy.SourceStrategy.From, buildConfigSourceStrategyFrom, fields)
		if err != nil {
			return jsonPatch, err
		}
		jsonPatch = append(jsonPatch, patch...)
	}

	if buildConfig.Spec.Strategy.DockerStrategy != nil {
		patch, err := updateSecret(buildConfig.Spec.Strategy.DockerStrategy.PullSecret, buildConfigDockerStrategyPullSecret, fields)
		if err != nil {
			return nil, err
		}
		if buildConfig.Spec.Strategy.DockerStrategy != nil {
			jsonPatch = append(jsonPatch, patch...)
			patch, err := updateBuildConfigImageReference(*buildConfig.Spec.Strategy.DockerStrategy.From, buildConfigDockerStrategyFrom, fields)
			if err != nil {
				return jsonPatch, err
			}
			jsonPatch = append(jsonPatch, patch...)
		}
	}

	if buildConfig.Spec.Strategy.CustomStrategy != nil {
		patch, err := updateSecret(buildConfig.Spec.Strategy.CustomStrategy.PullSecret, buildConfigCustomStrategyPullSecret, fields)
		if err != nil {
			return nil, err
		}
		jsonPatch = append(jsonPatch, patch...)
		patch, err = updateBuildConfigImageReference(buildConfig.Spec.Strategy.CustomStrategy.From, buildConfigCustomStrategyFrom, fields)
		if err != nil {
			return jsonPatch, err
		}
		jsonPatch = append(jsonPatch, patch...)
	}

	if buildConfig.Spec.Source.Images != nil {
		for i, imageSource := range buildConfig.Spec.Source.Images {
			patch, err := updateSecret(imageSource.PullSecret, fmt.Sprintf(buildConfigSourceImagesPullSecret, i), fields)
			if err != nil {
				return nil, err
			}
			jsonPatch = append(jsonPatch, patch...)
			patch, err = updateBuildConfigImageReference(imageSource.From, fmt.Sprintf(buildConfigSourceImagesFrom, i), fields)
			if err != nil {
				return jsonPatch, err
			}
			jsonPatch = append(jsonPatch, patch...)
		}
	}
	return jsonPatch, nil
}

func getPullSecrets(u unstructured.Unstructured) []v1.LocalObjectReference {
	js, err := u.MarshalJSON()
	if err != nil {
		return nil
	}

	pod := &v1.Pod{}

	err = json.Unmarshal(js, pod)
	if err != nil {
		return nil
	}

	return pod.Spec.ImagePullSecrets
}

func getPullSecretReferencesServiceAccount(u unstructured.Unstructured) []v1.LocalObjectReference {
	js, err := u.MarshalJSON()
	if err != nil {
		return nil
	}

	sa := &v1.ServiceAccount{}

	err = json.Unmarshal(js, sa)
	if err != nil {
		return nil
	}

	return sa.ImagePullSecrets
}

func getSecretReferencesServiceAccount(u unstructured.Unstructured) []v1.ObjectReference {
	js, err := u.MarshalJSON()
	if err != nil {
		return nil
	}

	sa := &v1.ServiceAccount{}

	err = json.Unmarshal(js, sa)
	if err != nil {
		return nil
	}

	return sa.Secrets
}
