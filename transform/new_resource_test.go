package transform

import (
	"encoding/json"
	"reflect"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestSplitNewResourceToSkeletonAndPatch_BasicResource(t *testing.T) {
	resource := unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "shipwright.io/v1beta1",
			"kind":       "Build",
			"metadata": map[string]interface{}{
				"name":      "my-app-build",
				"namespace": "default",
				"labels": map[string]interface{}{
					"app":            "my-app",
					"converted-from": "BuildConfig",
				},
				"annotations": map[string]interface{}{
					"source-kind": "BuildConfig",
				},
			},
			"spec": map[string]interface{}{
				"source": map[string]interface{}{
					"type": "Git",
					"git": map[string]interface{}{
						"url": "https://github.com/example/repo",
					},
				},
				"output": map[string]interface{}{
					"image": "quay.io/example/my-app:latest",
				},
			},
		},
	}

	skeleton, patch, err := SplitNewResourceToSkeletonAndPatch(resource)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Skeleton should have only apiVersion, kind, metadata.name, metadata.namespace
	if skeleton.GetAPIVersion() != "shipwright.io/v1beta1" {
		t.Errorf("skeleton apiVersion: got %q", skeleton.GetAPIVersion())
	}
	if skeleton.GetKind() != "Build" {
		t.Errorf("skeleton kind: got %q", skeleton.GetKind())
	}
	if skeleton.GetName() != "my-app-build" {
		t.Errorf("skeleton name: got %q", skeleton.GetName())
	}
	if skeleton.GetNamespace() != "default" {
		t.Errorf("skeleton namespace: got %q", skeleton.GetNamespace())
	}

	// Skeleton should NOT have labels, annotations, or spec
	if skeleton.GetLabels() != nil && len(skeleton.GetLabels()) > 0 {
		t.Errorf("skeleton should not have labels, got %v", skeleton.GetLabels())
	}
	if skeleton.GetAnnotations() != nil && len(skeleton.GetAnnotations()) > 0 {
		t.Errorf("skeleton should not have annotations, got %v", skeleton.GetAnnotations())
	}
	if _, exists := skeleton.Object["spec"]; exists {
		t.Errorf("skeleton should not have spec")
	}

	// Patch should exist and have 3 operations (annotations, labels, spec)
	if patch == nil {
		t.Fatal("patch should not be nil")
	}
	if len(patch) != 3 {
		t.Errorf("expected 3 patch ops (annotations, labels, spec), got %d", len(patch))
	}

	// Verify patch applies correctly to skeleton
	skelJSON, _ := skeleton.MarshalJSON()
	patched, err := patch.Apply(skelJSON)
	if err != nil {
		t.Fatalf("failed to apply patch to skeleton: %v", err)
	}

	var result unstructured.Unstructured
	if err := result.UnmarshalJSON(patched); err != nil {
		t.Fatalf("failed to unmarshal patched resource: %v", err)
	}

	if result.GetKind() != "Build" {
		t.Errorf("patched kind: got %q", result.GetKind())
	}
	labels := result.GetLabels()
	if labels["app"] != "my-app" {
		t.Errorf("patched labels missing app=my-app")
	}
	annotations := result.GetAnnotations()
	if annotations["source-kind"] != "BuildConfig" {
		t.Errorf("patched annotations missing source-kind")
	}
	spec, _, _ := unstructured.NestedMap(result.Object, "spec")
	if spec == nil {
		t.Errorf("patched resource should have spec")
	}
}

func TestSplitNewResourceToSkeletonAndPatch_SkeletonOnly(t *testing.T) {
	resource := unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "ConfigMap",
			"metadata": map[string]interface{}{
				"name":      "my-config",
				"namespace": "default",
			},
		},
	}

	skeleton, patch, err := SplitNewResourceToSkeletonAndPatch(resource)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if skeleton.GetKind() != "ConfigMap" {
		t.Errorf("skeleton kind: got %q", skeleton.GetKind())
	}

	// No extra fields → no patch
	if patch != nil {
		t.Errorf("expected nil patch for skeleton-only resource, got %d ops", len(patch))
	}
}

