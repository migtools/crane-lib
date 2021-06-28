package kubernetes_test

import (
	"fmt"
	"testing"

	jsonpatch "github.com/evanphx/json-patch"
	transform "github.com/konveyor/crane-lib/transform"
	internaljsonpatch "github.com/konveyor/crane-lib/transform/internal/jsonpatch"
	"github.com/konveyor/crane-lib/transform/kubernetes"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestRun(t *testing.T) {

	cases := []struct {
		Name                string
		Object              *unstructured.Unstructured
		AddedAnnotations    map[string]string
		RegistryReplacement map[string]string
		NewNamespace        string
		RemoveAnnotation    []string
		ShouldError         bool
		Response            transform.PluginResponse
		PatchResponseJson   string
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
			AddedAnnotations: map[string]string{
				"testing.io":       "adding-new-thing",
				"multiple-testing": "two-new-anno",
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
			PatchResponseJson: `[{"op": "remove", "path": "/spec/nodeName"}]`,
		},
		{
			Name: "HandleService",
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
			PatchResponseJson: `[{"op": "remove", "path": "/spec/clusterIP"}]`,
		},
	}

	for _, c := range cases {
		t.Run(c.Name, func(t *testing.T) {
			var p transform.Plugin = &kubernetes.KubernetesTransformPlugin{
				AddedAnnotations:    c.AddedAnnotations,
				RegistryReplacement: c.RegistryReplacement,
				NewNamespace:        c.NewNamespace,
				RemoveAnnotation:    c.RemoveAnnotation,
			}
			resp, err := p.Run(c.Object, nil)
			if err != nil && !c.ShouldError {
				t.Error(err)
			}

			if resp.Version != c.Response.Version {
				t.Error(fmt.Sprintf("Invalid version. Actual: %v, Expected: %v", resp.Version, c.Response.Version))
			}

			if resp.IsWhiteOut != c.Response.IsWhiteOut {
				t.Error(fmt.Sprintf("Invalid whiteout. Actual: %v, Expected: %v", resp.IsWhiteOut, c.Response.IsWhiteOut))
			}
			if len(c.PatchResponseJson) != 0 && len(resp.Patches) != 0 {
				expectPatch, err := jsonpatch.DecodePatch([]byte(c.PatchResponseJson))
				if err != nil {
					t.Error(err)
				}
				ok, err := internaljsonpatch.Equal(resp.Patches, expectPatch)
				if !ok || err != nil {
					t.Error(fmt.Sprintf("Invalid patches. Actual: %#v, Expected: %#v", resp.Patches, expectPatch))
				}
			}
		})
	}
}
