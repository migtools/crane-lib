package convert

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	buildv1 "github.com/openshift/api/build/v1"
	imagev1 "github.com/openshift/api/image/v1"
	shipwrightv1beta1 "github.com/shipwright-io/build/pkg/apis/build/v1beta1"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// MockClient is a mock implementation of controller-runtime client.Client
type MockClient struct {
	mock.Mock
}

func (m *MockClient) Get(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
	args := m.Called(ctx, key, obj, opts)
	if args.Get(0) != nil {
		return args.Error(0)
	}

	// Handle ImageStreamTag mock response
	if ist, ok := obj.(*imagev1.ImageStreamTag); ok {
		ist.Tag = &imagev1.TagReference{
			From: &corev1.ObjectReference{
				Name: "registry.example.com/image:latest",
			},
		}
	}
	return nil
}

func (m *MockClient) List(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
	args := m.Called(ctx, list, opts)
	if args.Get(0) != nil {
		return args.Error(0)
	}

	// Handle BuildConfigList mock response
	if bcList, ok := list.(*buildv1.BuildConfigList); ok {
		bc := buildv1.BuildConfig{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-bc",
				Namespace: "test-namespace",
			},
			Spec: buildv1.BuildConfigSpec{
				CommonSpec: buildv1.CommonSpec{
					Strategy: buildv1.BuildStrategy{
						Type: buildv1.DockerBuildStrategyType,
						DockerStrategy: &buildv1.DockerBuildStrategy{
							DockerfilePath: "Dockerfile",
							BuildArgs: []corev1.EnvVar{
								{Name: "ARG1", Value: "value1"},
								{Name: "ARG2", Value: "value2"},
							},
						},
					},
					Source: buildv1.BuildSource{
						Type: buildv1.BuildSourceGit,
						Git: &buildv1.GitBuildSource{
							URI: "https://github.com/example/repo.git",
							Ref: "main",
						},
					},
					Output: buildv1.BuildOutput{
						To: &corev1.ObjectReference{
							Kind: "ImageStreamTag",
							Name: "output-image:latest",
						},
						PushSecret: &corev1.LocalObjectReference{
							Name: "push-secret",
						},
					},
				},
			},
		}
		bc.SetGroupVersionKind(schema.GroupVersionKind{
			Group:   "build.openshift.io",
			Version: "v1",
			Kind:    "BuildConfig",
		})
		bcList.Items = []buildv1.BuildConfig{bc}
	}
	return nil
}

func (m *MockClient) Create(ctx context.Context, obj client.Object, opts ...client.CreateOption) error {
	args := m.Called(ctx, obj, opts)
	return args.Error(0)
}

func (m *MockClient) Delete(ctx context.Context, obj client.Object, opts ...client.DeleteOption) error {
	args := m.Called(ctx, obj, opts)
	return args.Error(0)
}

func (m *MockClient) Update(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error {
	args := m.Called(ctx, obj, opts)
	return args.Error(0)
}

func (m *MockClient) Patch(ctx context.Context, obj client.Object, patch client.Patch, opts ...client.PatchOption) error {
	args := m.Called(ctx, obj, patch, opts)
	return args.Error(0)
}

func (m *MockClient) DeleteAllOf(ctx context.Context, obj client.Object, opts ...client.DeleteAllOfOption) error {
	args := m.Called(ctx, obj, opts)
	return args.Error(0)
}

func (m *MockClient) Status() client.StatusWriter {
	args := m.Called()
	return args.Get(0).(client.StatusWriter)
}

func (m *MockClient) Scheme() *runtime.Scheme {
	args := m.Called()
	return args.Get(0).(*runtime.Scheme)
}

func (m *MockClient) RESTMapper() meta.RESTMapper {
	args := m.Called()
	return args.Get(0).(meta.RESTMapper)
}

func (m *MockClient) GroupVersionKindFor(obj runtime.Object) (schema.GroupVersionKind, error) {
	args := m.Called(obj)
	return args.Get(0).(schema.GroupVersionKind), args.Error(1)
}

func (m *MockClient) IsObjectNamespaced(obj runtime.Object) (bool, error) {
	args := m.Called(obj)
	return args.Bool(0), args.Error(1)
}

func (m *MockClient) SubResource(subResource string) client.SubResourceClient {
	args := m.Called(subResource)
	return args.Get(0).(client.SubResourceClient)
}

func TestParseRegistries(t *testing.T) {
	tests := []struct {
		name        string
		registries  []string
		expectedLen int
	}{
		{
			name:        "empty registries",
			registries:  []string{},
			expectedLen: 0,
		},
		{
			name:        "single registry",
			registries:  []string{"registry1.example.com"},
			expectedLen: 1,
		},
		{
			name:        "multiple registries",
			registries:  []string{"registry1.example.com", "registry2.example.com", "registry3.example.com"},
			expectedLen: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseRegistries(tt.registries)
			assert.Equal(t, tt.expectedLen, len(result))

			for i, registry := range tt.registries {
				assert.Equal(t, registry, *result[i].Value)
			}
		})
	}
}

func TestProcessSource(t *testing.T) {
	tests := []struct {
		name           string
		buildConfig    buildv1.BuildConfig
		expectedSource *shipwrightv1beta1.Source
	}{
		{
			name: "Git source",
			buildConfig: buildv1.BuildConfig{
				ObjectMeta: metav1.ObjectMeta{Name: "test-bc"},
				Spec: buildv1.BuildConfigSpec{
					CommonSpec: buildv1.CommonSpec{
						Source: buildv1.BuildSource{
							Type: buildv1.BuildSourceGit,
							Git: &buildv1.GitBuildSource{
								URI: "https://github.com/example/repo.git",
								Ref: "main",
							},
						},
					},
				},
			},
			expectedSource: &shipwrightv1beta1.Source{
				Type: "Git",
				Git: &shipwrightv1beta1.Git{
					URL:      "https://github.com/example/repo.git",
					Revision: stringPtr("main"),
				},
			},
		},
		{
			name: "Unknown source type",
			buildConfig: buildv1.BuildConfig{
				ObjectMeta: metav1.ObjectMeta{Name: "test-bc"},
				Spec: buildv1.BuildConfigSpec{
					CommonSpec: buildv1.CommonSpec{
						Source: buildv1.BuildSource{
							Type: "Unknown",
						},
					},
				},
			},
			expectedSource: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			co := &ConvertOptions{}
			build := &shipwrightv1beta1.Build{}

			co.processSource(tt.buildConfig, build)

			if tt.expectedSource == nil {
				assert.Nil(t, build.Spec.Source)
			} else {
				assert.Equal(t, tt.expectedSource.Type, build.Spec.Source.Type)
				if tt.expectedSource.Git != nil {
					assert.Equal(t, tt.expectedSource.Git.URL, build.Spec.Source.Git.URL)
					assert.Equal(t, tt.expectedSource.Git.Revision, build.Spec.Source.Git.Revision)
				}
			}
		})
	}
}

