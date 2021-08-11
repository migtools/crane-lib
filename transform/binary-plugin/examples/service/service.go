package main

import (
	"encoding/json"
	"fmt"
	"strconv"

	jsonpatch "github.com/evanphx/json-patch"
	"github.com/konveyor/crane-lib/transform"
	"github.com/konveyor/crane-lib/transform/cli"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/sets"

)

func main() {
	cli.RunAndExit(cli.NewCustomPlugin("ServicePlugin", "v1", nil, Run))
}

const (
	updateNodePortString = `[
{"op": "remove", "path": "/spec/ports/%v/nodePort"}
]`

)
// Removes ExternalIPs for LoadBalancer services
func Run(u *unstructured.Unstructured, extras map[string]string) (transform.PluginResponse, error) {
	// plugin writers need to write custome code here.
	var patch jsonpatch.Patch
	if u.GetKind() == "Service" {
		if IsLoadBalancerService(*u) {
			patchJSON := fmt.Sprintf(`[
{ "op": "remove", "path": "/spec/externalIPs"}
]`)
			intPatch, err := jsonpatch.DecodePatch([]byte(patchJSON))
			if err != nil {
				return transform.PluginResponse{}, err
			}
			patch = append(patch, intPatch...)
		}
		if ShouldRemoveServiceClusterIP(*u) {
			patchJSON := fmt.Sprintf(`[
{ "op": "remove", "path": "/spec/clusterIP"}
]`)
			intPatch, err := jsonpatch.DecodePatch([]byte(patchJSON))
			if err != nil {
				return transform.PluginResponse{}, err
			}
			patch = append(patch, intPatch...)
		}
		if ShouldRemoveServiceClusterIPs(*u) {
			patchJSON := fmt.Sprintf(`[
{ "op": "remove", "path": "/spec/clusterIPs"}
]`)
			intPatch, err := jsonpatch.DecodePatch([]byte(patchJSON))
			if err != nil {
				return transform.PluginResponse{}, err
			}
			patch = append(patch, intPatch...)
		}
		intPatch, err := getNodePortPatch(*u)
		if err != nil {
			return transform.PluginResponse{}, err
		}
		patch = append(patch, intPatch...)
	}
	return transform.PluginResponse{
		Version:    "v1",
		IsWhiteOut: false,
		Patches:    patch,
	}, nil
}

func IsLoadBalancerService(u unstructured.Unstructured) bool {
	if u.GetKind() != "Service" {
		return false
	}
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

func ShouldRemoveServiceClusterIP(u unstructured.Unstructured) bool {
	if u.GetKind() != "Service" {
		return false
	}
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
	clusterIP, ok := specMap["clusterIP"]
	if !ok {
		return false
	}
	return clusterIP != "None"
}

func ShouldRemoveServiceClusterIPs(u unstructured.Unstructured) bool {
	if u.GetKind() != "Service" {
		return false
	}
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
	if u.GetKind() != "Service" {
		return patch, nil
	}
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
