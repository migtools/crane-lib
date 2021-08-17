package kubernetes

import (
	"encoding/json"
	"fmt"
	"strconv"

	jsonpatch "github.com/evanphx/json-patch"
	transform "github.com/konveyor/crane-lib/transform"
	"github.com/konveyor/crane-lib/transform/internal/image"
	"github.com/konveyor/crane-lib/transform/types"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/sets"
)

const (
	containerImageUpdate        = "/spec/template/spec/containers/%v/image"
	initContainerImageUpdate    = "/spec/template/spec/initContainers/%v/image"
	podContainerImageUpdate     = "/spec/containers/%v/image"
	podInitContainerImageUpdate = "/spec/initContainers/%v/image"
	annotationInitial           = `%v
{"op": "add", "path": "/metadata/annotations/%v", "value": "%v"}`
	annotationNext = `%v,
{"op": "add", "path": "/metadata/annotations/%v", "value": "%v"}`
	removeAnnotationInitial = `%v
{"op": "remove", "path": "/metadata/annotations/%v"}`
	removeAnnotationNext = `%v,
{"op": "remove", "path": "/metadata/annotations/%v"}`
	podNodeName = `[
{"op": "remove", "path": "/spec/nodeName"}
]`

	podNodeSelector = `[
{"op": "remove", "path": "/spec/nodeSelector"}
]`

	podPriority = `[
{"op": "remove", "path": "/spec/priority"}
]`

	updateNamespaceString = `[
{"op": "replace", "path": "/metadata/namespace", "value": "%v"}
]`

	updateRoleBindingSVCACCTNamspacestring = `%v
{"op": "replace", "path": "/subjects/%v/namespace", "value": "%v"}`

	updateClusterIP = `[
{"op": "remove", "path": "/spec/clusterIP"}
]`
	updateClusterIPs = `[
{"op": "remove", "path": "/spec/clusterIPs"}
]`
	updateExternalIPs = `[
{"op": "remove", "path": "/spec/externalIPs"}
]`
	updateNodePortString = `[
{"op": "remove", "path": "/spec/ports/%v/nodePort"}
]`
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

type KubernetesTransformPlugin struct {
	AddAnnotations      map[string]string
	RemoveAnnotations   []string
	RegistryReplacement map[string]string
	NewNamespace        string
}

func (k KubernetesTransformPlugin) setOptionalFields(extras map[string]string) {
	k.NewNamespace = extras["NewNamespace"]
	if len(extras["AddAnnotations"]) > 0 {
		k.AddAnnotations = transform.ParseOptionalFieldMapVal(extras["AddAnnotations"])
	}
	if len(extras["RemoveAnnotations"]) > 0 {
		k.RemoveAnnotations = transform.ParseOptionalFieldSliceVal(extras["RemoveAnnotations"])
	}
	if len(extras["RegistryReplacement"]) > 0 {
		k.RegistryReplacement = transform.ParseOptionalFieldMapVal(extras["RegistryReplacement"])
	}
}

func (k KubernetesTransformPlugin) Run(u *unstructured.Unstructured, extras map[string]string) (transform.PluginResponse, error) {
	k.setOptionalFields(extras)
	resp := transform.PluginResponse{}
	// Set version in the future
	resp.Version = string(transform.V1)
	var err error
	resp.IsWhiteOut = k.getWhiteOuts(*u)
	if resp.IsWhiteOut {
		return resp, err
	}
	resp.Patches, err = k.getKubernetesTransforms(*u)
	return resp, err

}

func (k KubernetesTransformPlugin) Metadata() transform.PluginMetadata {
	return transform.PluginMetadata{
		Name:            "KubernetesPlugin",
		Version:         "v1",
		RequestVersion:  []transform.Version{transform.V1},
		ResponseVersion: []transform.Version{transform.V1},
		OptionalFields:  []transform.OptionalFields{
			{
				FlagName: "AddAnnotations",
				Help:     "Annotations to add to each resource",
				Example:  "annotation1=value1,annotation2=value2",
			},
			{
				FlagName: "RegistryReplacement",
				Help:     "Map of image registry paths to swap on transform, in the format original-registry1=target-registry1,original-registry2=target-registry2...",
				Example:  "docker-registry.default.svc:5000=image-registry.openshift-image-registry.svc:5000,docker.io/foo=quay.io/bar",
			},
			{
				FlagName: "NewNamespace",
				Help:     "Change the resource namespace to NewNamespace",
				Example:  "destination-namespace",
			},
			{
				FlagName: "RemoveAnnotations",
				Help:     "Annotations to remove",
				Example:  "annotation1,annotation2",
			},
		},
	}
}

var _ transform.Plugin = &KubernetesTransformPlugin{}

func (k KubernetesTransformPlugin) getWhiteOuts(obj unstructured.Unstructured) bool {
	groupKind := obj.GroupVersionKind().GroupKind()
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
	_, isPodSpecable := types.IsPodSpecable(obj)
	if (groupKind == podGK || isPodSpecable) && len(obj.GetOwnerReferences()) > 0 {
		return true
	}
	return false
}

func (k KubernetesTransformPlugin) getKubernetesTransforms(obj unstructured.Unstructured) (jsonpatch.Patch, error) {

	// Always attempt to add annotations for each thing.
	jsonPatch := jsonpatch.Patch{}
	if k.AddAnnotations != nil && len(k.AddAnnotations) > 0 {
		patches, err := addAnnotations(k.AddAnnotations)
		if err != nil {
			return nil, err
		}
		jsonPatch = append(jsonPatch, patches...)
	}
	if len(k.RemoveAnnotations) > 0 {
		patches, err := removeAnnotations(k.RemoveAnnotations)
		if err != nil {
			return nil, err
		}
		jsonPatch = append(jsonPatch, patches...)
	}
	if len(k.NewNamespace) > 0 {
		patches, err := updateNamespace(k.NewNamespace)
		if err != nil {
			return nil, err
		}
		jsonPatch = append(jsonPatch, patches...)
	}
	if podGK == obj.GetObjectKind().GroupVersionKind().GroupKind() {
		patches, err := removePodFields()
		if err != nil {
			return nil, err
		}
		jsonPatch = append(jsonPatch, patches...)
	}
	if k.RegistryReplacement != nil && len(k.RegistryReplacement) > 0 {
		if podGK == obj.GetObjectKind().GroupVersionKind().GroupKind() {
			js, err := obj.MarshalJSON()
			if err != nil {
				return nil, err
			}
			pod := &v1.Pod{}
			err = json.Unmarshal(js, pod)
			if err != nil {
				return nil, err
			}
			jps := jsonpatch.Patch{}
			for i, container := range pod.Spec.Containers {
				updatedImage, update := image.UpdateImageRegistry(k.RegistryReplacement, container.Image)
				if update {
					jp, err := image.UpdateImage(fmt.Sprintf(podContainerImageUpdate, i), updatedImage)
					if err != nil {
						return nil, err
					}
					jps = append(jps, jp...)
				}
			}
			for i, container := range pod.Spec.InitContainers {
				updatedImage, update := image.UpdateImageRegistry(k.RegistryReplacement, container.Image)
				if update {
					jp, err := image.UpdateImage(fmt.Sprintf(podInitContainerImageUpdate, i), updatedImage)
					if err != nil {
						return nil, err
					}
					jps = append(jps, jp...)
				}
			}
			jsonPatch = append(jsonPatch, jps...)
		} else if template, ok := types.IsPodSpecable(obj); ok {
			jps := jsonpatch.Patch{}
			for i, container := range template.Spec.Containers {
				updatedImage, update := image.UpdateImageRegistry(k.RegistryReplacement, container.Image)
				if update {
					jp, err := image.UpdateImage(fmt.Sprintf(containerImageUpdate, i), updatedImage)
					if err != nil {
						return nil, err
					}
					jps = append(jps, jp...)
				}
			}
			for i, container := range template.Spec.InitContainers {
				updatedImage, update := image.UpdateImageRegistry(k.RegistryReplacement, container.Image)
				if update {
					jp, err := image.UpdateImage(fmt.Sprintf(initContainerImageUpdate, i), updatedImage)
					if err != nil {
						return nil, err
					}
					jps = append(jps, jp...)
				}
			}
			jsonPatch = append(jsonPatch, jps...)
		}
	}
	if obj.GetObjectKind().GroupVersionKind().GroupKind() == serviceGK {
		patches, err := removeServiceFields(obj)
		if err != nil {
			return nil, err
		}
		jsonPatch = append(jsonPatch, patches...)
	}

	return jsonPatch, nil
}

func addAnnotations(addAnnotations map[string]string) (jsonpatch.Patch, error) {
	patchJSON := `[`
	i := 0
	for key, value := range addAnnotations {
		if i == 0 {
			patchJSON = fmt.Sprintf(annotationInitial, patchJSON, key, value)
		} else {
			patchJSON = fmt.Sprintf(annotationNext, patchJSON, key, value)
		}
		i++
	}

	patchJSON = fmt.Sprintf("%v]", patchJSON)
	patch, err := jsonpatch.DecodePatch([]byte(patchJSON))
	if err != nil {
		fmt.Printf("%v", patchJSON)
		return nil, err
	}
	return patch, nil
}

func removeAnnotations(removeAnnotations []string) (jsonpatch.Patch, error) {
	patchJSON := `[`
	i := 0
	for _, annotation := range removeAnnotations {
		if i == 0 {
			patchJSON = fmt.Sprintf(removeAnnotationInitial, patchJSON, annotation)
		} else {
			patchJSON = fmt.Sprintf(removeAnnotationNext, patchJSON, annotation)
		}
		i++
	}

	patchJSON = fmt.Sprintf("%v]", patchJSON)
	patch, err := jsonpatch.DecodePatch([]byte(patchJSON))
	if err != nil {
		fmt.Printf("%v", patchJSON)
		return nil, err
	}
	return patch, nil
}

func removePodFields() (jsonpatch.Patch, error) {
	var patches jsonpatch.Patch
	patches, err := jsonpatch.DecodePatch([]byte(podNodeName))
	if err != nil {
		return nil, err
	}
	patch, err := jsonpatch.DecodePatch([]byte(podNodeSelector))
	if err != nil {
		return nil, err
	}
	patches = append(patches, patch...)
	patch, err = jsonpatch.DecodePatch([]byte(podPriority))
	if err != nil {
		return nil, err
	}
	patches = append(patches, patch...)
	return patches, nil
}

func updateNamespace(newNamespace string) (jsonpatch.Patch, error) {
	patchJSON := fmt.Sprintf(updateNamespaceString, newNamespace)

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
		patchJSON = fmt.Sprintf(updateRoleBindingSVCACCTNamspacestring, patchJSON, i, newNamespace)
	}

	patch, err := jsonpatch.DecodePatch([]byte(patchJSON))
	if err != nil {
		return nil, err
	}
	return patch, nil
}

