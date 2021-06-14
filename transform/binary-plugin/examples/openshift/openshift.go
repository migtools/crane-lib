package main

import (
	"encoding/json"
	"fmt"
	"strings"

	jsonpatch "github.com/evanphx/json-patch"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

var defaultPullSecrets = []string{"builder-dockercfg-", "default-dockercfg-", "deployer-dockercfg-"}

func UpdateDefaultPullSecrets(u unstructured.Unstructured) (jsonpatch.Patch, error) {
	pullSecrets := getPullSecrets(u)

	jsonPatch := jsonpatch.Patch{}

	for n, secret := range pullSecrets {
		if isDefault(secret.Name) {

			patchJSON := fmt.Sprintf(`[
{ "op": "remove", "path": "/spec/imagePullSecrets/%v"}
]`, n)

			patch, err := jsonpatch.DecodePatch([]byte(patchJSON))
			if err != nil {
				return nil, err
			}
			jsonPatch = append(jsonPatch, patch...)
		}
	}

	return jsonPatch, nil
}

func UpdateServiceAccount(u unstructured.Unstructured) (jsonpatch.Patch, error) {
	pullSecrets := getPullSecretReferencesServiceAccount(u)

	jsonPatch := jsonpatch.Patch{}

	check := u.GetName() + "-dockercfg-"

	for n, secret := range pullSecrets {
		if strings.HasPrefix(secret.Name, check) {

			patchJSON := fmt.Sprintf(`[
{ "op": "remove", "path": "/imagePullSecrets/%v"}
]`, n)
			patch, err := jsonpatch.DecodePatch([]byte(patchJSON))
			if err != nil {
				return nil, err
			}
			jsonPatch = append(jsonPatch, patch...)

		}
	}

	secrets := getSecretReferencesServiceAccount(u)
	for n, secret := range secrets {
		if strings.HasPrefix(secret.Name, check) {

			patchJSON := fmt.Sprintf(`[
{ "op": "remove", "path": "/secrets/%v"}
]`, n)
			patch, err := jsonpatch.DecodePatch([]byte(patchJSON))
			if err != nil {
				return nil, err
			}
			jsonPatch = append(jsonPatch, patch...)
		}
	}
	return jsonPatch, nil
}

func UpdateRoute(u unstructured.Unstructured) (jsonpatch.Patch, error) {
	patchJSON := fmt.Sprintf(`[
{ "op": "remove", "path": "/spec/host"}
]`)

	patch, err := jsonpatch.DecodePatch([]byte(patchJSON))
	if err != nil {
		return nil, err
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