func TestSplitNewResourceToSkeletonAndPatch_ClusterScoped(t *testing.T) {
	resource := unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "rbac.authorization.k8s.io/v1",
			"kind":       "ClusterRole",
			"metadata": map[string]interface{}{
				"name": "my-role",
			},
			"rules": []interface{}{
				map[string]interface{}{
					"apiGroups": []interface{}{""},
					"resources": []interface{}{"pods"},
					"verbs":     []interface{}{"get", "list"},
				},
			},
		},
	}

	skeleton, patch, err := SplitNewResourceToSkeletonAndPatch(resource)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if skeleton.GetName() != "my-role" {
		t.Errorf("skeleton name: got %q", skeleton.GetName())
	}
	if skeleton.GetNamespace() != "" {
		t.Errorf("skeleton should have no namespace for cluster-scoped, got %q", skeleton.GetNamespace())
	}

	// Patch should add "rules"
	if patch == nil || len(patch) != 1 {
		t.Errorf("expected 1 patch op (rules), got %v", patch)
	}

	// Verify roundtrip
	skelJSON, _ := skeleton.MarshalJSON()
	patched, err := patch.Apply(skelJSON)
	if err != nil {
		t.Fatalf("failed to apply patch: %v", err)
	}
	var result map[string]interface{}
	if err := json.Unmarshal(patched, &result); err != nil {
		t.Fatalf("failed to unmarshal patched resource: %v", err)
	}
	rules, ok := result["rules"].([]interface{})
	if !ok || len(rules) != 1 {
		t.Errorf("patched resource should have 1 rule")
	}
}

func TestSplitNewResourceToSkeletonAndPatch_NilObject(t *testing.T) {
	resource := unstructured.Unstructured{}

	_, _, err := SplitNewResourceToSkeletonAndPatch(resource)
	if err == nil {
		t.Fatal("expected error for nil Object")
	}
}

func TestSplitNewResourceToSkeletonAndPatch_ComplexSpec(t *testing.T) {
	resource := unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "shipwright.io/v1beta1",
			"kind":       "Build",
			"metadata": map[string]interface{}{
				"name":      "complex-build",
				"namespace": "prod",
				"labels": map[string]interface{}{
					"app":     "complex",
					"version": "v2",
				},
			},
			"spec": map[string]interface{}{
				"source": map[string]interface{}{
					"type": "Git",
					"git": map[string]interface{}{
						"url": "https://github.com/example/complex",
						"ref": "main",
					},
				},
				"strategy": map[string]interface{}{
					"name": "buildpacks-v3",
					"kind": "ClusterBuildStrategy",
				},
				"output": map[string]interface{}{
					"image":       "quay.io/example/complex:latest",
					"credentials": map[string]interface{}{"name": "registry-cred"},
				},
				"env": []interface{}{
					map[string]interface{}{"name": "GO111MODULE", "value": "on"},
					map[string]interface{}{"name": "CGO_ENABLED", "value": "0"},
				},
			},
		},
	}

	skeleton, patch, err := SplitNewResourceToSkeletonAndPatch(resource)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify skeleton is minimal
	if len(skeleton.Object) != 3 { // apiVersion, kind, metadata
		t.Errorf("skeleton should have 3 top-level keys, got %d: %v", len(skeleton.Object), keysOf(skeleton.Object))
	}
	skelMeta, _ := skeleton.Object["metadata"].(map[string]interface{})
	if len(skelMeta) != 2 { // name, namespace
		t.Errorf("skeleton metadata should have 2 keys, got %d: %v", len(skelMeta), keysOf(skelMeta))
	}

	// Verify roundtrip: skeleton + patch = original
	skelJSON, _ := skeleton.MarshalJSON()
	patched, err := patch.Apply(skelJSON)
	if err != nil {
		t.Fatalf("failed to apply patch: %v", err)
	}

	var result unstructured.Unstructured
	if err := result.UnmarshalJSON(patched); err != nil {
		t.Fatalf("failed to unmarshal patched resource: %v", err)
	}

	// Check complex nested values survived
	env, _, _ := unstructured.NestedSlice(result.Object, "spec", "env")
	if len(env) != 2 {
		t.Errorf("expected 2 env vars after patch, got %d", len(env))
	}

	strategyName, _, _ := unstructured.NestedString(result.Object, "spec", "strategy", "name")
	if strategyName != "buildpacks-v3" {
		t.Errorf("expected strategy name 'buildpacks-v3', got %q", strategyName)
	}
}

