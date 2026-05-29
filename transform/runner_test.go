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
	Func func(request PluginRequest) (PluginResponse, error)
	name string
}

func (fp fakePlugin) Run(request PluginRequest) (PluginResponse, error) {
	return fp.Func(request)
}

func (fp fakePlugin) Metadata() PluginMetadata {
	return PluginMetadata{Name: fp.name}
}

func TestRunnerRun(t *testing.T) {
	cases := []struct {
		Name                 string
		Plugins              []Plugin
		Object               unstructured.Unstructured
		PatchesString        string
		IgnoredPatchesString string
		PluginPriorities     map[string]int
		OptionalFlags        map[string]string
		IsWhiteOut           bool
		ShouldError          bool
		ExpectedNewResources int
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
					Func: func(request PluginRequest) (PluginResponse, error) {
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
					Func: func(request PluginRequest) (PluginResponse, error) {
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
					Func: func(request PluginRequest) (PluginResponse, error) {
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
					Func: func(request PluginRequest) (PluginResponse, error) {
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
					Func: func(request PluginRequest) (PluginResponse, error) {
						request.Unstructured.SetGroupVersionKind(schema.GroupVersionKind{
							Group:   "group.testing.io",
							Version: "v1",
							Kind:    "Test",
						})
						return PluginResponse{}, nil
					},
					name: "",
				},
				fakePlugin{
					Func: func(request PluginRequest) (PluginResponse, error) {
						gvk := schema.GroupVersionKind{
							Group:   "group.testing.io",
							Version: "v1alpha1",
							Kind:    "Test",
						}
						if request.Unstructured.GroupVersionKind() == gvk {
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
					Func: func(request PluginRequest) (PluginResponse, error) {
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
					Func: func(request PluginRequest) (PluginResponse, error) {
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
					Func: func(request PluginRequest) (PluginResponse, error) {
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
					Func: func(request PluginRequest) (PluginResponse, error) {
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
					Func: func(request PluginRequest) (PluginResponse, error) {
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
					Func: func(request PluginRequest) (PluginResponse, error) {
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
					Func: func(request PluginRequest) (PluginResponse, error) {
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
					Func: func(request PluginRequest) (PluginResponse, error) {
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
		{
			Name:   "RunWithTwoPluginsCollidedPatchesDifferentOps",
			Object: unstructured.Unstructured{},
			Plugins: []Plugin{
				fakePlugin{
					Func: func(request PluginRequest) (PluginResponse, error) {
					p, err := jsonpatch.DecodePatch([]byte(`[{"op": "replace", "path": "/spec/testing", "value": "test"}]`))
					if err != nil {
						return PluginResponse{}, err
					}
					return PluginResponse{
						Patches: p,
					}, nil
				},
					name: "pluginreplace",
				},
				fakePlugin{
					Func: func(request PluginRequest) (PluginResponse, error) {
					p, err := jsonpatch.DecodePatch([]byte(`[{"op": "remove", "path": "/spec/testing"}]`))
					if err != nil {
						return PluginResponse{}, err
					}
					return PluginResponse{
						Patches: p,
					}, nil
				},
					name: "pluginremove",
				},
			},
			PatchesString:        `[{"op": "replace", "path": "/spec/testing", "value": "test"}]`,
			IgnoredPatchesString: `[{"PluginName": "pluginremove", "Operation": {"op": "remove", "path": "/spec/testing"}}]`,
		},
		{
			Name:   "RunWithPluginParsingOptionalFlags",
			Object: unstructured.Unstructured{},
			Plugins: []Plugin{
				fakePlugin{
					Func: func(request PluginRequest) (PluginResponse, error) {
						extraVal := request.Extras["testFlag"]
						p, err := jsonpatch.DecodePatch([]byte(`[{"op": "add", "path": "/spec/testing", "value": "` + extraVal + `"}]`))
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
			OptionalFlags: map[string]string{
				"testFlag": "testFlagValue",
			},
			PatchesString: `[{"op": "add", "path": "/spec/testing", "value": "testFlagValue"}]`,
		},
		{
			Name:   "RunWithPluginGeneratingSingleNewResource",
			Object: unstructured.Unstructured{},
			Plugins: []Plugin{
				fakePlugin{
					Func: func(request PluginRequest) (PluginResponse, error) {
						newResource := unstructured.Unstructured{
							Object: map[string]interface{}{
								"apiVersion": "shipwright.io/v1beta1",
								"kind":       "Build",
								"metadata": map[string]interface{}{
									"name": "myapp-build",
								},
								"spec": map[string]interface{}{},
							},
						}
						return PluginResponse{
							NewResources: []unstructured.Unstructured{newResource},
						}, nil
					},
					name: "buildconfig-converter",
				},
			},
			ExpectedNewResources: 1,
		},
		{
			Name:   "RunWithPluginGeneratingMultipleNewResources",
			Object: unstructured.Unstructured{},
			Plugins: []Plugin{
				fakePlugin{
					Func: func(request PluginRequest) (PluginResponse, error) {
						resource1 := unstructured.Unstructured{
							Object: map[string]interface{}{
								"apiVersion": "shipwright.io/v1beta1",
								"kind":       "Build",
								"metadata": map[string]interface{}{
									"name": "build-1",
								},
							},
						}
						resource2 := unstructured.Unstructured{
							Object: map[string]interface{}{
								"apiVersion": "tekton.dev/v1",
								"kind":       "Pipeline",
								"metadata": map[string]interface{}{
									"name": "pipeline-1",
								},
							},
						}
						resource3 := unstructured.Unstructured{
							Object: map[string]interface{}{
								"apiVersion": "v1",
								"kind":       "ConfigMap",
								"metadata": map[string]interface{}{
									"name": "config-1",
								},
							},
						}
						return PluginResponse{
							NewResources: []unstructured.Unstructured{resource1, resource2, resource3},
						}, nil
					},
					name: "multi-resource-generator",
				},
			},
			ExpectedNewResources: 3,
		},
		{
			Name:   "RunWithWhiteoutAndNewResource",
			Object: unstructured.Unstructured{},
			Plugins: []Plugin{
				fakePlugin{
					Func: func(request PluginRequest) (PluginResponse, error) {
						replacement := unstructured.Unstructured{
							Object: map[string]interface{}{
								"apiVersion": "apps/v1",
								"kind":       "Deployment",
								"metadata": map[string]interface{}{
									"name": "replacement-deployment",
								},
							},
						}
						return PluginResponse{
							IsWhiteOut:   true,
							NewResources: []unstructured.Unstructured{replacement},
						}, nil
					},
					name: "replacement-plugin",
				},
			},
			IsWhiteOut:           true,
			ExpectedNewResources: 1,
		},
		{
			Name:   "RunWithOldPluginBackwardCompatibility",
			Object: unstructured.Unstructured{},
			Plugins: []Plugin{
				fakePlugin{
					Func: func(request PluginRequest) (PluginResponse, error) {
						p, err := jsonpatch.DecodePatch([]byte(`[{"op": "add", "path": "/spec/replicas", "value": 3}]`))
						if err != nil {
							return PluginResponse{}, err
						}
						return PluginResponse{
							Version: "v1",
							Patches: p,
						}, nil
					},
					name: "old-plugin",
				},
			},
			PatchesString: `[{"op": "add", "path": "/spec/replicas", "value": 3}]`,
		},
		{
			Name:   "RunWithMultiplePluginsGeneratingResources",
			Object: unstructured.Unstructured{},
			Plugins: []Plugin{
				fakePlugin{
					Func: func(request PluginRequest) (PluginResponse, error) {
						resource := unstructured.Unstructured{
							Object: map[string]interface{}{
								"apiVersion": "v1",
								"kind":       "Service",
								"metadata": map[string]interface{}{
									"name": "service-1",
								},
							},
						}
						return PluginResponse{
							NewResources: []unstructured.Unstructured{resource},
						}, nil
					},
					name: "plugin1",
				},
				fakePlugin{
					Func: func(request PluginRequest) (PluginResponse, error) {
						resource := unstructured.Unstructured{
							Object: map[string]interface{}{
								"apiVersion": "v1",
								"kind":       "ConfigMap",
								"metadata": map[string]interface{}{
									"name": "configmap-1",
								},
							},
						}
						return PluginResponse{
							NewResources: []unstructured.Unstructured{resource},
						}, nil
					},
					name: "plugin2",
				},
			},
			ExpectedNewResources: 2,
		},
		{
			Name:   "RunWithPluginEmptyNewResources",
			Object: unstructured.Unstructured{},
			Plugins: []Plugin{
				fakePlugin{
					Func: func(request PluginRequest) (PluginResponse, error) {
						return PluginResponse{
							NewResources: []unstructured.Unstructured{},
						}, nil
					},
					name: "empty-resources-plugin",
				},
			},
		},
	}

	for _, c := range cases {
		t.Run(c.Name, func(t *testing.T) {
			runner := NewRunner(logrus.New(), c.PluginPriorities, c.OptionalFlags)
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
			// Verify NewResources count
			if len(response.NewResources) != c.ExpectedNewResources {
				t.Errorf("incorrect new resources count, actual: %v expected: %v", len(response.NewResources), c.ExpectedNewResources)
			}
		})
	}

}

func TestNewRunner(t *testing.T) {
	t.Run("WithLogger", func(t *testing.T) {
		logger := logrus.New()
		priorities := map[string]int{"plugin1": 1}
		flags := map[string]string{"flag1": "value1"}

		runner := NewRunner(logger, priorities, flags)

		if runner.Log != logger {
			t.Error("Log was not set correctly")
		}
		if runner.PluginPriorities["plugin1"] != 1 {
			t.Error("PluginPriorities was not set correctly")
		}
		if runner.OptionalFlags["flag1"] != "value1" {
			t.Error("OptionalFlags was not set correctly")
		}
	})

	t.Run("WithNilLogger", func(t *testing.T) {
		runner := NewRunner(nil, nil, nil)

		if runner.Log == nil {
			t.Error("Log should not be nil when nil logger is passed")
		}

		// Verify runner can execute without panic
		plugin := fakePlugin{
			Func: func(request PluginRequest) (PluginResponse, error) {
				newResource := unstructured.Unstructured{
					Object: map[string]interface{}{
						"apiVersion": "v1",
						"kind":       "ConfigMap",
						"metadata": map[string]interface{}{
							"name": "test",
						},
					},
				}
				return PluginResponse{
					NewResources: []unstructured.Unstructured{newResource},
				}, nil
			},
			name: "test-plugin",
		}
		response, err := runner.Run(unstructured.Unstructured{}, []Plugin{plugin})
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if len(response.NewResources) != 1 {
			t.Errorf("expected 1 new resource, got %d", len(response.NewResources))
		}
	})
}