func removeServiceFields(obj unstructured.Unstructured) (jsonpatch.Patch, error) {
	var patches jsonpatch.Patch
	if isLoadBalancerService(obj) {
		patch, err := jsonpatch.DecodePatch([]byte(updateExternalIPs))
		if err != nil {
			return nil, err
		}
		patches = append(patches, patch...)
	}

	if shouldRemoveServiceClusterIP(obj) {
		patch, err := jsonpatch.DecodePatch([]byte(updateClusterIP))
		if err != nil {
			return nil, err
		}
		patches = append(patches, patch...)
	}
	if shouldRemoveServiceClusterIPs(obj) {
		patch, err := jsonpatch.DecodePatch([]byte(updateClusterIPs))
		if err != nil {
			return nil, err
		}
		patches = append(patches, patch...)
	}
	patch, err := getNodePortPatch(obj)
	if err != nil {
		return nil, err
	}
	patches = append(patches, patch...)
	return patches, nil
}

func isLoadBalancerService(u unstructured.Unstructured) bool {
	// Get Spec
	spec, ok := u.UnstructuredContent()["spec"]
	if !ok {
		return false
	}

	specMap, ok := spec.(map[string]interface{})
	if !ok {
		return false
	}
	// Get type
	serviceType, ok := specMap["type"]
	if !ok {
		return false
	}
	return serviceType == "LoadBalancer"
}