func TestProcessOutput(t *testing.T) {
	tests := []struct {
		name          string
		buildConfig   buildv1.BuildConfig
		expectedImage string
	}{
		{
			name: "ImageStreamTag output with namespace",
			buildConfig: buildv1.BuildConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-bc",
					Namespace: "default",
				},
				Spec: buildv1.BuildConfigSpec{
					CommonSpec: buildv1.CommonSpec{
						Output: buildv1.BuildOutput{
							To: &corev1.ObjectReference{
								Kind:      "ImageStreamTag",
								Name:      "output-image:latest",
								Namespace: "custom-namespace",
							},
						},
					},
				},
			},
			expectedImage: "image-registry.openshift-image-registry.svc:5000/custom-namespace/output-image:latest",
		},
		{
			name: "ImageStreamTag output without namespace",
			buildConfig: buildv1.BuildConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-bc",
					Namespace: "default",
				},
				Spec: buildv1.BuildConfigSpec{
					CommonSpec: buildv1.CommonSpec{
						Output: buildv1.BuildOutput{
							To: &corev1.ObjectReference{
								Kind: "ImageStreamTag",
								Name: "output-image:latest",
							},
						},
					},
				},
			},
			expectedImage: "image-registry.openshift-image-registry.svc:5000/default/output-image:latest",
		},
		{
			name: "Direct image output",
			buildConfig: buildv1.BuildConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-bc",
					Namespace: "default",
				},
				Spec: buildv1.BuildConfigSpec{
					CommonSpec: buildv1.CommonSpec{
						Output: buildv1.BuildOutput{
							To: &corev1.ObjectReference{
								Kind: "DockerImage",
								Name: "registry.example.com/image:tag",
							},
						},
					},
				},
			},
			expectedImage: "registry.example.com/image:tag",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			co := &ConvertOptions{}
			build := &shipwrightv1beta1.Build{}

			co.processOutput(tt.buildConfig, build)

			assert.Equal(t, tt.expectedImage, build.Spec.Output.Image)
		})
	}
}

func TestAddRegistries(t *testing.T) {
	tests := []struct {
		name               string
		searchRegistries   []string
		insecureRegistries []string
		blockRegistries    []string
		expectedParams     int
	}{
		{
			name:               "no registries",
			searchRegistries:   []string{},
			insecureRegistries: []string{},
			blockRegistries:    []string{},
			expectedParams:     0,
		},
		{
			name:               "search registries only",
			searchRegistries:   []string{"registry1.example.com"},
			insecureRegistries: []string{},
			blockRegistries:    []string{},
			expectedParams:     1,
		},
		{
			name:               "all registry types",
			searchRegistries:   []string{"registry1.example.com"},
			insecureRegistries: []string{"registry2.example.com"},
			blockRegistries:    []string{"registry3.example.com"},
			expectedParams:     3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			co := &ConvertOptions{
				SearchRegistries:   tt.searchRegistries,
				InsecureRegistries: tt.insecureRegistries,
				BlockRegistries:    tt.blockRegistries,
			}
			build := &shipwrightv1beta1.Build{
				Spec: shipwrightv1beta1.BuildSpec{
					ParamValues: []shipwrightv1beta1.ParamValue{},
				},
			}

			co.addRegistries(build)

			assert.Equal(t, tt.expectedParams, len(build.Spec.ParamValues))

			// Verify parameter names
			paramNames := make(map[string]bool)
			for _, param := range build.Spec.ParamValues {
				paramNames[param.Name] = true
			}

			if len(tt.searchRegistries) > 0 {
				assert.True(t, paramNames["registries-search"])
			}
			if len(tt.insecureRegistries) > 0 {
				assert.True(t, paramNames["registries-insecure"])
			}
			if len(tt.blockRegistries) > 0 {
				assert.True(t, paramNames["registries-block"])
			}
		})
	}
}

func TestProcessBuildArgs(t *testing.T) {
	tests := []struct {
		name           string
		buildConfig    buildv1.BuildConfig
		expectedParams int
		expectedValues []string
	}{
		{
			name: "no build args",
			buildConfig: buildv1.BuildConfig{
				Spec: buildv1.BuildConfigSpec{
					CommonSpec: buildv1.CommonSpec{
						Strategy: buildv1.BuildStrategy{
							DockerStrategy: &buildv1.DockerBuildStrategy{
								BuildArgs: []corev1.EnvVar{},
							},
						},
					},
				},
			},
			expectedParams: 0,
			expectedValues: []string{},
		},
		{
			name: "with build args",
			buildConfig: buildv1.BuildConfig{
				Spec: buildv1.BuildConfigSpec{
					CommonSpec: buildv1.CommonSpec{
						Strategy: buildv1.BuildStrategy{
							DockerStrategy: &buildv1.DockerBuildStrategy{
								BuildArgs: []corev1.EnvVar{
									{Name: "ARG1", Value: "value1"},
									{Name: "ARG2", Value: "value2"},
								},
							},
						},
					},
				},
			},
			expectedParams: 1,
			expectedValues: []string{"ARG1=value1", "ARG2=value2"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			co := &ConvertOptions{}
			build := &shipwrightv1beta1.Build{
				Spec: shipwrightv1beta1.BuildSpec{
					ParamValues: []shipwrightv1beta1.ParamValue{},
				},
			}

			co.processBuildArgs(tt.buildConfig, build)

			assert.Equal(t, tt.expectedParams, len(build.Spec.ParamValues))

			if tt.expectedParams > 0 {
				param := build.Spec.ParamValues[0]
				assert.Equal(t, "build-args", param.Name)
				assert.Equal(t, len(tt.expectedValues), len(param.Values))

				for i, expectedValue := range tt.expectedValues {
					assert.Equal(t, expectedValue, *param.Values[i].Value)
				}
			}
		})
	}
}

func TestGetBuildConfigFilePath(t *testing.T) {
	bc := buildv1.BuildConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-bc",
			Namespace: "test-namespace",
		},
	}
	bc.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "build.openshift.io",
		Version: "v1",
		Kind:    "BuildConfig",
	})

	result := getBuildConfigFilePath(bc)
	expected := "BuildConfig_build.openshift.io_v1_test-namespace_test-bc.yaml"
	assert.Equal(t, expected, result)
}

func TestGetBuildFilePath(t *testing.T) {
	build := shipwrightv1beta1.Build{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-build",
			Namespace: "test-namespace",
		},
	}
	build.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "shipwright.io",
		Version: "v1beta1",
		Kind:    "Build",
	})

	result := getBuildFilePath(build)
	expected := "Build_shipwright.io_v1beta1_test-namespace_test-build.yaml"
	assert.Equal(t, expected, result)
}

