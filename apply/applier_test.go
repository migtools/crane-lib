package apply_test

import (
	"reflect"
	"testing"

	"github.com/konveyor/crane-lib/apply"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestApplierApply(t *testing.T) {
	cases := []struct {
		Name           string
		Object         unstructured.Unstructured
		Patch          string
		ShouldErr      bool
		ExpectedObject unstructured.Unstructured
	}{
		{
			Name:           "TestNoPatch",
			Object:         unstructured.Unstructured{},
			Patch:          "",
			ShouldErr:      true,
			ExpectedObject: unstructured.Unstructured{},
		},
		{
			Name:           "TestInvalidPatch",
			Object:         unstructured.Unstructured{},
			Patch:          `[}]`,
			ShouldErr:      true,
			ExpectedObject: unstructured.Unstructured{},
		},
		{
			Name: "NewAnnotations",
			Object: unstructured.Unstructured{
				Object: map[string]interface{}{
					"kind":       "DumbThing",
					"apiVersion": "test.io/v1",
					"metadata": map[string]interface{}{
						"name": "test-thing",
					},
				},
			},
			Patch:     `[{"op": "add", "path": "/metadata/annotations/test-new", "value": "new"}]`,
			ShouldErr: false,
			ExpectedObject: unstructured.Unstructured{
				Object: map[string]interface{}{
					"kind":       "DumbThing",
					"apiVersion": "test.io/v1",
					"metadata": map[string]interface{}{
						"name": "test-thing",
						"annotations": map[string]interface{}{
							"test-new": "new",
						},
					},
				},
			},
		},
		{
			Name: "AddAnnotations",
			Object: unstructured.Unstructured{
				Object: map[string]interface{}{
					"kind":       "DumbThing",
					"apiVersion": "test.io/v1",
					"metadata": map[string]interface{}{
						"name": "test-thing",
						"annotations": map[string]interface{}{
							"test-new": "new",
						},
					},
				},
			},
			Patch:     `[{"op": "add", "path": "/metadata/annotations/test-new21", "value": "new21"}]`,
			ShouldErr: false,
			ExpectedObject: unstructured.Unstructured{
				Object: map[string]interface{}{
					"kind":       "DumbThing",
					"apiVersion": "test.io/v1",
					"metadata": map[string]interface{}{
						"name": "test-thing",
						"annotations": map[string]interface{}{
							"test-new":   "new",
							"test-new21": "new21",
						},
					},
				},
			},
		},
	}

	for _, c := range cases {
		t.Run(c.Name, func(t *testing.T) {
			a := apply.Applier{}

			doc, err := a.Apply(c.Object, []byte(c.Patch))
			if err != nil && !c.ShouldErr {
				t.Fatalf("unable to run apply - %v", err)
			}

			if err != nil && c.ShouldErr {
				return
			}

			u := unstructured.Unstructured{}

			err = u.UnmarshalJSON(doc)
			if err != nil {
				t.Fatalf("unable to use output json - %v", err)
			}

			if !reflect.DeepEqual(u.Object, c.ExpectedObject.Object) {
				t.Fatalf("Object did not match expected output")
			}
		})
	}
}