func TestSplitNewResourceToSkeletonAndPatch_JSONPointerEscaping(t *testing.T) {
	resource := unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "ConfigMap",
			"metadata": map[string]interface{}{
				"name":      "escape-test",
				"namespace": "default",
				"annotations": map[string]interface{}{
					"example.com/foo":  "slash-value",
					"tilde~annotation": "tilde-value",
					"both~/combined":   "both-value",
				},
			},
			"data": map[string]interface{}{
				"key": "value",
			},
		},
	}

	skeleton, patch, err := SplitNewResourceToSkeletonAndPatch(resource)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if patch == nil {
		t.Fatal("patch should not be nil")
	}

	skelJSON, _ := skeleton.MarshalJSON()
	patched, err := patch.Apply(skelJSON)
	if err != nil {
		t.Fatalf("failed to apply patch to skeleton: %v", err)
	}

	var result unstructured.Unstructured
	if err := result.UnmarshalJSON(patched); err != nil {
		t.Fatalf("failed to unmarshal patched resource: %v", err)
	}

	annotations := result.GetAnnotations()
	if annotations["example.com/foo"] != "slash-value" {
		t.Errorf("annotation with / not preserved, got %v", annotations)
	}
	if annotations["tilde~annotation"] != "tilde-value" {
		t.Errorf("annotation with ~ not preserved, got %v", annotations)
	}
	if annotations["both~/combined"] != "both-value" {
		t.Errorf("annotation with ~ and / not preserved, got %v", annotations)
	}
}

func TestSplitNewResourceToSkeletonAndPatch_RoundtripEquality(t *testing.T) {
	resource := unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "shipwright.io/v1beta1",
			"kind":       "Build",
			"metadata": map[string]interface{}{
				"name":      "roundtrip-build",
				"namespace": "prod",
				"labels": map[string]interface{}{
					"app":     "roundtrip",
					"version": "v1",
				},
				"annotations": map[string]interface{}{
					"source-kind": "BuildConfig",
				},
			},
			"spec": map[string]interface{}{
				"source": map[string]interface{}{
					"type": "Git",
					"git": map[string]interface{}{
						"url": "https://github.com/example/repo",
					},
				},
				"output": map[string]interface{}{
					"image": "quay.io/example/roundtrip:latest",
				},
			},
		},
	}

	skeleton, patch, err := SplitNewResourceToSkeletonAndPatch(resource)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	skelJSON, _ := skeleton.MarshalJSON()
	patched, err := patch.Apply(skelJSON)
	if err != nil {
		t.Fatalf("failed to apply patch: %v", err)
	}

	var result unstructured.Unstructured
	if err := result.UnmarshalJSON(patched); err != nil {
		t.Fatalf("failed to unmarshal patched resource: %v", err)
	}

	originalJSON, _ := json.Marshal(resource.Object)
	resultJSON, _ := json.Marshal(result.Object)

	var originalNormalized, resultNormalized interface{}
	if err := json.Unmarshal(originalJSON, &originalNormalized); err != nil {
		t.Fatalf("failed to unmarshal original JSON: %v", err)
	}
	if err := json.Unmarshal(resultJSON, &resultNormalized); err != nil {
		t.Fatalf("failed to unmarshal result JSON: %v", err)
	}

	if !reflect.DeepEqual(originalNormalized, resultNormalized) {
		t.Errorf("roundtrip mismatch:\noriginal: %s\nresult:   %s", originalJSON, resultJSON)
	}
}

func keysOf(m map[string]interface{}) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}