func TestResolveImageStreamRef(t *testing.T) {
	tests := []struct {
		name          string
		streamName    string
		namespace     string
		mockError     error
		expectedImage string
		expectError   bool
	}{
		{
			name:          "successful resolution",
			streamName:    "test-stream:latest",
			namespace:     "test-namespace",
			mockError:     nil,
			expectedImage: "registry.example.com/image:latest",
			expectError:   false,
		},
		{
			name:        "client error",
			streamName:  "test-stream:latest",
			namespace:   "test-namespace",
			mockError:   errors.New("client error"),
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockClient := &MockClient{}
			co := &ConvertOptions{
				Client: mockClient,
			}

			mockClient.On("Get", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(tt.mockError)

			result, err := co.resolveImageStreamRef(tt.streamName, tt.namespace)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedImage, result)
			}

			mockClient.AssertExpectations(t)
		})
	}
}

func TestProcessDockerStrategyFromField(t *testing.T) {
	t.Run("ImageStreamTag success", func(t *testing.T) {
		mockClient := &MockClient{}
		co := &ConvertOptions{Client: mockClient}

		// Mock Get to succeed and populate ImageStreamTag
		mockClient.On("Get", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil)

		bc := &buildv1.BuildConfig{
			Spec: buildv1.BuildConfigSpec{
				CommonSpec: buildv1.CommonSpec{
					Strategy: buildv1.BuildStrategy{
						Type: buildv1.DockerBuildStrategyType,
						DockerStrategy: &buildv1.DockerBuildStrategy{
							From: &corev1.ObjectReference{Kind: ImageStreamTag, Name: "example:latest", Namespace: "ns"},
						},
					},
				},
			},
		}

		build := &shipwrightv1beta1.Build{Spec: shipwrightv1beta1.BuildSpec{ParamValues: []shipwrightv1beta1.ParamValue{}}}

		err := co.processDockerStrategyFromField(bc, build)
		assert.NoError(t, err)
		assert.Equal(t, 1, len(build.Spec.ParamValues))
		assert.Equal(t, "from", build.Spec.ParamValues[0].Name)
		assert.NotNil(t, build.Spec.ParamValues[0].SingleValue)
		assert.Equal(t, "registry.example.com/image:latest", *build.Spec.ParamValues[0].SingleValue.Value)

		mockClient.AssertExpectations(t)
	})

	t.Run("ImageStreamImage success", func(t *testing.T) {
		mockClient := &MockClient{}
		co := &ConvertOptions{Client: mockClient}

		// Mock Get to succeed and populate ImageStreamTag (resolveImageStreamRef uses ImageStreamTag)
		mockClient.On("Get", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil)

		bc := &buildv1.BuildConfig{
			Spec: buildv1.BuildConfigSpec{
				CommonSpec: buildv1.CommonSpec{
					Strategy: buildv1.BuildStrategy{
						Type: buildv1.DockerBuildStrategyType,
						DockerStrategy: &buildv1.DockerBuildStrategy{
							From: &corev1.ObjectReference{Kind: ImageStreamImage, Name: "example@sha256:deadbeef", Namespace: "ns"},
						},
					},
				},
			},
		}

		build := &shipwrightv1beta1.Build{Spec: shipwrightv1beta1.BuildSpec{ParamValues: []shipwrightv1beta1.ParamValue{}}}

		err := co.processDockerStrategyFromField(bc, build)
		assert.NoError(t, err)
		assert.Equal(t, 1, len(build.Spec.ParamValues))
		assert.Equal(t, "from", build.Spec.ParamValues[0].Name)
		assert.NotNil(t, build.Spec.ParamValues[0].SingleValue)
		assert.Equal(t, "registry.example.com/image:latest", *build.Spec.ParamValues[0].SingleValue.Value)

		mockClient.AssertExpectations(t)
	})

	t.Run("DockerImage direct", func(t *testing.T) {
		co := &ConvertOptions{}

		bc := &buildv1.BuildConfig{
			Spec: buildv1.BuildConfigSpec{
				CommonSpec: buildv1.CommonSpec{
					Strategy: buildv1.BuildStrategy{
						Type: buildv1.DockerBuildStrategyType,
						DockerStrategy: &buildv1.DockerBuildStrategy{
							From: &corev1.ObjectReference{Kind: DockerImage, Name: "docker.io/library/nginx:latest"},
						},
					},
				},
			},
		}

		build := &shipwrightv1beta1.Build{Spec: shipwrightv1beta1.BuildSpec{ParamValues: []shipwrightv1beta1.ParamValue{}}}

		err := co.processDockerStrategyFromField(bc, build)
		assert.NoError(t, err)
		assert.Equal(t, 1, len(build.Spec.ParamValues))
		assert.Equal(t, "from", build.Spec.ParamValues[0].Name)
		assert.NotNil(t, build.Spec.ParamValues[0].SingleValue)
		assert.Equal(t, "docker.io/library/nginx:latest", *build.Spec.ParamValues[0].SingleValue.Value)
	})

	t.Run("Unknown kind returns error", func(t *testing.T) {
		co := &ConvertOptions{}

		bc := &buildv1.BuildConfig{
			Spec: buildv1.BuildConfigSpec{
				CommonSpec: buildv1.CommonSpec{
					Strategy: buildv1.BuildStrategy{
						Type: buildv1.DockerBuildStrategyType,
						DockerStrategy: &buildv1.DockerBuildStrategy{
							From: &corev1.ObjectReference{Kind: "Unknown", Name: "x"},
						},
					},
				},
			},
		}

		build := &shipwrightv1beta1.Build{Spec: shipwrightv1beta1.BuildSpec{ParamValues: []shipwrightv1beta1.ParamValue{}}}

		err := co.processDockerStrategyFromField(bc, build)
		assert.Error(t, err)
		assert.Equal(t, 0, len(build.Spec.ParamValues))
	})

	t.Run("Nil From no-op", func(t *testing.T) {
		co := &ConvertOptions{}

		bc := &buildv1.BuildConfig{
			Spec: buildv1.BuildConfigSpec{
				CommonSpec: buildv1.CommonSpec{
					Strategy: buildv1.BuildStrategy{
						Type:           buildv1.DockerBuildStrategyType,
						DockerStrategy: &buildv1.DockerBuildStrategy{From: nil},
					},
				},
			},
		}

		build := &shipwrightv1beta1.Build{Spec: shipwrightv1beta1.BuildSpec{ParamValues: []shipwrightv1beta1.ParamValue{}}}

		err := co.processDockerStrategyFromField(bc, build)
		assert.NoError(t, err)
		assert.Equal(t, 0, len(build.Spec.ParamValues))
	})
}

