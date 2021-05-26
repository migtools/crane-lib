package kubernetes

import (
	"encoding/json"
	"fmt"
	"strings"

	jsonpatch "github.com/evanphx/json-patch"
	"github.com/konveyor/crane-lib/transforms/types"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

var endpointGK = schema.GroupKind{
	Group: "",
	Kind:  "Endpoints",
}

var endpointSliceGK = schema.GroupKind{
	Group: "discovery.k8s.io",
	Kind:  "EndpointSlice",
}

var pvcGK = schema.GroupKind{
	Group: "",
	Kind:  "PersistentVolumeClaim",
}

var podGK = schema.GroupKind{
	Group: "",
	Kind:  "Pod",
}

var serviceGK = schema.GroupKind{
	Group: "",
	Kind:  "Service",
}

func GetWhiteOuts(groupKind schema.GroupKind) bool {
	if groupKind == endpointGK {
		return true
	}

	if groupKind == endpointSliceGK {
		return true
	}

	// For right now we assume PVC's are handled by a different part
	// of the tool chain.
	if groupKind == pvcGK {
		return true
	}
	return false
}

func GetKubernetesTransforms(obj unstructured.Unstructured,
	addedAnnotations map[string]string,
	registryReplacement map[string]string,
	newNamespace string,
	removeAnnotation []string) ([]byte, error) {

	// Always attempt to add annotations for each thing.
	jsonPatch := jsonpatch.Patch{}
	if len(addedAnnotations) > 0 {
		patches, err := addAnnotations(addedAnnotations)
		if err != nil {
			return nil, err
		}
		jsonPatch = append(jsonPatch, patches...)
	}
	if podGK == obj.GetObjectKind().GroupVersionKind().GroupKind() {
		patches, err := removePodSelectedNode()
		if err != nil {
			return nil, err
		}
		jsonPatch = append(jsonPatch, patches...)
	}
	if len(registryReplacement) > 0 {
		if podGK == obj.GetObjectKind().GroupVersionKind().GroupKind() {
			// jsonPatch for return
		} else if template, ok := types.IsPodSpecable(obj); ok {
			jps := jsonpatch.Patch{}
			for i, container := range template.Spec.Containers {
				updatedImage, update := updateImageRegistry(registryReplacement, container.Image)
				if update {
					jp, err := updateImage(fmt.Sprintf("/spec/template/spec/containers/%v/image", i), updatedImage)
					if err != nil {
						return nil, err
					}
					jps = append(jps, jp...)
				}
			}
			for i, container := range template.Spec.InitContainers {
				updatedImage, update := updateImageRegistry(registryReplacement, container.Image)
				if update {
					jp, err := updateImage(fmt.Sprintf("/spec/template/spec/initContainers/%v/image", i), updatedImage)
					if err != nil {
						return nil, err
					}
					jps = append(jps, jp...)
				}
			}
		}
	}
	if obj.GetObjectKind().GroupVersionKind().GroupKind() == serviceGK {
		patches, err := removeServiceClusterIPs()
		if err != nil {
			return nil, err
		}
		jsonPatch = append(jsonPatch, patches...)
	}

	return json.Marshal(jsonPatch)
}

func updateImageRegistry(registryReplacements map[string]string, oldImageName string) (string, bool) {
	// Break up oldImage to get the registry URL. Assume all manifests are using fully qualified image paths, if not ignore.
	imageParts := strings.Split(oldImageName, "/")
	if len(imageParts) != 3 {
		return "", false
	}
	if newRegistry, ok := registryReplacements[imageParts[0]]; ok {
		return strings.Join([]string{newRegistry, imageParts[1], imageParts[2]}, "/"), true
	}

	return "", false
}

func addAnnotations(addedAnnotations map[string]string) (jsonpatch.Patch, error) {
	patchJSON := `[`
	i := 0
	for key, value := range addedAnnotations {
		if i == 0 {
			patchJSON = fmt.Sprintf(`%v
{"op", "add", "path": "/metadata/annotations/%v", "value": "%v"}`, patchJSON, key, value)
		} else {
			patchJSON = fmt.Sprintf(`%v,
{"op", "add", "path": "/metadata/annotations/%v", "value": "%v"}`, patchJSON, key, value)
		}
		i++
	}

	patchJSON = fmt.Sprintf("%v]", patchJSON)
	patch, err := jsonpatch.DecodePatch([]byte(patchJSON))
	if err != nil {
		return nil, err
	}
	return patch, nil
}

func updateImage(containerImagePath, updatedImagePath string) (jsonpatch.Patch, error) {
	patchJSON := fmt.Sprintf(`[
{"op": "replace", "path": "%v", "value": "%v"}
]`, containerImagePath, updatedImagePath)

	patch, err := jsonpatch.DecodePatch([]byte(patchJSON))
	if err != nil {
		return nil, err
	}
	return patch, nil
}

func removePodSelectedNode() (jsonpatch.Patch, error) {
	patchJSON := fmt.Sprintf(`[
{"op": "remove", "path": "/spec/nodeName"}
]`)
	patch, err := jsonpatch.DecodePatch([]byte(patchJSON))
	if err != nil {
		return nil, err
	}
	return patch, nil
}

func updateNamespace(newNamespace string) (jsonpatch.Patch, error) {
	patchJSON := fmt.Sprintf(`[
{"op": "replace", "path": "/namespace", "value": "%v"}
]`, newNamespace)

	patch, err := jsonpatch.DecodePatch([]byte(patchJSON))
	if err != nil {
		return nil, err
	}
	return patch, nil
}

func updateRoleBindingSVCACCTNamespace(newNamespace string, numberOfSubjects int) (jsonpatch.Patch, error) {
	patchJSON := "["
	for i := 0; i < numberOfSubjects; i++ {
		if i != 0 {
			patchJSON = fmt.Sprintf("%v,", patchJSON)
		}
		patchJSON = fmt.Sprintf(`%v
{"op": "replace", "path": "/subjects/%v/namespace", "value": "%v"}`, patchJSON, i, newNamespace)
	}

	patch, err := jsonpatch.DecodePatch([]byte(patchJSON))
	if err != nil {
		return nil, err
	}
	return patch, nil
}

func removeServiceClusterIPs() (jsonpatch.Patch, error) {
	patchJSON := fmt.Sprintf(`[
{"op": "remove", "path": "/spec/clusterIP"}
]`)
	patch, err := jsonpatch.DecodePatch([]byte(patchJSON))
	if err != nil {
		return nil, err
	}
	return patch, nil
}