func shouldRemoveServiceClusterIP(u unstructured.Unstructured) bool {
	// Get Spec
	spec, ok := u.UnstructuredContent()["spec"]
	if !ok {
		return false
	}

	specMap, ok := spec.(map[string]interface{})
	if !ok {
		return false
	}
	clusterIP, ok := specMap["clusterIP"]
	if !ok {
		return false
	}
	return clusterIP != "None"
}

func shouldRemoveServiceClusterIPs(u unstructured.Unstructured) bool {
	// Get Spec
	spec, ok := u.UnstructuredContent()["spec"]
	if !ok {
		return false
	}

	specMap, ok := spec.(map[string]interface{})
	if !ok {
		return false
	}
	clusterIPs, ok := specMap["clusterIPs"]
	if !ok {
		return false
	}
	// At this point, we have clusterIPs. Remove unless there's a first None element

	clusterIPsSlice, ok := clusterIPs.([]interface{})
	if !ok {
		return true
	}
	if len(clusterIPsSlice) == 0 {
		return true
	}
	return (clusterIPsSlice[0] != "None")
}

func getNodePortPatch(u unstructured.Unstructured) (jsonpatch.Patch, error) {
	var patch jsonpatch.Patch
	// Get Spec
	spec, ok := u.UnstructuredContent()["spec"]
	if !ok {
		return patch, nil
	}
	specMap, ok := spec.(map[string]interface{})
	if !ok {
		return patch, nil
	}
	// Get type
	serviceType, ok := specMap["type"]
	if !ok {
		return patch, nil
	}
	if serviceType == "ExternalName" {
		return patch, nil
	}
	servicePorts, ok := specMap["ports"]
	if !ok {
		return patch, nil
	}
	portsSlice, ok := servicePorts.([]interface{})
	if !ok {
		return patch, nil
	}

	explicitNodePorts := sets.NewString()
	unnamedPortInts := sets.NewInt()
	lastAppliedConfig, ok := u.GetAnnotations()["kubectl.kubernetes.io/last-applied-configuration"]
	if ok {
		appliedServiceUnstructured := new(map[string]interface{})
		if err := json.Unmarshal([]byte(lastAppliedConfig), appliedServiceUnstructured); err != nil {
			return patch, err
		}

		ports, bool, err := unstructured.NestedSlice(*appliedServiceUnstructured, "spec", "ports")

		if err != nil {
			return patch, err
		}

		if bool {
			for _, port := range ports {
				p, ok := port.(map[string]interface{})
				if !ok {
					continue
				}
				nodePort, nodePortBool, err := unstructured.NestedFieldNoCopy(p, "nodePort")
				if err != nil {
					return patch, err
				}
				if nodePortBool {
					nodePortInt, err := getNodePortInt(nodePort)
					if err != nil {
						return patch, err
					}
					if nodePortInt > 0 {
						portName, ok := p["name"]
						if !ok {
							// unnamed port
							unnamedPortInts.Insert(nodePortInt)
						} else {
							explicitNodePorts.Insert(portName.(string))
						}

					}
				}
			}
		}
	}

	for i, portInterface := range portsSlice {
		removeNodePort := false
		var nameStr string
		port, ok := portInterface.(map[string]interface{})
		if !ok {
			continue
		}
		name, ok := port["name"]
		if ok {
			nameStr, _ = name.(string)
		}
		nodePort, ok := port["nodePort"]
		if !ok {
			continue
		}
		nodePortInt, err := getNodePortInt(nodePort)
		if err != nil {
			return patch, err
		}
		if nodePortInt == 0 {
			continue
		}
		if len(nameStr) > 0 {
			if !explicitNodePorts.Has(nameStr) {
				removeNodePort = true
			}
		} else {
			if !unnamedPortInts.Has(int(nodePortInt)) {
				removeNodePort = true
			}
		}
		if removeNodePort {
			patchJSON := fmt.Sprintf(updateNodePortString, i)
			intPatch, err := jsonpatch.DecodePatch([]byte(patchJSON))
			if err != nil {
				return patch, err
			}
			patch = append(patch, intPatch...)
		}
	}

	return patch, nil
}

func getNodePortInt(nodePort interface{}) (int, error) {
	nodePortInt := 0
	switch nodePort.(type) {
	case int:
		nodePortInt = nodePort.(int)
	case int32:
		nodePortInt = int(nodePort.(int32))
	case int64:
		nodePortInt = int(nodePort.(int64))
	case float64:
		nodePortInt = int(nodePort.(float64))
	case string:
		nodePortInt, err := strconv.Atoi(nodePort.(string))
		if err != nil {
			return nodePortInt, err
		}
	}
	return nodePortInt, nil
}