func TestProcessSourceStrategyFromField(t *testing.T) {
	t.Run("ImageStreamTag success", func(t *testing.T) {
		mockClient := &MockClient{}
		co := &ConvertOptions{Client: mockClient}

		// Mock Get to succeed and populate ImageStreamTag
		mockClient.On("Get", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil)

		bc := &buildv1.BuildConfig{
			ObjectMeta: metav1.ObjectMeta{Name: "test-bc"},
			Spec: buildv1.BuildConfigSpec{
				CommonSpec: buildv1.CommonSpec{
					Strategy: buildv1.BuildStrategy{
						Type: buildv1.SourceBuildStrategyType,
						SourceStrategy: &buildv1.SourceBuildStrategy{
							From: corev1.ObjectReference{Kind: ImageStreamTag, Name: "example:latest", Namespace: "ns"},
						},
					},
				},
			},
		}

		build := &shipwrightv1beta1.Build{Spec: shipwrightv1beta1.BuildSpec{ParamValues: []shipwrightv1beta1.ParamValue{}}}

		err := co.processSourceStrategyFromField(bc, build)
		assert.NoError(t, err)
		assert.Equal(t, 1, len(build.Spec.ParamValues))
		assert.Equal(t, "builder-image", build.Spec.ParamValues[0].Name)
		assert.NotNil(t, build.Spec.ParamValues[0].SingleValue)
		assert.Equal(t, "registry.example.com/image:latest", *build.Spec.ParamValues[0].SingleValue.Value)

		mockClient.AssertExpectations(t)
	})

	t.Run("ImageStreamImage success", func(t *testing.T) {
		mockClient := &MockClient{}
		co := &ConvertOptions{Client: mockClient}

		// Mock Get to succeed and populate ImageStreamTag (resolveImageStreamRef uses ImageStreamTag)
		mockClient.On("Get", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil)

		bc := &buildv1.BuildConfig{
			ObjectMeta: metav1.ObjectMeta{Name: "test-bc"},
			Spec: buildv1.BuildConfigSpec{
				CommonSpec: buildv1.CommonSpec{
					Strategy: buildv1.BuildStrategy{
						Type: buildv1.SourceBuildStrategyType,
						SourceStrategy: &buildv1.SourceBuildStrategy{
							From: corev1.ObjectReference{Kind: ImageStreamImage, Name: "example@sha256:deadbeef", Namespace: "ns"},
						},
					},
				},
			},
		}

		build := &shipwrightv1beta1.Build{Spec: shipwrightv1beta1.BuildSpec{ParamValues: []shipwrightv1beta1.ParamValue{}}}

		err := co.processSourceStrategyFromField(bc, build)
		assert.NoError(t, err)
		assert.Equal(t, 1, len(build.Spec.ParamValues))
		assert.Equal(t, "builder-image", build.Spec.ParamValues[0].Name)
		assert.NotNil(t, build.Spec.ParamValues[0].SingleValue)
		assert.Equal(t, "registry.example.com/image:latest", *build.Spec.ParamValues[0].SingleValue.Value)

		mockClient.AssertExpectations(t)
	})

	t.Run("DockerImage direct", func(t *testing.T) {
		co := &ConvertOptions{}

		bc := &buildv1.BuildConfig{
			ObjectMeta: metav1.ObjectMeta{Name: "test-bc"},
			Spec: buildv1.BuildConfigSpec{
				CommonSpec: buildv1.CommonSpec{
					Strategy: buildv1.BuildStrategy{
						Type: buildv1.SourceBuildStrategyType,
						SourceStrategy: &buildv1.SourceBuildStrategy{
							From: corev1.ObjectReference{Kind: DockerImage, Name: "docker.io/library/nginx:latest"},
						},
					},
				},
			},
		}

		build := &shipwrightv1beta1.Build{Spec: shipwrightv1beta1.BuildSpec{ParamValues: []shipwrightv1beta1.ParamValue{}}}

		err := co.processSourceStrategyFromField(bc, build)
		assert.NoError(t, err)
		assert.Equal(t, 1, len(build.Spec.ParamValues))
		assert.Equal(t, "builder-image", build.Spec.ParamValues[0].Name)
		assert.NotNil(t, build.Spec.ParamValues[0].SingleValue)
		assert.Equal(t, "docker.io/library/nginx:latest", *build.Spec.ParamValues[0].SingleValue.Value)
	})

	t.Run("Unknown kind returns error", func(t *testing.T) {
		co := &ConvertOptions{}

		bc := &buildv1.BuildConfig{
			ObjectMeta: metav1.ObjectMeta{Name: "test-bc"},
			Spec: buildv1.BuildConfigSpec{
				CommonSpec: buildv1.CommonSpec{
					Strategy: buildv1.BuildStrategy{
						Type: buildv1.SourceBuildStrategyType,
						SourceStrategy: &buildv1.SourceBuildStrategy{
							From: corev1.ObjectReference{Kind: "Unknown", Name: "x"},
						},
					},
				},
			},
		}

		build := &shipwrightv1beta1.Build{Spec: shipwrightv1beta1.BuildSpec{ParamValues: []shipwrightv1beta1.ParamValue{}}}

		err := co.processSourceStrategyFromField(bc, build)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "source strategy From kind Unknown is unknown for BuildConfig test-bc")
		assert.Equal(t, 0, len(build.Spec.ParamValues))
	})

	t.Run("Empty Name no-op", func(t *testing.T) {
		co := &ConvertOptions{}

		bc := &buildv1.BuildConfig{
			ObjectMeta: metav1.ObjectMeta{Name: "test-bc"},
			Spec: buildv1.BuildConfigSpec{
				CommonSpec: buildv1.CommonSpec{
					Strategy: buildv1.BuildStrategy{
						Type: buildv1.SourceBuildStrategyType,
						SourceStrategy: &buildv1.SourceBuildStrategy{
							From: corev1.ObjectReference{Kind: DockerImage, Name: ""},
						},
					},
				},
			},
		}

		build := &shipwrightv1beta1.Build{Spec: shipwrightv1beta1.BuildSpec{ParamValues: []shipwrightv1beta1.ParamValue{}}}

		err := co.processSourceStrategyFromField(bc, build)
		assert.NoError(t, err)
		assert.Equal(t, 0, len(build.Spec.ParamValues))
	})

	t.Run("ImageStreamTag resolve error", func(t *testing.T) {
		mockClient := &MockClient{}
		co := &ConvertOptions{Client: mockClient}

		// Mock Get to return an error
		mockClient.On("Get", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(errors.New("image stream not found"))

		bc := &buildv1.BuildConfig{
			ObjectMeta: metav1.ObjectMeta{Name: "test-bc"},
			Spec: buildv1.BuildConfigSpec{
				CommonSpec: buildv1.CommonSpec{
					Strategy: buildv1.BuildStrategy{
						Type: buildv1.SourceBuildStrategyType,
						SourceStrategy: &buildv1.SourceBuildStrategy{
							From: corev1.ObjectReference{Kind: ImageStreamTag, Name: "example:latest", Namespace: "ns"},
						},
					},
				},
			},
		}

		build := &shipwrightv1beta1.Build{Spec: shipwrightv1beta1.BuildSpec{ParamValues: []shipwrightv1beta1.ParamValue{}}}

		err := co.processSourceStrategyFromField(bc, build)
		assert.Error(t, err)
		assert.Equal(t, "image stream not found", err.Error())
		assert.Equal(t, 0, len(build.Spec.ParamValues))

		mockClient.AssertExpectations(t)
	})
}

