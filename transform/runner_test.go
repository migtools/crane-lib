package transform

import (
	"encoding/json"
	"fmt"
	"testing"

	jsonpatch "github.com/evanphx/json-patch"
	internaljsonpatch "github.com/konveyor/crane-lib/transform/internal/jsonpatch"
	"github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

type fakePlugin struct {
	Func func(u *unstructured.Unstructured) (PluginResponse, error)
	name string
}

func (fp fakePlugin) Run(u *unstructured.Unstructured) (PluginResponse, error) {
	return fp.Func(u)
}
func (fp fakePlugin) Name() string {
	return fp.name
}

func TestRunnerRun(t *testing.T) {
	cases := []struct {
		Name                 string
		Plugins              []Plugin
		Object               unstructured.Unstructured
		PatchesString        string
		IgnoredPatchesString string
		PluginPriorities     map[string]int
		IsWhiteOut           bool
		ShouldError          bool
	}{
		{
			Name:   "RunWithNoPlugins",
			Object: unstructured.Unstructured{},
		},
		{
			Name:   "RunWithNoPluginResponse",
			Object: unstructured.Unstructured{},
			Plugins: []Plugin{
				fakePlugin{
					Func: func(u *unstructured.Unstructured) (PluginResponse, error) {
					return PluginResponse{
						Version:    "v1",
						IsWhiteOut: false,
						Patches:    []jsonpatch.Operation{},
					}, nil
				},
					name: "",
				},
			},
			PatchesString: `[]`,
		},
		{
			Name:   "RunWithOneWhiteoutPlugin",
			Object: unstructured.Unstructured{},
			Plugins: []Plugin{
				fakePlugin{
					Func: func(u *unstructured.Unstructured) (PluginResponse, error) {
					return PluginResponse{
						IsWhiteOut: true,
					}, nil
				},
					name: "",
				},
			},
			IsWhiteOut: true,
		},
		{
			Name:   "RunWithOnePatchPlugin",
			Object: unstructured.Unstructured{},
			Plugins: []Plugin{
				fakePlugin{
					Func: func(u *unstructured.Unstructured) (PluginResponse, error) {
					p, err := jsonpatch.DecodePatch([]byte(`[{"op": "add", "path": "/spec/testing", "value": "test"}]`))
					if err != nil {
						return PluginResponse{}, err
					}
					return PluginResponse{
						Patches: p,
					}, nil
				},
					name: "",
				},
			},
			PatchesString: `[{"op": "add", "path": "/spec/testing", "value": "test"}]`,
		},
		{
			Name:   "RunWithOneErrorPlugin",
			Object: unstructured.Unstructured{},
			Plugins: []Plugin{
				fakePlugin{
					Func: func(u *unstructured.Unstructured) (PluginResponse, error) {
					return PluginResponse{}, fmt.Errorf("Adding a new error to test handling of error")
				},
					name: "",
				},
			},
			ShouldError: true,
		},
		{
			Name: "RunWithTwoPluginsOneWithMutation",
			Object: unstructured.Unstructured{
				Object: map[string]interface{}{
					"kind":       "Test",
					"apiVersion": "group.testing.io/v1alpha1",
				},
			},
			Plugins: []Plugin{
				fakePlugin{
					Func: func(u *unstructured.Unstructured) (PluginResponse, error) {
					u.SetGroupVersionKind(schema.GroupVersionKind{
						Group:   "group.testing.io",
						Version: "v1",
						Kind:    "Test",
					})
					return PluginResponse{}, nil
				},
					name: "",
				},
				fakePlugin{
					Func: func(u *unstructured.Unstructured) (PluginResponse, error) {
					gvk := schema.GroupVersionKind{
						Group:   "group.testing.io",
						Version: "v1alpha1",
						Kind:    "Test",
					}
					if u.GroupVersionKind() == gvk {
						return PluginResponse{}, nil
					}
					return PluginResponse{}, fmt.Errorf("Plugin was able to change the object")
				},
					name: "",
				},
			},
			ShouldError: false,
		},
		{
			Name:   "RunWithTwoPluginsAddingPatches",
			Object: unstructured.Unstructured{},
			Plugins: []Plugin{
				fakePlugin{
					Func: func(u *unstructured.Unstructured) (PluginResponse, error) {
					p, err := jsonpatch.DecodePatch([]byte(`[{"op": "add", "path": "/spec/testing", "value": "test"}]`))
					if err != nil {
						return PluginResponse{}, err
					}
					return PluginResponse{
						Patches: p,
					}, nil
				},
					name: "",
				},
				fakePlugin{
					Func: func(u *unstructured.Unstructured) (PluginResponse, error) {
					p, err := jsonpatch.DecodePatch([]byte(`[{"op": "add", "path": "/spec/newValue", "value": "test"}]`))
					if err != nil {
						return PluginResponse{}, err
					}
					return PluginResponse{
						Patches: p,
					}, nil
				},
					name: "",
				},
			},
			PatchesString: `[{"op": "add", "path": "/spec/newValue", "value": "test"},{"op": "add", "path": "/spec/testing", "value": "test"}]`,
		},
		{
			Name:   "RunWithTwoPluginsDuplicatePatches",
			Object: unstructured.Unstructured{},
			Plugins: []Plugin{
				fakePlugin{
					Func: func(u *unstructured.Unstructured) (PluginResponse, error) {
					p, err := jsonpatch.DecodePatch([]byte(`[{"op": "add", "path": "/spec/testing", "value": "test"}]`))
					if err != nil {
						return PluginResponse{}, err
					}
					return PluginResponse{
						Patches: p,
					}, nil
				},
					name: "",
				},
				fakePlugin{
					Func: func(u *unstructured.Unstructured) (PluginResponse, error) {
					p, err := jsonpatch.DecodePatch([]byte(`[{"op": "add", "path": "/spec/testing", "value": "test"}]`))
					if err != nil {
						return PluginResponse{}, err
					}
					return PluginResponse{
						Patches: p,
					}, nil
				},
					name: "",
				},
			},
			PatchesString: `[{"op": "add", "path": "/spec/testing", "value": "test"}]`,
		},
		{
			Name:   "RunWithTwoPluginsCollidedPatches",
			Object: unstructured.Unstructured{},
			Plugins: []Plugin{
				fakePlugin{
					Func: func(u *unstructured.Unstructured) (PluginResponse, error) {
					p, err := jsonpatch.DecodePatch([]byte(`[{"op": "add", "path": "/spec/testing", "value": "test"}]`))
					if err != nil {
						return PluginResponse{}, err
					}
					return PluginResponse{
						Patches: p,
					}, nil
				},
					name: "plugin1",
				},
				fakePlugin{
					Func: func(u *unstructured.Unstructured) (PluginResponse, error) {
					p, err := jsonpatch.DecodePatch([]byte(`[{"op": "add", "path": "/spec/testing", "value": "test1"}]`))
					if err != nil {
						return PluginResponse{}, err
					}
					return PluginResponse{
						Patches: p,
					}, nil
				},
					name: "plugin2",
				},
			},
			PatchesString:        `[{"op": "add", "path": "/spec/testing", "value": "test"}]`,
			IgnoredPatchesString: `[{"PluginName": "plugin2", "Operation": {"op": "add", "path": "/spec/testing", "value": "test1"}}]`,
		},
		{
			Name:   "RunWithTwoPluginsCollidedPatchesPriorityToSecond",
			Object: unstructured.Unstructured{},
			Plugins: []Plugin{
				fakePlugin{
					Func: func(u *unstructured.Unstructured) (PluginResponse, error) {
					p, err := jsonpatch.DecodePatch([]byte(`[{"op": "add", "path": "/spec/testing", "value": "test"}]`))
					if err != nil {
						return PluginResponse{}, err
					}
					return PluginResponse{
						Patches: p,
					}, nil
				},
					name: "plugin1",
				},
				fakePlugin{
					Func: func(u *unstructured.Unstructured) (PluginResponse, error) {
					p, err := jsonpatch.DecodePatch([]byte(`[{"op": "add", "path": "/spec/testing", "value": "test1"}]`))
					if err != nil {
						return PluginResponse{}, err
					}
					return PluginResponse{
						Patches: p,
					}, nil
				},
					name: "plugin2",
				},
			},
			PluginPriorities: map[string]int{
				"plugin2": 0,
			},
			PatchesString:        `[{"op": "add", "path": "/spec/testing", "value": "test1"}]`,
			IgnoredPatchesString: `[{"PluginName": "plugin1", "Operation": {"op": "add", "path": "/spec/testing", "value": "test"}}]`,
		},
	}

	for _, c := range cases {
		t.Run(c.Name, func(t *testing.T) {
			runner := Runner{
				Log:              logrus.New(),
				PluginPriorities: c.PluginPriorities,
			}
			response, err := runner.Run(c.Object, c.Plugins)
			if err != nil && !c.ShouldError {
				t.Error(err)
			}
			if response.HaveWhiteOut != c.IsWhiteOut {
				t.Errorf("incorrect white out determination, actual: %v expected: %v", response.HaveWhiteOut, c.IsWhiteOut)
			}

			// Two Bytes tells us that it is an empty list
			if len(c.PatchesString) != 0 || len(response.TransformFile) > 2 {
				p, err := jsonpatch.DecodePatch([]byte(c.PatchesString))
				if err != nil {
					t.Error(err)
				}
				p2, err := jsonpatch.DecodePatch(response.TransformFile)
				if err != nil {
					fmt.Printf("\n\n%v", string(response.TransformFile))
					t.Error(err)
				}

				if ok, err := internaljsonpatch.Equal(p2, p); !ok || err != nil {
					t.Errorf("incorrect jsonpathc, actual: %v expected: %v\nerror: %v", string(response.TransformFile), c.PatchesString, err)
				}
			}
			// Two Bytes tells us that it is an empty list
			if len(c.IgnoredPatchesString) != 0 || len(response.IgnoredPatches) > 2 {
				ignoredPluginOperations := []PluginOperation{}
				err := json.Unmarshal(response.IgnoredPatches, &ignoredPluginOperations)
				if err != nil {
					t.Error(err)
				}
				expectedIgnoredPluginOperations := []PluginOperation{}
				err = json.Unmarshal([]byte(c.IgnoredPatchesString), &expectedIgnoredPluginOperations)
				if err != nil {
					t.Error(err)
				}
				if ok := EqualPluginOperationList(ignoredPluginOperations, expectedIgnoredPluginOperations); !ok || err != nil {
					t.Errorf("incorrect plugin operations, actual: %v expected: %v", string(response.IgnoredPatches), c.IgnoredPatchesString)
				}
			}
		})
	}

}
