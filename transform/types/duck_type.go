package types

import (
	"encoding/json"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func IsPodSpecable(u unstructured.Unstructured) (*v1.PodTemplateSpec, bool) {
	// Get Spec
	spec, ok := u.UnstructuredContent()["spec"]
	if !ok {
		return nil, false
	}

	specMap, ok := spec.(map[string]interface{})
	if !ok {
		return nil, false
	}

	// Is template apart of the spec
	templateInterface, ok := specMap["template"]
	if !ok {
		return nil, false
	}

	// does template marshal into PodTemplateSpec

	jsonTemplate, err := json.Marshal(templateInterface)
	if err != nil {
		return nil, false
	}

	template := v1.PodTemplateSpec{}

	err = json.Unmarshal(jsonTemplate, &template)
	if err != nil {
		return nil, false
	}

	return &template, true
}
