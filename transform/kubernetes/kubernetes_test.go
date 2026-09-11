package kubernetes_test

import (
	"encoding/json"
	"fmt"
	"strings"
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
		DisableWhiteoutOwned bool
		RemoveAnnotations    []string
		ExtraWhiteouts       []schema.GroupKind
		IncludeOnly          []schema.GroupKind
		PVCStorageClassMap   map[string]string
		WhiteoutPVC          bool
		Extras               map[string]string
		ShouldError          bool
		Response             transform.PluginResponse
		PatchResponseJson    string
		ExpectNoPatches      bool
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
			Name: "PVCDeploymentDownscaled",
			Object: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"kind":       "Deployment",
					"apiVersion": "apps/v1",
					"metadata": map[string]interface{}{
						"name": "app",
					},
					"spec": map[string]interface{}{
						"replicas": int64(3),
						"template": map[string]interface{}{
							"spec": map[string]interface{}{
								"volumes": []interface{}{map[string]interface{}{
									"name": "data",
									"persistentVolumeClaim": map[string]interface{}{
										"claimName": "data",
									},
								}},
							},
						},
					},
				},
			},
			AddAnnotations:     map[string]string{"team": "ops"},
			Extras:            map[string]string{kubernetes.DownscaleWorkloadsFlag: "true"},
			Response:          transform.PluginResponse{IsWhiteOut: false, Version: "v1"},
			PatchResponseJson: `[{"op":"add","path":"/metadata/annotations","value":{"crane.konveyor.io/original-replicas":"3","team":"ops"}},{"op":"replace","path":"/spec/replicas","value":0}]`,
		},
		{
			Name: "PVCStatefulSetWithVolumeClaimTemplateDownscaled",
			Object: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"kind":       "StatefulSet",
					"apiVersion": "apps/v1",
					"metadata": map[string]interface{}{
						"annotations": map[string]interface{}{"app": "database"},
					},
					"spec": map[string]interface{}{
						"replicas": int64(2),
						"volumeClaimTemplates": []interface{}{map[string]interface{}{
							"metadata": map[string]interface{}{"name": "data"},
							"spec":     map[string]interface{}{},
						}},
					},
				},
			},
			Extras:            map[string]string{kubernetes.DownscaleWorkloadsFlag: "true"},
			Response:          transform.PluginResponse{IsWhiteOut: false, Version: "v1"},
			PatchResponseJson: `[{"op":"add","path":"/metadata/annotations/crane.konveyor.io~1original-replicas","value":"2"},{"op":"replace","path":"/spec/replicas","value":0}]`,
		},
		{
			Name: "PVCPodWhiteoutWhenDownscaling",
			Object: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"kind":       "Pod",
					"apiVersion": "v1",
					"spec": map[string]interface{}{
						"volumes": []interface{}{map[string]interface{}{
							"name": "data",
							"persistentVolumeClaim": map[string]interface{}{
								"claimName": "data",
							},
						}},
					},
				},
			},
			Extras:   map[string]string{kubernetes.DownscaleWorkloadsFlag: "true"},
			Response: transform.PluginResponse{IsWhiteOut: true, Version: "v1"},
		},
		{
			Name: "BlockPVCFieldsCleanedAndStorageClassMapped",
			Object: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"kind":       "PersistentVolumeClaim",
					"apiVersion": "v1",
					"metadata": map[string]interface{}{
						"finalizers": []interface{}{"kubernetes.io/pvc-protection"},
						"annotations": map[string]interface{}{
							"pv.kubernetes.io/bind-completed":               "yes",
							"volume.beta.kubernetes.io/storage-provisioner": "old-provisioner",
							"volume.kubernetes.io/selected-node":            "source-node",
							"keep":                                          "value",
						},
					},
					"spec": map[string]interface{}{
						"accessModes":      []interface{}{"ReadWriteOnce"},
						"storageClassName": "old-storage-class",
						"volumeMode":       "Block",
						"volumeName":       "pvc-source-id",
					},
				},
			},
			Response: transform.PluginResponse{
				IsWhiteOut: false,
				Version:    "v1",
			},
			PVCStorageClassMap: map[string]string{"old-storage-class": "new-storage-class"},
			PatchResponseJson:  `[{"op": "remove", "path": "/spec/volumeName"},{"op": "remove", "path": "/metadata/finalizers"},{"op": "remove", "path": "/metadata/annotations/pv.kubernetes.io~1bind-completed"},{"op": "remove", "path": "/metadata/annotations/volume.beta.kubernetes.io~1storage-provisioner"},{"op": "remove", "path": "/metadata/annotations/volume.kubernetes.io~1selected-node"},{"op": "replace", "path": "/spec/storageClassName", "value": "new-storage-class"}]`,
		},
		{
			Name: "StatefulSetVolumeClaimTemplateStorageClassMapped",
			Object: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"kind":       "StatefulSet",
					"apiVersion": "apps/v1",
					"spec": map[string]interface{}{
						"volumeClaimTemplates": []interface{}{
							map[string]interface{}{
								"metadata": map[string]interface{}{"name": "data"},
								"spec":     map[string]interface{}{"storageClassName": "old-storage-class"},
							},
						},
					},
				},
			},
			Response:           transform.PluginResponse{IsWhiteOut: false, Version: "v1"},
			PVCStorageClassMap: map[string]string{"old-storage-class": "new-storage-class"},
			PatchResponseJson:  `[{"op": "replace", "path": "/spec/volumeClaimTemplates/0/spec/storageClassName", "value": "new-storage-class"}]`,
		},
		{
			Name: "PVCWhiteOutWhenConfigured",
			Object: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"kind":       "PersistentVolumeClaim",
					"apiVersion": "v1",
				},
			},
			WhiteoutPVC: true,
			Response: transform.PluginResponse{
				IsWhiteOut: true,
				Version:    "v1",
			},
		},
		{
			Name: "SubscriptionWhiteOut",
			Object: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"kind":       "Subscription",
					"apiVersion": "operators.coreos.com/v1alpha1",
				},
			},
			Response: transform.PluginResponse{
				IsWhiteOut: true,
				Version:    "v1",
			},
		},
		{
			Name: "InstallPlanWhiteOut",
			Object: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"kind":       "InstallPlan",
					"apiVersion": "operators.coreos.com/v1alpha1",
				},
			},
			Response: transform.PluginResponse{
				IsWhiteOut: true,
				Version:    "v1",
			},
		},
		{
			Name: "ClusterServiceVersionWhiteOut",
			Object: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"kind":       "ClusterServiceVersion",
					"apiVersion": "operators.coreos.com/v1alpha1",
				},
			},
			Response: transform.PluginResponse{
				IsWhiteOut: true,
				Version:    "v1",
			},
		},
		{
			Name: "CatalogSourceWhiteOut",
			Object: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"kind":       "CatalogSource",
					"apiVersion": "operators.coreos.com/v1alpha1",
				},
			},
			Response: transform.PluginResponse{
				IsWhiteOut: true,
				Version:    "v1",
			},
		},
		{
			Name: "OperatorGroupWhiteOut",
			Object: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"kind":       "OperatorGroup",
					"apiVersion": "operators.coreos.com/v1",
				},
			},
			Response: transform.PluginResponse{
				IsWhiteOut: true,
				Version:    "v1",
			},
		},
		{
			Name: "OperatorConditionWhiteOut",
			Object: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"kind":       "OperatorCondition",
					"apiVersion": "operators.coreos.com/v2",
				},
			},
			Response: transform.PluginResponse{
				IsWhiteOut: true,
				Version:    "v1",
			},
		},
		{
			Name: "EventWhiteOut",
			Object: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"kind":       "Event",
					"apiVersion": "v1",
				},
			},
			Response: transform.PluginResponse{
				IsWhiteOut: true,
				Version:    "v1",
			},
		},
		{
			Name: "EventsK8sWhiteOut",
			Object: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"kind":       "Event",
					"apiVersion": "events.k8s.io/v1",
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
			ExtraWhiteouts: []schema.GroupKind{
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
			IncludeOnly: []schema.GroupKind{
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
								"name":        "PodOwner",
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
								"name":        "PodOwner",
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
								"name":        "PodOwner",
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
								"name":        "PodOwner",
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
			Name: "RemoveLastAppliedConfigurationAnnotation",
			Object: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"kind":       "Deployment",
					"apiVersion": "apps/v1",
					"metadata": map[string]interface{}{
						"name":      "test-deployment",
						"namespace": "default",
						"annotations": map[string]interface{}{
							"kubectl.kubernetes.io/last-applied-configuration": `{"apiVersion":"apps/v1","kind":"Deployment","metadata":{"name":"test-deployment"}}`,
							"other-annotation": "keep-this",
						},
					},
				},
			},
			Response: transform.PluginResponse{
				IsWhiteOut: false,
				Version:    "v1",
			},
			PatchResponseJson: `[{"op":"remove","path":"/metadata/annotations/kubectl.kubernetes.io~1last-applied-configuration"}]`,
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
						"type":      "LoadBalancer",
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
						"type":      "LoadBalancer",
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
			PatchResponseJson: `[{"op":"remove","path":"/metadata/annotations/kubectl.kubernetes.io~1last-applied-configuration"},{"op": "remove", "path": "/spec/ports/0/nodePort"}]`,
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
			PatchResponseJson: `[{"op":"remove","path":"/metadata/annotations/kubectl.kubernetes.io~1last-applied-configuration"},{"op": "remove", "path": "/spec/ports/1/nodePort"}]`,
		},
		{
			Name: "RemoveBatchControllerUIDFromJobWithManualSelectorFalse",
			Object: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"kind":       "Job",
					"apiVersion": "batch/v1",
					"metadata": map[string]interface{}{
						"name":      "wordpress-install",
						"namespace": "new-migrated-namespace",
						"labels": map[string]interface{}{
							"batch.kubernetes.io/controller-uid": "be719474-856d-4d84-80ad-4f4ab9ecdd30",
							"batch.kubernetes.io/job-name":       "wordpress-install",
							"migrated-with":                      "crane",
						},
						"annotations": map[string]interface{}{
							"batch.kubernetes.io/controller-uid": "be719474-856d-4d84-80ad-4f4ab9ecdd30",
							"other-annotation":                   "keep-this",
						},
					},
					"spec": map[string]interface{}{
						"manualSelector": false,
						"selector": map[string]interface{}{
							"matchLabels": map[string]interface{}{
								"batch.kubernetes.io/controller-uid": "be719474-856d-4d84-80ad-4f4ab9ecdd30",
								"migrated-with":                      "crane",
							},
						},
						"template": map[string]interface{}{
							"metadata": map[string]interface{}{
								"labels": map[string]interface{}{
									"batch.kubernetes.io/controller-uid": "be719474-856d-4d84-80ad-4f4ab9ecdd30",
									"batch.kubernetes.io/job-name":       "wordpress-install",
									"migrated-with":                      "crane",
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
			PatchResponseJson: `[{"op":"remove","path":"/metadata/annotations/batch.kubernetes.io~1controller-uid"},{"op":"remove","path":"/metadata/labels/batch.kubernetes.io~1controller-uid"},{"op":"remove","path":"/spec/selector"},{"op":"remove","path":"/spec/template/metadata/labels/batch.kubernetes.io~1controller-uid"},{"op":"add","path":"/spec/suspend","value":true}]`,
		},
		{
			Name: "RemoveBatchControllerUIDFromJobWithManualSelectorTrue",
			Object: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"kind":       "Job",
					"apiVersion": "batch/v1",
					"metadata": map[string]interface{}{
						"name":      "custom-job",
						"namespace": "test-namespace",
						"labels": map[string]interface{}{
							"batch.kubernetes.io/controller-uid": "be719474-856d-4d84-80ad-4f4ab9ecdd30",
							"custom-label":                       "value",
						},
						"annotations": map[string]interface{}{
							"batch.kubernetes.io/controller-uid": "be719474-856d-4d84-80ad-4f4ab9ecdd30",
						},
					},
					"spec": map[string]interface{}{
						"manualSelector": true,
						"selector": map[string]interface{}{
							"matchLabels": map[string]interface{}{
								"batch.kubernetes.io/controller-uid": "be719474-856d-4d84-80ad-4f4ab9ecdd30",
								"custom-label":                       "value",
							},
						},
						"template": map[string]interface{}{
							"metadata": map[string]interface{}{
								"labels": map[string]interface{}{
									"batch.kubernetes.io/controller-uid": "be719474-856d-4d84-80ad-4f4ab9ecdd30",
									"custom-label":                       "value",
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
			PatchResponseJson: `[{"op":"remove","path":"/metadata/annotations/batch.kubernetes.io~1controller-uid"},{"op":"remove","path":"/metadata/labels/batch.kubernetes.io~1controller-uid"},{"op":"remove","path":"/spec/selector/matchLabels/batch.kubernetes.io~1controller-uid"},{"op":"remove","path":"/spec/template/metadata/labels/batch.kubernetes.io~1controller-uid"},{"op":"add","path":"/spec/suspend","value":true}]`,
		},
		{
			Name: "RemoveBatchControllerUIDFromJobWithoutManualSelector",
			Object: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"kind":       "Job",
					"apiVersion": "batch/v1",
					"metadata": map[string]interface{}{
						"name":      "auto-selector-job",
						"namespace": "test-namespace",
						"labels": map[string]interface{}{
							"batch.kubernetes.io/controller-uid": "be719474-856d-4d84-80ad-4f4ab9ecdd30",
						},
					},
					"spec": map[string]interface{}{
						"selector": map[string]interface{}{
							"matchLabels": map[string]interface{}{
								"batch.kubernetes.io/controller-uid": "be719474-856d-4d84-80ad-4f4ab9ecdd30",
								"app": "test",
							},
						},
						"template": map[string]interface{}{
							"metadata": map[string]interface{}{
								"labels": map[string]interface{}{
									"batch.kubernetes.io/controller-uid": "be719474-856d-4d84-80ad-4f4ab9ecdd30",
									"app": "test",
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
			PatchResponseJson: `[{"op":"remove","path":"/metadata/labels/batch.kubernetes.io~1controller-uid"},{"op":"remove","path":"/spec/selector"},{"op":"remove","path":"/spec/template/metadata/labels/batch.kubernetes.io~1controller-uid"},{"op":"add","path":"/spec/suspend","value":true}]`,
		},
		{
			Name: "RemoveLegacyControllerUIDFromJob",
			Object: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"kind":       "Job",
					"apiVersion": "batch/v1",
					"metadata": map[string]interface{}{
						"name":      "legacy-job",
						"namespace": "test-namespace",
						"labels": map[string]interface{}{
							"controller-uid": "be719474-856d-4d84-80ad-4f4ab9ecdd30",
							"job-name":       "legacy-job",
						},
						"annotations": map[string]interface{}{
							"controller-uid": "be719474-856d-4d84-80ad-4f4ab9ecdd30",
						},
					},
					"spec": map[string]interface{}{
						"manualSelector": true,
						"selector": map[string]interface{}{
							"matchLabels": map[string]interface{}{
								"controller-uid": "be719474-856d-4d84-80ad-4f4ab9ecdd30",
								"job-name":       "legacy-job",
							},
						},
						"template": map[string]interface{}{
							"metadata": map[string]interface{}{
								"labels": map[string]interface{}{
									"controller-uid": "be719474-856d-4d84-80ad-4f4ab9ecdd30",
									"job-name":       "legacy-job",
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
			PatchResponseJson: `[{"op":"remove","path":"/metadata/annotations/controller-uid"},{"op":"remove","path":"/metadata/labels/controller-uid"},{"op":"remove","path":"/spec/selector/matchLabels/controller-uid"},{"op":"remove","path":"/spec/template/metadata/labels/controller-uid"},{"op":"add","path":"/spec/suspend","value":true}]`,
		},
		{
			Name: "RemoveBothControllerUIDKeysFromJob",
			Object: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"kind":       "Job",
					"apiVersion": "batch/v1",
					"metadata": map[string]interface{}{
						"name":      "mixed-job",
						"namespace": "test-namespace",
						"labels": map[string]interface{}{
							"batch.kubernetes.io/controller-uid": "be719474-856d-4d84-80ad-4f4ab9ecdd30",
							"controller-uid":                     "be719474-856d-4d84-80ad-4f4ab9ecdd30",
							"app":                                "test",
						},
						"annotations": map[string]interface{}{
							"batch.kubernetes.io/controller-uid": "be719474-856d-4d84-80ad-4f4ab9ecdd30",
							"controller-uid":                     "be719474-856d-4d84-80ad-4f4ab9ecdd30",
						},
					},
					"spec": map[string]interface{}{
						"manualSelector": true,
						"selector": map[string]interface{}{
							"matchLabels": map[string]interface{}{
								"batch.kubernetes.io/controller-uid": "be719474-856d-4d84-80ad-4f4ab9ecdd30",
								"controller-uid":                     "be719474-856d-4d84-80ad-4f4ab9ecdd30",
								"app":                                "test",
							},
						},
						"template": map[string]interface{}{
							"metadata": map[string]interface{}{
								"labels": map[string]interface{}{
									"batch.kubernetes.io/controller-uid": "be719474-856d-4d84-80ad-4f4ab9ecdd30",
									"controller-uid":                     "be719474-856d-4d84-80ad-4f4ab9ecdd30",
									"app":                                "test",
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
			PatchResponseJson: `[{"op":"remove","path":"/metadata/annotations/batch.kubernetes.io~1controller-uid"},{"op":"remove","path":"/metadata/annotations/controller-uid"},{"op":"remove","path":"/metadata/labels/batch.kubernetes.io~1controller-uid"},{"op":"remove","path":"/metadata/labels/controller-uid"},{"op":"remove","path":"/spec/selector/matchLabels/batch.kubernetes.io~1controller-uid"},{"op":"remove","path":"/spec/selector/matchLabels/controller-uid"},{"op":"remove","path":"/spec/template/metadata/labels/batch.kubernetes.io~1controller-uid"},{"op":"remove","path":"/spec/template/metadata/labels/controller-uid"},{"op":"add","path":"/spec/suspend","value":true}]`,
		},
		{
			Name: "SuspendStandaloneJobWithoutIdempotentAnnotation",
			Object: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"kind":       "Job",
					"apiVersion": "batch/v1",
					"metadata": map[string]interface{}{
						"name":      "standalone-job",
						"namespace": "test-namespace",
						"labels": map[string]interface{}{
							"app": "test",
						},
						"annotations": map[string]interface{}{
							"some-annotation": "some-value",
						},
					},
					"spec": map[string]interface{}{
						"template": map[string]interface{}{
							"metadata": map[string]interface{}{
								"labels": map[string]interface{}{
									"app": "test",
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
			PatchResponseJson: `[{"op":"add","path":"/spec/suspend","value":true}]`,
		},
		{
			Name: "DoNotSuspendStandaloneJobWithIdempotentAnnotation",
			Object: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"kind":       "Job",
					"apiVersion": "batch/v1",
					"metadata": map[string]interface{}{
						"name":      "idempotent-job",
						"namespace": "test-namespace",
						"labels": map[string]interface{}{
							"app": "database-migration",
						},
						"annotations": map[string]interface{}{
							"crane.konveyor.io/job-idempotent": "true",
						},
					},
					"spec": map[string]interface{}{
						"template": map[string]interface{}{
							"metadata": map[string]interface{}{
								"labels": map[string]interface{}{
									"app": "database-migration",
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
		},
		{
			Name: "DoNotSuspendJobWithOwnerReferences",
			Object: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"kind":       "Job",
					"apiVersion": "batch/v1",
					"metadata": map[string]interface{}{
						"name":      "cronjob-spawned-job",
						"namespace": "test-namespace",
						"labels": map[string]interface{}{
							"app": "test",
						},
						"ownerReferences": []interface{}{
							map[string]interface{}{
								"apiVersion": "batch/v1",
								"kind":       "CronJob",
								"name":       "my-cronjob",
								"uid":        "12345",
							},
						},
					},
					"spec": map[string]interface{}{
						"template": map[string]interface{}{
							"metadata": map[string]interface{}{
								"labels": map[string]interface{}{
									"app": "test",
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
			Name: "SuspendStandaloneJobAlreadySuspended",
			Object: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"kind":       "Job",
					"apiVersion": "batch/v1",
					"metadata": map[string]interface{}{
						"name":      "already-suspended-job",
						"namespace": "test-namespace",
					},
					"spec": map[string]interface{}{
						"suspend": true,
						"template": map[string]interface{}{
							"metadata": map[string]interface{}{
								"labels": map[string]interface{}{
									"app": "test",
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
		},
		{
			Name: "SATokenSecretWhiteOut",
			Object: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"kind":       "Secret",
					"apiVersion": "v1",
					"type":       "kubernetes.io/service-account-token",
					"metadata": map[string]interface{}{
						"name":      "app-sa-token-xwzl7",
						"namespace": "myapp",
						"annotations": map[string]interface{}{
							"kubernetes.io/service-account.name": "app-sa",
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
			Name: "DockercfgSecretWhiteOut",
			Object: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"kind":       "Secret",
					"apiVersion": "v1",
					"type":       "kubernetes.io/dockercfg",
					"metadata": map[string]interface{}{
						"name":      "builder-dockercfg-abc12",
						"namespace": "myapp",
						"annotations": map[string]interface{}{
							"kubernetes.io/service-account.name": "builder",
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
			Name: "UserCreatedSATokenSecretNotWhiteOut",
			Object: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"kind":       "Secret",
					"apiVersion": "v1",
					"type":       "kubernetes.io/service-account-token",
					"metadata": map[string]interface{}{
						"name":      "registry-token",
						"namespace": "myapp",
					},
				},
			},
			Response: transform.PluginResponse{
				IsWhiteOut: false,
				Version:    "v1",
			},
		},
		{
			Name: "UserCreatedDockercfgSecretNotWhiteOut",
			Object: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"kind":       "Secret",
					"apiVersion": "v1",
					"type":       "kubernetes.io/dockercfg",
					"metadata": map[string]interface{}{
						"name":      "private-registry",
						"namespace": "myapp",
					},
				},
			},
			Response: transform.PluginResponse{
				IsWhiteOut: false,
				Version:    "v1",
			},
		},
		{
			Name: "OpaqueSecretNotWhiteOut",
			Object: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"kind":       "Secret",
					"apiVersion": "v1",
					"type":       "Opaque",
					"metadata": map[string]interface{}{
						"name":      "my-app-secret",
						"namespace": "myapp",
					},
					"data": map[string]interface{}{
						"password": "c2VjcmV0",
					},
				},
			},
			Response: transform.PluginResponse{
				IsWhiteOut: false,
				Version:    "v1",
			},
		},
		{
			Name: "TLSSecretNotWhiteOut",
			Object: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"kind":       "Secret",
					"apiVersion": "v1",
					"type":       "kubernetes.io/tls",
					"metadata": map[string]interface{}{
						"name":      "my-tls-cert",
						"namespace": "myapp",
					},
				},
			},
			Response: transform.PluginResponse{
				IsWhiteOut: false,
				Version:    "v1",
			},
		},
		{
			Name: "ServiceAccountSecretsFieldStripped",
			Object: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"kind":       "ServiceAccount",
					"apiVersion": "v1",
					"metadata": map[string]interface{}{
						"name":      "app-sa",
						"namespace": "myapp",
					},
					"secrets": []interface{}{
						map[string]interface{}{
							"name": "app-sa-token-xwzl7",
						},
					},
				},
			},
			Response: transform.PluginResponse{
				IsWhiteOut: false,
				Version:    "v1",
			},
			PatchResponseJson: `[{"op": "remove", "path": "/secrets"}]`,
		},
		{
			Name: "ServiceAccountWithoutSecretsFieldUntouched",
			Object: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"kind":       "ServiceAccount",
					"apiVersion": "v1",
					"metadata": map[string]interface{}{
						"name":      "app-sa",
						"namespace": "myapp",
					},
				},
			},
			Response: transform.PluginResponse{
				IsWhiteOut: false,
				Version:    "v1",
			},
			ExpectNoPatches: true,
		},
	}

	for _, c := range cases {
		t.Run(c.Name, func(t *testing.T) {
			var p transform.Plugin = &kubernetes.KubernetesTransformPlugin{
				AddAnnotations:       c.AddAnnotations,
				RegistryReplacement:  c.RegistryReplacement,
				RemoveAnnotations:    c.RemoveAnnotations,
				DisableWhiteoutOwned: c.DisableWhiteoutOwned,
				ExtraWhiteouts:       c.ExtraWhiteouts,
				IncludeOnly:          c.IncludeOnly,
				PVCStorageClassMap:   c.PVCStorageClassMap,
				WhiteoutPVC:          c.WhiteoutPVC,
			}
			resp, err := p.Run(transform.PluginRequest{Unstructured: *c.Object, Extras: c.Extras})
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
			if c.ExpectNoPatches && len(resp.Patches) != 0 {
				actual, _ := json.Marshal(resp.Patches)
				t.Errorf("Expected no patches but got: %s", actual)
			}
		})
	}
}

func TestPVCStorageClassMapOptional(t *testing.T) {
	pvc := unstructured.Unstructured{Object: map[string]interface{}{
		"kind":       "PersistentVolumeClaim",
		"apiVersion": "v1",
		"spec": map[string]interface{}{
			"storageClassName": "old-storage-class",
		},
	}}

	t.Run("valid mapping", func(t *testing.T) {
		plugin := &kubernetes.KubernetesTransformPlugin{}
		response, err := plugin.Run(transform.PluginRequest{
			Unstructured: pvc,
			Extras: map[string]string{
				kubernetes.PVCStorageClassMap: "old-storage-class:new-storage-class",
			},
		})
		if err != nil {
			t.Fatal(err)
		}
		expected, err := jsonpatch.DecodePatch([]byte(`[{"op":"replace","path":"/spec/storageClassName","value":"new-storage-class"}]`))
		if err != nil {
			t.Fatal(err)
		}
		if equal, err := internaljsonpatch.Equal(response.Patches, expected); err != nil || !equal {
			t.Fatalf("unexpected patches: %v", response.Patches)
		}
	})

	t.Run("invalid mapping", func(t *testing.T) {
		plugin := &kubernetes.KubernetesTransformPlugin{}
		_, err := plugin.Run(transform.PluginRequest{
			Unstructured: pvc,
			Extras: map[string]string{
				kubernetes.PVCStorageClassMap: "invalid",
			},
		})
		if err == nil {
			t.Fatal("expected invalid StorageClass mapping to fail")
		}
	})

	t.Run("whiteout PVC", func(t *testing.T) {
		plugin := &kubernetes.KubernetesTransformPlugin{}
		response, err := plugin.Run(transform.PluginRequest{
			Unstructured: pvc,
			Extras: map[string]string{
				kubernetes.WhiteoutPVCFlag: "true",
			},
		})
		if err != nil {
			t.Fatal(err)
		}
		if !response.IsWhiteOut {
			t.Fatal("expected PVC to be whiteouted when whiteout-pvc is true")
		}
	})
}

func TestPVCRenameMapRenamesPVCManifestAndWorkloadReference(t *testing.T) {
	pvc := unstructured.Unstructured{Object: map[string]interface{}{
		"kind":       "PersistentVolumeClaim",
		"apiVersion": "v1",
		"metadata": map[string]interface{}{"name": "old"},
	}}
	workload := unstructured.Unstructured{Object: map[string]interface{}{
		"kind":       "Deployment",
		"apiVersion": "apps/v1",
		"spec": map[string]interface{}{
			"template": map[string]interface{}{
				"spec": map[string]interface{}{
					"volumes": []interface{}{map[string]interface{}{
						"name": "data",
						"persistentVolumeClaim": map[string]interface{}{
							"claimName": "old",
						},
					}},
				},
			},
		},
	}}

	plugin := &kubernetes.KubernetesTransformPlugin{}
	for _, test := range []struct {
		name     string
		object   unstructured.Unstructured
		expected string
	}{
		{
			name:     "PVC manifest",
			object:   pvc,
			expected: `[{"op":"replace","path":"/metadata/name","value":"new"}]`,
		},
		{
			name:     "workload reference",
			object:   workload,
			expected: `[{"op":"replace","path":"/spec/template/spec/volumes/0/persistentVolumeClaim/claimName","value":"new"}]`,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			response, err := plugin.Run(transform.PluginRequest{
				Unstructured: test.object,
				Extras: map[string]string{
					kubernetes.PVCRenameMap: "old:new",
				},
			})
			if err != nil {
				t.Fatal(err)
			}
			expected, err := jsonpatch.DecodePatch([]byte(test.expected))
			if err != nil {
				t.Fatal(err)
			}
			if equal, err := internaljsonpatch.Equal(response.Patches, expected); err != nil || !equal {
				t.Fatalf("unexpected patches: %v", response.Patches)
			}
		})
	}
}

func TestDownscaleWorkloadsPreservesAddedAnnotations(t *testing.T) {
	workload := unstructured.Unstructured{Object: map[string]interface{}{
		"kind":       "Deployment",
		"apiVersion": "apps/v1",
		"metadata": map[string]interface{}{
			"name": "app",
		},
		"spec": map[string]interface{}{
			"replicas": int64(3),
			"template": map[string]interface{}{
				"spec": map[string]interface{}{
					"volumes": []interface{}{map[string]interface{}{
						"name": "data",
						"persistentVolumeClaim": map[string]interface{}{
							"claimName": "data",
						},
					}},
				},
			},
		},
	}}
	runner := transform.NewRunner(nil, nil, map[string]string{
		kubernetes.DownscaleWorkloadsFlag: "true",
		kubernetes.AddAnnotationsFlag:     "team=ops",
	})
	plugin := &kubernetes.KubernetesTransformPlugin{}
	original, err := workload.MarshalJSON()
	if err != nil {
		t.Fatal(err)
	}

	for i := 0; i < 100; i++ {
		response, err := runner.Run(workload, []transform.Plugin{plugin})
		if err != nil {
			t.Fatal(err)
		}
		patch, err := jsonpatch.DecodePatch(response.TransformFile)
		if err != nil {
			t.Fatal(err)
		}
		transformed, err := patch.Apply(original)
		if err != nil {
			t.Fatal(err)
		}
		var result unstructured.Unstructured
		if err := result.UnmarshalJSON(transformed); err != nil {
			t.Fatal(err)
		}
		annotations := result.GetAnnotations()
		if annotations["team"] != "ops" || annotations[kubernetes.OriginalReplicasAnnotation] != "3" {
			t.Fatalf("annotations were not preserved: %v", annotations)
		}
	}
}

func TestDownscaleWorkloadsRejectsReservedAnnotationFlags(t *testing.T) {
	obj := unstructured.Unstructured{Object: map[string]interface{}{
		"kind":       "Deployment",
		"apiVersion": "apps/v1",
	}}

	tests := []struct {
		name   string
		extras map[string]string
	}{
		{
			name: "add reserved annotation",
			extras: map[string]string{
				kubernetes.DownscaleWorkloadsFlag: "true",
				kubernetes.AddAnnotationsFlag:     kubernetes.OriginalReplicasAnnotation + "=3",
			},
		},
		{
			name: "remove reserved annotation",
			extras: map[string]string{
				kubernetes.DownscaleWorkloadsFlag: "true",
				kubernetes.RemoveAnnotationsFlag:  kubernetes.OriginalReplicasAnnotation,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			plugin := &kubernetes.KubernetesTransformPlugin{}
			_, err := plugin.Run(transform.PluginRequest{Unstructured: obj, Extras: tt.extras})
			if err == nil {
				t.Fatalf("expected %s conflict to fail", kubernetes.OriginalReplicasAnnotation)
			}
			if !strings.Contains(err.Error(), kubernetes.OriginalReplicasAnnotation) {
				t.Fatalf("expected error to identify reserved annotation, got: %v", err)
			}
		})
	}
}

func TestPVCBooleanOptionsRejectInvalidValues(t *testing.T) {
	obj := unstructured.Unstructured{Object: map[string]interface{}{
		"kind":       "PersistentVolumeClaim",
		"apiVersion": "v1",
	}}

	for _, flag := range []string{kubernetes.WhiteoutPVCFlag, kubernetes.DownscaleWorkloadsFlag} {
		t.Run(flag, func(t *testing.T) {
			plugin := &kubernetes.KubernetesTransformPlugin{}
			_, err := plugin.Run(transform.PluginRequest{
				Unstructured: obj,
				Extras:       map[string]string{flag: "treu"},
			})
			if err == nil {
				t.Fatalf("expected invalid %s value to fail", flag)
			}
			if !strings.Contains(err.Error(), flag) {
				t.Fatalf("expected error to identify %s, got: %v", flag, err)
			}
		})
	}
}
