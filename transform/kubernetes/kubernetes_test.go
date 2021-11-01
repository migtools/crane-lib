package kubernetes_test

import (
        "encoding/json"
	"fmt"
	"testing"

	jsonpatch "github.com/evanphx/json-patch"
	transform "github.com/konveyor/crane-lib/transform"
	internaljsonpatch "github.com/konveyor/crane-lib/transform/internal/jsonpatch"
	"github.com/konveyor/crane-lib/transform/kubernetes"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func TestRun(t *testing.T) {

	cases := []struct {
		Name                 string
		Object               *unstructured.Unstructured
		AddAnnotations       map[string]string
		RegistryReplacement  map[string]string
		NewNamespace         string
		DisableWhiteoutOwned bool
		RemoveAnnotations    []string
		ExtraWhiteouts       []schema.GroupKind
		IncludeOnly          []schema.GroupKind
		ShouldError          bool
		Response             transform.PluginResponse
		PatchResponseJson    string
	}{
		{
			Name: "EnpointWhiteOut",
			Object: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"kind":       "Endpoints",
					"apiVersion": "v1",
				},
			},
			Response: transform.PluginResponse{
				IsWhiteOut: true,
				Version:    "v1",
			},
		},
		{
			Name: "EnpointSliceWhiteOut",
			Object: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"kind":       "EndpointSlice",
					"apiVersion": "discovery.k8s.io/v1",
				},
			},
			Response: transform.PluginResponse{
				IsWhiteOut: true,
				Version:    "v1",
			},
		},
		{
			Name: "PVCWhiteOut",
			Object: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"kind":       "PersistentVolumeClaim",
					"apiVersion": "v1",
				},
			},
			Response: transform.PluginResponse{
				IsWhiteOut: true,
				Version:    "v1",
			},
		},
		{
			Name: "NoDeploymentWhiteoutByDefault",
			Object: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"kind":       "Deployment",
					"apiVersion": "apps/v1",
				},
			},
			Response: transform.PluginResponse{
				IsWhiteOut: false,
				Version:    "v1",
			},
		},
		{
			Name: "DeploymentAdditionalWhiteout",
			Object: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"kind":       "Deployment",
					"apiVersion": "apps/v1",
				},
			},
			ExtraWhiteouts: []schema.GroupKind {
				{
					Group: "apps",
					Kind:  "Deployment",
				},
			},
			Response: transform.PluginResponse{
				IsWhiteOut: true,
				Version:    "v1",
			},
		},
		{
			Name: "DeploymentWhiteoutWithIncludeOnly",
			Object: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"kind":       "Deployment",
					"apiVersion": "apps/v1",
				},
			},
			IncludeOnly: []schema.GroupKind {
				{
					Group: "",
					Kind:  "Pod",
				},
			},
			Response: transform.PluginResponse{
				IsWhiteOut: true,
				Version:    "v1",
			},
		},
		{
			Name: "OwnedPodWhiteOut",
			Object: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"kind":       "Pod",
					"apiVersion": "v1",
					"metadata": map[string]interface{}{
						"ownerReferences": []interface{}{
							map[string]interface{}{
								"apiVersion": "apps/v1",
								"kind":       "ReplicaSet",
								"ame":       "PodOwner",
								"uid":        "1de6b4d2-ea5b-11eb-b902-021bddcaf6e4",
							},
						},
					},
				},
			},
			Response: transform.PluginResponse{
				IsWhiteOut: true,
				Version:    "v1",
			},
		},
		{
			Name: "OwnedPodSpecableWhiteOut",
			Object: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"kind":       "InvalidGVK",
					"apiVersion": "v1",
					"metadata": map[string]interface{}{
						"ownerReferences": []interface{}{
							map[string]interface{}{
								"apiVersion": "apps/v1",
								"kind":       "ReplicaSet",
								"ame":       "PodOwner",
								"uid":        "1de6b4d2-ea5b-11eb-b902-021bddcaf6e4",
							},
						},
					},
					"spec": map[string]interface{}{
						"template": v1.PodTemplateSpec{
							Spec: v1.PodSpec{
								InitContainers: []v1.Container{
									{
										Image: "quay.io/shawn_hurley/testing-image",
									},
								},
								Containers: []v1.Container{
									{
										Image: "quay.io/shawn_hurley/testing-image-real",
									},
								},
							},
						},
					},
				},
			},
			Response: transform.PluginResponse{
				IsWhiteOut: true,
				Version:    "v1",
			},
		},
		{
			Name: "OwnedPodWhiteOutDisabled",
			Object: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"kind":       "Pod",
					"apiVersion": "v1",
					"metadata": map[string]interface{}{
						"ownerReferences": []interface{}{
							map[string]interface{}{
								"apiVersion": "apps/v1",
								"kind":       "ReplicaSet",
								"ame":       "PodOwner",
								"uid":        "1de6b4d2-ea5b-11eb-b902-021bddcaf6e4",
							},
						},
					},
				},
			},
			DisableWhiteoutOwned: true,
			Response: transform.PluginResponse{
				IsWhiteOut: false,
				Version:    "v1",
			},
		},
		{
			Name: "OwnedPodSpecableWhiteOutDisabled",
			Object: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"kind":       "InvalidGVK",
					"apiVersion": "v1",
					"metadata": map[string]interface{}{
						"ownerReferences": []interface{}{
							map[string]interface{}{
								"apiVersion": "apps/v1",
								"kind":       "ReplicaSet",
								"ame":       "PodOwner",
								"uid":        "1de6b4d2-ea5b-11eb-b902-021bddcaf6e4",
							},
						},
					},
					"spec": map[string]interface{}{
						"template": v1.PodTemplateSpec{
							Spec: v1.PodSpec{
								InitContainers: []v1.Container{
									{
										Image: "quay.io/shawn_hurley/testing-image",
									},
								},
								Containers: []v1.Container{
									{
										Image: "quay.io/shawn_hurley/testing-image-real",
									},
								},
							},
						},
					},
				},
			},
			DisableWhiteoutOwned: true,
			Response: transform.PluginResponse{
				IsWhiteOut: false,
				Version:    "v1",
			},
		},
		{
			Name: "PodSpecableContainersUpdated",
			Object: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"kind":       "InvalidGVK",
					"apiVersion": "v1",
					"spec": map[string]interface{}{
						"template": v1.PodTemplateSpec{
							Spec: v1.PodSpec{
								InitContainers: []v1.Container{
									{
										Image: "quay.io/shawn_hurley/testing-image",
									},
								},
								Containers: []v1.Container{
									{
										Image: "quay.io/shawn_hurley/testing-image-real",
									},
								},
							},
						},
					},
				},
			},
			Response: transform.PluginResponse{
				IsWhiteOut: false,
				Version:    "v1",
			},
			PatchResponseJson: `[{"op": "replace", "path": "/spec/template/spec/initContainers/0/image", "value": "dockerhub.io/shawn_hurley/testing-image"}, {"op": "replace", "path": "/spec/template/spec/containers/0/image", "value": "dockerhub.io/shawn_hurley/testing-image-real"}]`,
			RegistryReplacement: map[string]string{
				"quay.io": "dockerhub.io",
			},
		},
		{
			Name: "NonPodSpecable",
			Object: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"kind":       "InvalidGVK",
					"apiVersion": "v1",
					"spec": map[string]interface{}{
						"podTemplate": v1.PodTemplateSpec{
							Spec: v1.PodSpec{
								InitContainers: []v1.Container{
									{
										Image: "quay.io/shawn_hurley/testing-image",
									},
								},
								Containers: []v1.Container{
									{
										Image: "quay.io/shawn_hurley/testing-image-real",
									},
								},
							},
						},
					},
				},
			},
			Response: transform.PluginResponse{
				IsWhiteOut: false,
				Version:    "v1",
			},
			RegistryReplacement: map[string]string{
				"quay.io": "dockerhub.io",
			},
		},
		{
			Name: "RemoveMetadataAndStatus",
			Object: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"kind":       "InvalidGVK",
					"apiVersion": "v1",
					"metadata": map[string]interface{}{
						"uid":             "1de6b4d2-ea5b-11eb-b902-021bddcaf6e4",
						"resourceVersion": "19281149",
					},
					"status": map[string]interface{}{
						"something":      "12345",
						"something-else": "abcde",
					},
				},
			},
			Response: transform.PluginResponse{
				IsWhiteOut: false,
				Version:    "v1",
			},
			PatchResponseJson: `[{"op": "remove", "path": "/metadata/uid"},{"op": "remove", "path": "/metadata/resourceVersion"},{"op": "remove", "path": "/status"}]`,
		},
		{
			Name: "AddAnnotations",
			Object: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"kind":       "InvalidGVK",
					"apiVersion": "v1",
				},
			},
			Response: transform.PluginResponse{
				IsWhiteOut: false,
				Version:    "v1",
			},
			PatchResponseJson: `[{"op": "add", "path": "/metadata/annotations/multiple-testing", "value": "two-new-anno"},{"op": "add", "path": "/metadata/annotations/testing.io", "value": "adding-new-thing"}]`,
			AddAnnotations: map[string]string{
				"testing.io":       "adding-new-thing",
				"multiple-testing": "two-new-anno",
			},
		},
		{
			Name: "RemoveAnnotations",
			Object: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"kind":       "InvalidGVK",
					"apiVersion": "v1",
				},
			},
			Response: transform.PluginResponse{
				IsWhiteOut: false,
				Version:    "v1",
			},
			PatchResponseJson: `[{"op": "remove", "path": "/metadata/annotations/multiple-testing"},{"op": "remove", "path": "/metadata/annotations/testing.io"}]`,
			RemoveAnnotations: []string{
				"testing.io",
				"multiple-testing",
			},
		},
		{
			Name: "HandlePod",
			Object: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"kind":       "Pod",
					"apiVersion": "v1",
					"spec": v1.PodSpec{
						InitContainers: []v1.Container{
							{
								Image: "quay.io/shawn_hurley/testing-image",
							},
						},
						Containers: []v1.Container{
							{
								Image: "quay.io/shawn_hurley/testing-image-real",
							},
						},
					},
				},
			},
			Response: transform.PluginResponse{
				IsWhiteOut: false,
				Version:    "v1",
			},
			PatchResponseJson: `[{"op": "remove", "path": "/spec/nodeName"},{"op": "remove", "path": "/spec/nodeSelector"},{"op": "remove", "path": "/spec/priority"}]`,
		},
		{
			Name: "HandleBaseService",
			Object: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"kind":       "Service",
					"apiVersion": "v1",
				},
			},
			Response: transform.PluginResponse{
				IsWhiteOut: false,
				Version:    "v1",
			},
			PatchResponseJson: ``,
		},
		{
			Name: "HandleLoadBalancerService",
			Object: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"kind":       "Service",
					"apiVersion": "v1",
					"spec": map[string]interface{}{
						"type": "LoadBalancer",
					},
				},
			},
			Response: transform.PluginResponse{
				IsWhiteOut: false,
				Version:    "v1",
			},
			PatchResponseJson: `[{"op": "remove", "path": "/spec/externalIPs"}]`,
		},
		{
			Name: "HandleLoadBalancerServiceWithClusterIP",
			Object: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"kind":       "Service",
					"apiVersion": "v1",
					"spec": map[string]interface{}{
						"type": "LoadBalancer",
						"clusterIP": "1.2.3.4",
					},
				},
			},
			Response: transform.PluginResponse{
				IsWhiteOut: false,
				Version:    "v1",
			},
			PatchResponseJson: `[{"op": "remove", "path": "/spec/clusterIP"},{"op": "remove", "path": "/spec/externalIPs"}]`,
		},
		{
			Name: "HandleLoadBalancerServiceWithClusterIPs",
			Object: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"kind":       "Service",
					"apiVersion": "v1",
					"spec": map[string]interface{}{
						"type": "LoadBalancer",
						"clusterIPs": []interface{}{
							"1.2.3.4",
						},
					},
				},
			},
			Response: transform.PluginResponse{
				IsWhiteOut: false,
				Version:    "v1",
			},
			PatchResponseJson: `[{"op": "remove", "path": "/spec/clusterIPs"},{"op": "remove", "path": "/spec/externalIPs"}]`,
		},
		{
			Name: "HandleLoadBalancerServiceWithClusterIPNone",
			Object: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"kind":       "Service",
					"apiVersion": "v1",
					"spec": map[string]interface{}{
						"type": "LoadBalancer",
						"clusterIP": "None",
					},
				},
			},
			Response: transform.PluginResponse{
				IsWhiteOut: false,
				Version:    "v1",
			},
			PatchResponseJson: `[{"op": "remove", "path": "/spec/externalIPs"}]`,
		},
		{
			Name: "HandleLoadBalancerNodePort",
			Object: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"kind":       "Service",
					"apiVersion": "v1",
					"spec": map[string]interface{}{
						"type": "CustomType",
						"ports": []interface{}{
							map[string]interface{}{
								"port":     31000,
								"nodePort": 31000,
							},
							map[string]interface{}{
								"port":     31001,
								"nodePort": 31001,
							},
						},
					},
				},
			},
			Response: transform.PluginResponse{
				IsWhiteOut: false,
				Version:    "v1",
			},
			PatchResponseJson: `[{"op": "remove", "path": "/spec/ports/0/nodePort"},{"op": "remove", "path": "/spec/ports/1/nodePort"}]`,
		},
		{
			Name: "HandleNodePortUnnamedAnnotation",
			Object: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"kind":       "Service",
					"apiVersion": "v1",
					"metadata": map[string]interface{}{
						"name": "svc-1",
						"annotations": map[string]interface{}{
							"kubectl.kubernetes.io/last-applied-configuration": `
      {"apiVersion":"v1","kind":"Service","metadata":{"name":"svc-1","namespace":"foo"},"spec":{"ports":[{"nodePort":31001}]}}`,
						},
					},

					"spec": map[string]interface{}{
						"type": "CustomType",
						"ports": []interface{}{
							map[string]interface{}{
								"port":     31000,
								"nodePort": 31000,
							},
							map[string]interface{}{
								"port":     31001,
								"nodePort": 31001,
							},
						},
					},
				},
			},
			Response: transform.PluginResponse{
				IsWhiteOut: false,
				Version:    "v1",
			},
			PatchResponseJson: `[{"op": "remove", "path": "/spec/ports/0/nodePort"}]`,
		},
		{
			Name: "HandleNodePortNamedAnnotation",
			Object: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"kind":       "Service",
					"apiVersion": "v1",
					"metadata": map[string]interface{}{
						"name": "svc-1",
						"annotations": map[string]interface{}{
							"kubectl.kubernetes.io/last-applied-configuration": `
      {"apiVersion":"v1","kind":"Service","metadata":{"name":"svc-1","namespace":"foo"},"spec":{"ports":[{"name": "foo","nodePort":31000}]}}`,
						},
					},

					"spec": map[string]interface{}{
						"type": "CustomType",
						"ports": []interface{}{
							map[string]interface{}{
								"name":     "foo",
								"port":     31000,
								"nodePort": 31000,
							},
							map[string]interface{}{
								"name":     "bar",
								"port":     31001,
								"nodePort": 31001,
							},
						},
					},
				},
			},
			Response: transform.PluginResponse{
				IsWhiteOut: false,
				Version:    "v1",
			},
			PatchResponseJson: `[{"op": "remove", "path": "/spec/ports/1/nodePort"}]`,
		},
	}

	for _, c := range cases {
		t.Run(c.Name, func(t *testing.T) {
			var p transform.Plugin = &kubernetes.KubernetesTransformPlugin{
				AddAnnotations:       c.AddAnnotations,
				RegistryReplacement:  c.RegistryReplacement,
				NewNamespace:         c.NewNamespace,
				RemoveAnnotations:    c.RemoveAnnotations,
				DisableWhiteoutOwned: c.DisableWhiteoutOwned,
				ExtraWhiteouts:       c.ExtraWhiteouts,
				IncludeOnly:          c.IncludeOnly,
			}
			resp, err := p.Run(transform.PluginRequest{Unstructured:*c.Object})
			if err != nil && !c.ShouldError {
				t.Error(err)
			}

			if resp.Version != c.Response.Version {
				t.Error(fmt.Sprintf("Invalid version. Actual: %v, Expected: %v", resp.Version, c.Response.Version))
			}

			if resp.IsWhiteOut != c.Response.IsWhiteOut {
				t.Error(fmt.Sprintf("Invalid whiteout. Actual: %v, Expected: %v", resp.IsWhiteOut, c.Response.IsWhiteOut))
			}
			if len(c.PatchResponseJson) != 0 {
				expectPatch, err := jsonpatch.DecodePatch([]byte(c.PatchResponseJson))
				if err != nil {
					t.Error(err)
				}
				if len(resp.Patches) != 0 {
					ok, err := internaljsonpatch.Equal(resp.Patches, expectPatch)
					if !ok || err != nil {
						actual, _ := json.Marshal(resp.Patches)
						t.Error(fmt.Sprintf("Invalid patches. Actual: %s, Expected: %v", actual, c.PatchResponseJson))
					}
				} else {
					t.Error(fmt.Sprintf("Patches Expected: %#v, none found", expectPatch))
				}
			}
		})
	}
}
