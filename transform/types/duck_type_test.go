package types_test

import (
	"fmt"
	"testing"

	"github.com/konveyor/crane-lib/transforms/types"
	v1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
)

func deploymentToUnstructured() unstructured.Unstructured {
	d := v1.Deployment{
		Spec: v1.DeploymentSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Image: "Hello World",
						},
					},
				},
			},
		},
	}
	u, err := runtime.DefaultUnstructuredConverter.ToUnstructured(&d)
	if err != nil {
		fmt.Printf("%v", err)
	}
	unstruc := unstructured.Unstructured{}
	unstruc.Object = u
	fmt.Printf("u: %v", u)
	return unstruc
}

func TestIsPodSpecable(t *testing.T) {
	cases := []struct {
		Name          string
		Object        unstructured.Unstructured
		IsPodSpecable bool
	}{
		{
			Name: "IsPodSpecable",
			Object: unstructured.Unstructured{
				Object: map[string]interface{}{
					"spec": map[string]interface{}{
						"template": map[string]interface{}{
							"image": "testImage",
						},
					},
				},
			},
			IsPodSpecable: true,
		},
		{
			Name: "IsNotPodSpecable",
			Object: unstructured.Unstructured{
				Object: map[string]interface{}{
					"spec": map[string]interface{}{
						"template1": map[string]interface{}{
							"image": "testImage",
						},
					},
				},
			},
			IsPodSpecable: false,
		},
		{
			Name: "IsNotPodSpecableNoSpec",
			Object: unstructured.Unstructured{
				Object: map[string]interface{}{},
			},
			IsPodSpecable: false,
		},
		{
			Name: "IsNotPodSpecableInvalidSpec",
			Object: unstructured.Unstructured{
				Object: map[string]interface{}{
					"spec": "testing",
				},
			},
			IsPodSpecable: false,
		},
		{
			Name: "IsNotPodSpecableInvalidTemplate",
			Object: unstructured.Unstructured{
				Object: map[string]interface{}{
					"spec": map[string]interface{}{
						"template": "hello world",
					},
				},
			},
			IsPodSpecable: false,
		},
		{
			Name: "IsNotPodSpecableInvalidObject",
			Object: unstructured.Unstructured{
				Object: map[string]interface{}{
					"spec": map[string]interface{}{
						"template": map[string]interface{}{
							"spec": "testing",
						},
					},
				},
			},
			IsPodSpecable: false,
		},
		{
			Name:          "IsNotPodSpecableDeplyment",
			Object:        deploymentToUnstructured(),
			IsPodSpecable: true,
		},
	}

	for _, c := range cases {
		t.Run(c.Name, func(t *testing.T) {
			_, isPodSpecable := types.IsPodSpecable(c.Object)
			if isPodSpecable != c.IsPodSpecable {
				t.Errorf("podSpecable is not correct, actual: %v, expected: %v", isPodSpecable, c.IsPodSpecable)
			}
		})
	}

}