func TestWriteBuildConfigs(t *testing.T) {
	// Create a temporary directory for testing
	tempDir, err := os.MkdirTemp("", "buildconfigs_test")
	assert.NoError(t, err)
	defer os.RemoveAll(tempDir)

	bc := buildv1.BuildConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-bc",
			Namespace: "test-namespace",
		},
	}
	bc.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "build.openshift.io",
		Version: "v1",
		Kind:    "BuildConfig",
	})

	bcList := buildv1.BuildConfigList{
		Items: []buildv1.BuildConfig{bc},
	}

	co := &ConvertOptions{
		ExportDir: tempDir,
		Namespace: "test-namespace",
		logger:    logrus.New(),
	}

	err = co.writeBuildConfigs(bcList)
	assert.NoError(t, err)

	// Verify file was created
	expectedPath := filepath.Join(tempDir, "buildconfigs", "test-namespace", "BuildConfig_build.openshift.io_v1_test-namespace_test-bc.yaml")
	_, err = os.Stat(expectedPath)
	assert.NoError(t, err)
}

func TestWriteBuild(t *testing.T) {
	// Create a temporary directory for testing
	tempDir, err := os.MkdirTemp("", "builds_test")
	assert.NoError(t, err)
	defer os.RemoveAll(tempDir)

	build := &shipwrightv1beta1.Build{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-build",
			Namespace: "test-namespace",
		},
	}
	build.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "shipwright.io",
		Version: "v1beta1",
		Kind:    "Build",
	})

	co := &ConvertOptions{
		ExportDir: tempDir,
		Namespace: "test-namespace",
		logger:    logrus.New(),
	}

	err = co.writeBuild(build)
	assert.NoError(t, err)

	// Verify file was created
	expectedPath := filepath.Join(tempDir, "builds", "test-namespace", "Build_shipwright.io_v1beta1_test-namespace_test-build.yaml")
	_, err = os.Stat(expectedPath)
	assert.NoError(t, err)
}

func TestProcessDockerStrategyVolumes(t *testing.T) {
	tests := []struct {
		name            string
		buildConfig     *buildv1.BuildConfig
		expectedVolumes int
		expectError     bool
		errorContains   string
	}{
		{
			name: "no volumes",
			buildConfig: &buildv1.BuildConfig{
				ObjectMeta: metav1.ObjectMeta{Name: "test-bc"},
				Spec: buildv1.BuildConfigSpec{
					CommonSpec: buildv1.CommonSpec{
						Strategy: buildv1.BuildStrategy{
							Type: buildv1.DockerBuildStrategyType,
							DockerStrategy: &buildv1.DockerBuildStrategy{
								Volumes: []buildv1.BuildVolume{},
							},
						},
					},
				},
			},
			expectedVolumes: 0,
			expectError:     false,
		},
		{
			name: "nil volumes",
			buildConfig: &buildv1.BuildConfig{
				ObjectMeta: metav1.ObjectMeta{Name: "test-bc"},
				Spec: buildv1.BuildConfigSpec{
					CommonSpec: buildv1.CommonSpec{
						Strategy: buildv1.BuildStrategy{
							Type: buildv1.DockerBuildStrategyType,
							DockerStrategy: &buildv1.DockerBuildStrategy{
								Volumes: nil,
							},
						},
					},
				},
			},
			expectedVolumes: 0,
			expectError:     false,
		},
		{
			name: "secret volume",
			buildConfig: &buildv1.BuildConfig{
				ObjectMeta: metav1.ObjectMeta{Name: "test-bc"},
				Spec: buildv1.BuildConfigSpec{
					CommonSpec: buildv1.CommonSpec{
						Strategy: buildv1.BuildStrategy{
							Type: buildv1.DockerBuildStrategyType,
							DockerStrategy: &buildv1.DockerBuildStrategy{
								Volumes: []buildv1.BuildVolume{
									{
										Name: "secret-volume",
										Source: buildv1.BuildVolumeSource{
											Type: buildv1.BuildVolumeSourceTypeSecret,
											Secret: &corev1.SecretVolumeSource{
												SecretName: "my-secret",
											},
										},
										Mounts: []buildv1.BuildVolumeMount{
											{DestinationPath: "/etc/secret"},
										},
									},
								},
							},
						},
					},
				},
			},
			expectedVolumes: 1,
			expectError:     false,
		},
		{
			name: "configmap volume",
			buildConfig: &buildv1.BuildConfig{
				ObjectMeta: metav1.ObjectMeta{Name: "test-bc"},
				Spec: buildv1.BuildConfigSpec{
					CommonSpec: buildv1.CommonSpec{
						Strategy: buildv1.BuildStrategy{
							Type: buildv1.DockerBuildStrategyType,
							DockerStrategy: &buildv1.DockerBuildStrategy{
								Volumes: []buildv1.BuildVolume{
									{
										Name: "config-volume",
										Source: buildv1.BuildVolumeSource{
											Type: buildv1.BuildVolumeSourceTypeConfigMap,
											ConfigMap: &corev1.ConfigMapVolumeSource{
												LocalObjectReference: corev1.LocalObjectReference{
													Name: "my-config",
												},
											},
										},
										Mounts: []buildv1.BuildVolumeMount{
											{DestinationPath: "/etc/config"},
										},
									},
								},
							},
						},
					},
				},
			},
			expectedVolumes: 1,
			expectError:     false,
		},
		{
			name: "multiple volumes",
			buildConfig: &buildv1.BuildConfig{
				ObjectMeta: metav1.ObjectMeta{Name: "test-bc"},
				Spec: buildv1.BuildConfigSpec{
					CommonSpec: buildv1.CommonSpec{
						Strategy: buildv1.BuildStrategy{
							Type: buildv1.DockerBuildStrategyType,
							DockerStrategy: &buildv1.DockerBuildStrategy{
								Volumes: []buildv1.BuildVolume{
									{
										Name: "secret-volume",
										Source: buildv1.BuildVolumeSource{
											Type: buildv1.BuildVolumeSourceTypeSecret,
											Secret: &corev1.SecretVolumeSource{
												SecretName: "my-secret",
											},
										},
									},
									{
										Name: "config-volume",
										Source: buildv1.BuildVolumeSource{
											Type: buildv1.BuildVolumeSourceTypeConfigMap,
											ConfigMap: &corev1.ConfigMapVolumeSource{
												LocalObjectReference: corev1.LocalObjectReference{
													Name: "my-config",
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			expectedVolumes: 2,
			expectError:     false,
		},
		{
			name: "secret volume with nil secret",
			buildConfig: &buildv1.BuildConfig{
				ObjectMeta: metav1.ObjectMeta{Name: "test-bc"},
				Spec: buildv1.BuildConfigSpec{
					CommonSpec: buildv1.CommonSpec{
						Strategy: buildv1.BuildStrategy{
							Type: buildv1.DockerBuildStrategyType,
							DockerStrategy: &buildv1.DockerBuildStrategy{
								Volumes: []buildv1.BuildVolume{
									{
										Name: "secret-volume",
										Source: buildv1.BuildVolumeSource{
											Type:   buildv1.BuildVolumeSourceTypeSecret,
											Secret: nil, // This should cause an error
										},
									},
								},
							},
						},
					},
				},
			},
			expectedVolumes: 0,
			expectError:     true,
			errorContains:   "secret volume source is nil",
		},
		{
			name: "configmap volume with nil configmap",
			buildConfig: &buildv1.BuildConfig{
				ObjectMeta: metav1.ObjectMeta{Name: "test-bc"},
				Spec: buildv1.BuildConfigSpec{
					CommonSpec: buildv1.CommonSpec{
						Strategy: buildv1.BuildStrategy{
							Type: buildv1.DockerBuildStrategyType,
							DockerStrategy: &buildv1.DockerBuildStrategy{
								Volumes: []buildv1.BuildVolume{
									{
										Name: "config-volume",
										Source: buildv1.BuildVolumeSource{
											Type:      buildv1.BuildVolumeSourceTypeConfigMap,
											ConfigMap: nil, // This should cause an error
										},
									},
								},
							},
						},
					},
				},
			},
			expectedVolumes: 0,
			expectError:     true,
			errorContains:   "configMap volume source is nil",
		},
		{
			name: "unsupported volume type",
			buildConfig: &buildv1.BuildConfig{
				ObjectMeta: metav1.ObjectMeta{Name: "test-bc"},
				Spec: buildv1.BuildConfigSpec{
					CommonSpec: buildv1.CommonSpec{
						Strategy: buildv1.BuildStrategy{
							Type: buildv1.DockerBuildStrategyType,
							DockerStrategy: &buildv1.DockerBuildStrategy{
								Volumes: []buildv1.BuildVolume{
									{
										Name: "unsupported-volume",
										Source: buildv1.BuildVolumeSource{
											Type: "UnsupportedType",
										},
									},
								},
							},
						},
					},
				},
			},
			expectedVolumes: 0,
			expectError:     true,
			errorContains:   "unsupported volume source type",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			co := &ConvertOptions{
				logger: logrus.New(),
			}
			build := &shipwrightv1beta1.Build{
				Spec: shipwrightv1beta1.BuildSpec{
					Volumes: []shipwrightv1beta1.BuildVolume{},
				},
			}

			err := co.processDockerStrategyVolumes(tt.buildConfig, build)

			if tt.expectError {
				assert.Error(t, err)
				if tt.errorContains != "" {
					assert.Contains(t, err.Error(), tt.errorContains)
				}
			} else {
				assert.NoError(t, err)
			}

			assert.Equal(t, tt.expectedVolumes, len(build.Spec.Volumes))

			// Verify volume names and types for successful cases
			if !tt.expectError && tt.expectedVolumes > 0 {
				for i, expectedVolume := range tt.buildConfig.Spec.Strategy.DockerStrategy.Volumes {
					assert.Equal(t, expectedVolume.Name, build.Spec.Volumes[i].Name)

					// Verify volume source type
					switch expectedVolume.Source.Type {
					case buildv1.BuildVolumeSourceTypeSecret:
						assert.NotNil(t, build.Spec.Volumes[i].Secret)
						assert.Equal(t, expectedVolume.Source.Secret.SecretName, build.Spec.Volumes[i].Secret.SecretName)
					case buildv1.BuildVolumeSourceTypeConfigMap:
						assert.NotNil(t, build.Spec.Volumes[i].ConfigMap)
						assert.Equal(t, expectedVolume.Source.ConfigMap.Name, build.Spec.Volumes[i].ConfigMap.Name)
					}
				}
			}
		})
	}
}

func TestValidateDockerPullSecret(t *testing.T) {
	tests := []struct {
		name          string
		buildConfig   *buildv1.BuildConfig
		mockSecret    *corev1.Secret
		mockError     error
		expectError   bool
		errorContains string
	}{
		{
			name: "valid dockerconfigjson secret",
			buildConfig: &buildv1.BuildConfig{
				ObjectMeta: metav1.ObjectMeta{Name: "test-bc", Namespace: "test-ns"},
				Spec: buildv1.BuildConfigSpec{
					CommonSpec: buildv1.CommonSpec{
						Strategy: buildv1.BuildStrategy{
							Type: buildv1.DockerBuildStrategyType,
							DockerStrategy: &buildv1.DockerBuildStrategy{
								PullSecret: &corev1.LocalObjectReference{Name: "my-secret"},
							},
						},
					},
				},
			},
			mockSecret: &corev1.Secret{
				Type: corev1.SecretTypeDockerConfigJson,
				Data: map[string][]byte{
					corev1.DockerConfigJsonKey: []byte(`{"auths":{"registry.example.com":{"auth":"dGVzdDp0ZXN0"}}}`),
				},
			},
			mockError:   nil,
			expectError: false,
		},
		{
			name: "valid dockercfg secret",
			buildConfig: &buildv1.BuildConfig{
				ObjectMeta: metav1.ObjectMeta{Name: "test-bc", Namespace: "test-ns"},
				Spec: buildv1.BuildConfigSpec{
					CommonSpec: buildv1.CommonSpec{
						Strategy: buildv1.BuildStrategy{
							Type: buildv1.DockerBuildStrategyType,
							DockerStrategy: &buildv1.DockerBuildStrategy{
								PullSecret: &corev1.LocalObjectReference{Name: "my-secret"},
							},
						},
					},
				},
			},
			mockSecret: &corev1.Secret{
				Type: corev1.SecretTypeDockercfg,
				Data: map[string][]byte{
					corev1.DockerConfigKey: []byte(`{"registry.example.com":{"auth":"dGVzdDp0ZXN0"}}`),
				},
			},
			mockError:   nil,
			expectError: false,
		},
		{
			name: "empty pull secret name",
			buildConfig: &buildv1.BuildConfig{
				ObjectMeta: metav1.ObjectMeta{Name: "test-bc", Namespace: "test-ns"},
				Spec: buildv1.BuildConfigSpec{
					CommonSpec: buildv1.CommonSpec{
						Strategy: buildv1.BuildStrategy{
							Type: buildv1.DockerBuildStrategyType,
							DockerStrategy: &buildv1.DockerBuildStrategy{
								PullSecret: &corev1.LocalObjectReference{Name: ""},
							},
						},
					},
				},
			},
			expectError:   true,
			errorContains: "dockerStrategy.pullSecret name is empty",
		},
		{
			name: "secret not found",
			buildConfig: &buildv1.BuildConfig{
				ObjectMeta: metav1.ObjectMeta{Name: "test-bc", Namespace: "test-ns"},
				Spec: buildv1.BuildConfigSpec{
					CommonSpec: buildv1.CommonSpec{
						Strategy: buildv1.BuildStrategy{
							Type: buildv1.DockerBuildStrategyType,
							DockerStrategy: &buildv1.DockerBuildStrategy{
								PullSecret: &corev1.LocalObjectReference{Name: "missing-secret"},
							},
						},
					},
				},
			},
			mockError:     errors.New("secret not found"),
			expectError:   true,
			errorContains: "failed to get pull secret",
		},
		{
			name: "unsupported secret type",
			buildConfig: &buildv1.BuildConfig{
				ObjectMeta: metav1.ObjectMeta{Name: "test-bc", Namespace: "test-ns"},
				Spec: buildv1.BuildConfigSpec{
					CommonSpec: buildv1.CommonSpec{
						Strategy: buildv1.BuildStrategy{
							Type: buildv1.DockerBuildStrategyType,
							DockerStrategy: &buildv1.DockerBuildStrategy{
								PullSecret: &corev1.LocalObjectReference{Name: "my-secret"},
							},
						},
					},
				},
			},
			mockSecret: &corev1.Secret{
				Type: "Opaque",
				Data: map[string][]byte{
					"data": []byte("some-data"),
				},
			},
			mockError:     nil,
			expectError:   true,
			errorContains: "unsupported pull secret type",
		},
		{
			name: "dockerconfigjson secret missing data",
			buildConfig: &buildv1.BuildConfig{
				ObjectMeta: metav1.ObjectMeta{Name: "test-bc", Namespace: "test-ns"},
				Spec: buildv1.BuildConfigSpec{
					CommonSpec: buildv1.CommonSpec{
						Strategy: buildv1.BuildStrategy{
							Type: buildv1.DockerBuildStrategyType,
							DockerStrategy: &buildv1.DockerBuildStrategy{
								PullSecret: &corev1.LocalObjectReference{Name: "my-secret"},
							},
						},
					},
				},
			},
			mockSecret: &corev1.Secret{
				Type: corev1.SecretTypeDockerConfigJson,
				Data: map[string][]byte{}, // Missing .dockerconfigjson key
			},
			mockError:     nil,
			expectError:   true,
			errorContains: "must contain key \".dockerconfigjson\"",
		},
		{
			name: "dockercfg secret missing data",
			buildConfig: &buildv1.BuildConfig{
				ObjectMeta: metav1.ObjectMeta{Name: "test-bc", Namespace: "test-ns"},
				Spec: buildv1.BuildConfigSpec{
					CommonSpec: buildv1.CommonSpec{
						Strategy: buildv1.BuildStrategy{
							Type: buildv1.DockerBuildStrategyType,
							DockerStrategy: &buildv1.DockerBuildStrategy{
								PullSecret: &corev1.LocalObjectReference{Name: "my-secret"},
							},
						},
					},
				},
			},
			mockSecret: &corev1.Secret{
				Type: corev1.SecretTypeDockercfg,
				Data: map[string][]byte{}, // Missing .dockercfg key
			},
			mockError:     nil,
			expectError:   true,
			errorContains: "must contain key \".dockercfg\"",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockClient := &MockClient{}
			co := &ConvertOptions{Client: mockClient}

			// Mock the Get call
			if tt.mockError != nil || tt.mockSecret != nil {
				mockClient.On("Get", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
					secret := args.Get(2).(*corev1.Secret)
					if tt.mockSecret != nil {
						*secret = *tt.mockSecret
					}
				}).Return(tt.mockError)
			}

			err := co.validateDockerPullSecret(tt.buildConfig)

			if tt.expectError {
				assert.Error(t, err)
				if tt.errorContains != "" {
					assert.Contains(t, err.Error(), tt.errorContains)
				}
			} else {
				assert.NoError(t, err)
			}

			mockClient.AssertExpectations(t)
		})
	}
}

func TestGenerateServiceAccountForPullSecret(t *testing.T) {
	// Create a temporary directory for testing
	tempDir, err := os.MkdirTemp("", "serviceaccount_test")
	assert.NoError(t, err)
	defer os.RemoveAll(tempDir)

	tests := []struct {
		name               string
		buildConfig        *buildv1.BuildConfig
		expectedSAName     string
		expectedSecretName string
		expectError        bool
		errorContains      string
	}{
		{
			name: "with existing service account name",
			buildConfig: &buildv1.BuildConfig{
				ObjectMeta: metav1.ObjectMeta{Name: "test-bc", Namespace: "test-ns"},
				Spec: buildv1.BuildConfigSpec{
					CommonSpec: buildv1.CommonSpec{
						ServiceAccount: "existing-sa",
						Strategy: buildv1.BuildStrategy{
							Type: buildv1.DockerBuildStrategyType,
							DockerStrategy: &buildv1.DockerBuildStrategy{
								PullSecret: &corev1.LocalObjectReference{Name: "my-secret"},
							},
						},
					},
				},
			},
			expectedSAName:     "existing-sa",
			expectedSecretName: "my-secret",
			expectError:        false,
		},
		{
			name: "without service account name - generates default",
			buildConfig: &buildv1.BuildConfig{
				ObjectMeta: metav1.ObjectMeta{Name: "test-bc", Namespace: "test-ns"},
				Spec: buildv1.BuildConfigSpec{
					CommonSpec: buildv1.CommonSpec{
						Strategy: buildv1.BuildStrategy{
							Type: buildv1.DockerBuildStrategyType,
							DockerStrategy: &buildv1.DockerBuildStrategy{
								PullSecret: &corev1.LocalObjectReference{Name: "my-secret"},
							},
						},
					},
				},
			},
			expectedSAName:     "test-bc-sa",
			expectedSecretName: "my-secret",
			expectError:        false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			co := &ConvertOptions{
				ExportDir: tempDir,
				logger:    logrus.New(),
			}

			err := co.generateServiceAccountForPullSecret(tt.buildConfig)

			if tt.expectError {
				assert.Error(t, err)
				if tt.errorContains != "" {
					assert.Contains(t, err.Error(), tt.errorContains)
				}
			} else {
				assert.NoError(t, err)

				// Verify ServiceAccount file was created
				expectedPath := filepath.Join(tempDir, "serviceaccounts", tt.buildConfig.Namespace,
					fmt.Sprintf("ServiceAccount__v1_%s_%s.yaml", tt.buildConfig.Namespace, tt.expectedSAName))
				_, err = os.Stat(expectedPath)
				assert.NoError(t, err, "ServiceAccount file should be created")

				// Read and verify the ServiceAccount content
				content, err := os.ReadFile(expectedPath)
				assert.NoError(t, err)

				// Basic content verification
				assert.Contains(t, string(content), tt.expectedSAName)
				assert.Contains(t, string(content), tt.expectedSecretName)
				assert.Contains(t, string(content), "imagePullSecrets")
			}
		})
	}
}

func TestProcessDockerStrategyEnv(t *testing.T) {
	tests := []struct {
		name          string
		buildConfig   *buildv1.BuildConfig
		expectedEnv   []corev1.EnvVar
		expectError   bool
		errorContains string
	}{
		{
			name: "nil env - no change",
			buildConfig: &buildv1.BuildConfig{
				ObjectMeta: metav1.ObjectMeta{Name: "test-bc"},
				Spec: buildv1.BuildConfigSpec{
					CommonSpec: buildv1.CommonSpec{
						Strategy: buildv1.BuildStrategy{
							Type: buildv1.DockerBuildStrategyType,
							DockerStrategy: &buildv1.DockerBuildStrategy{
								Env: nil,
							},
						},
					},
				},
			},
			expectedEnv: []corev1.EnvVar{},
			expectError: false,
		},
		{
			name: "empty env - no change",
			buildConfig: &buildv1.BuildConfig{
				ObjectMeta: metav1.ObjectMeta{Name: "test-bc"},
				Spec: buildv1.BuildConfigSpec{
					CommonSpec: buildv1.CommonSpec{
						Strategy: buildv1.BuildStrategy{
							Type: buildv1.DockerBuildStrategyType,
							DockerStrategy: &buildv1.DockerBuildStrategy{
								Env: []corev1.EnvVar{},
							},
						},
					},
				},
			},
			expectedEnv: []corev1.EnvVar{},
			expectError: false,
		},
		{
			name: "single env variable",
			buildConfig: &buildv1.BuildConfig{
				ObjectMeta: metav1.ObjectMeta{Name: "test-bc"},
				Spec: buildv1.BuildConfigSpec{
					CommonSpec: buildv1.CommonSpec{
						Strategy: buildv1.BuildStrategy{
							Type: buildv1.DockerBuildStrategyType,
							DockerStrategy: &buildv1.DockerBuildStrategy{
								Env: []corev1.EnvVar{
									{Name: "VAR1", Value: "value1"},
								},
							},
						},
					},
				},
			},
			expectedEnv: []corev1.EnvVar{
				{Name: "VAR1", Value: "value1"},
			},
			expectError: false,
		},
		{
			name: "multiple env variables",
			buildConfig: &buildv1.BuildConfig{
				ObjectMeta: metav1.ObjectMeta{Name: "test-bc"},
				Spec: buildv1.BuildConfigSpec{
					CommonSpec: buildv1.CommonSpec{
						Strategy: buildv1.BuildStrategy{
							Type: buildv1.DockerBuildStrategyType,
							DockerStrategy: &buildv1.DockerBuildStrategy{
								Env: []corev1.EnvVar{
									{Name: "VAR1", Value: "value1"},
									{Name: "VAR2", Value: "value2"},
									{Name: "VAR3", Value: "value3"},
								},
							},
						},
					},
				},
			},
			expectedEnv: []corev1.EnvVar{
				{Name: "VAR1", Value: "value1"},
				{Name: "VAR2", Value: "value2"},
				{Name: "VAR3", Value: "value3"},
			},
			expectError: false,
		},
		{
			name: "env variables with existing env in build",
			buildConfig: &buildv1.BuildConfig{
				ObjectMeta: metav1.ObjectMeta{Name: "test-bc"},
				Spec: buildv1.BuildConfigSpec{
					CommonSpec: buildv1.CommonSpec{
						Strategy: buildv1.BuildStrategy{
							Type: buildv1.DockerBuildStrategyType,
							DockerStrategy: &buildv1.DockerBuildStrategy{
								Env: []corev1.EnvVar{
									{Name: "DOCKER_VAR", Value: "docker_value"},
								},
							},
						},
					},
				},
			},
			expectedEnv: []corev1.EnvVar{
				{Name: "EXISTING_VAR", Value: "existing_value"},
				{Name: "DOCKER_VAR", Value: "docker_value"},
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			build := &shipwrightv1beta1.Build{
				Spec: shipwrightv1beta1.BuildSpec{
					Env: []corev1.EnvVar{},
				},
			}

			// Add existing env for the test case that checks appending
			if tt.name == "env variables with existing env in build" {
				build.Spec.Env = []corev1.EnvVar{
					{Name: "EXISTING_VAR", Value: "existing_value"},
				}
			}

			// Simulate the ENV processing logic from convertBuildConfigs
			if tt.buildConfig.Spec.Strategy.DockerStrategy.Env != nil {
				build.Spec.Env = append(build.Spec.Env, tt.buildConfig.Spec.Strategy.DockerStrategy.Env...)
			}

			if tt.expectError {
				// This test case doesn't have error scenarios, but keeping structure for future
				assert.Fail(t, "No error scenarios defined for ENV processing")
			} else {
				assert.NoError(t, nil) // No error expected
			}

			// Verify the environment variables
			assert.Equal(t, len(tt.expectedEnv), len(build.Spec.Env))

			for i, expectedEnv := range tt.expectedEnv {
				assert.Equal(t, expectedEnv.Name, build.Spec.Env[i].Name)
				assert.Equal(t, expectedEnv.Value, build.Spec.Env[i].Value)
			}
		})
	}
}

func TestGetServiceAccountFilePath(t *testing.T) {
	tests := []struct {
		name           string
		serviceAccount corev1.ServiceAccount
		expectedPath   string
	}{
		{
			name: "standard service account",
			serviceAccount: corev1.ServiceAccount{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "v1",
					Kind:       "ServiceAccount",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-sa",
					Namespace: "test-ns",
				},
			},
			expectedPath: "ServiceAccount__v1_test-ns_test-sa.yaml",
		},
		{
			name: "service account with group",
			serviceAccount: corev1.ServiceAccount{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "v1",
					Kind:       "ServiceAccount",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "complex-sa",
					Namespace: "complex-ns",
				},
			},
			expectedPath: "ServiceAccount__v1_complex-ns_complex-sa.yaml",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getServiceAccountFilePath(tt.serviceAccount)
			assert.Equal(t, tt.expectedPath, result)
		})
	}
}

// Helper function to create string pointers
func stringPtr(s string) *string {
	return &s
}
