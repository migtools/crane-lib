package convert

import (
	"context"
	"errors"
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
		name           string
		buildConfig    buildv1.BuildConfig
		expectedImage  string
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
		name              string
		searchRegistries  []string
		insecureRegistries []string
		blockRegistries   []string
		expectedParams    int
	}{
		{
			name:              "no registries",
			searchRegistries:  []string{},
			insecureRegistries: []string{},
			blockRegistries:   []string{},
			expectedParams:    0,
		},
		{
			name:              "search registries only",
			searchRegistries:  []string{"registry1.example.com"},
			insecureRegistries: []string{},
			blockRegistries:   []string{},
			expectedParams:    1,
		},
		{
			name:              "all registry types",
			searchRegistries:  []string{"registry1.example.com"},
			insecureRegistries: []string{"registry2.example.com"},
			blockRegistries:   []string{"registry3.example.com"},
			expectedParams:    3,
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
		name           string
		streamName     string
		namespace      string
		mockError      error
		expectedImage  string
		expectError    bool
	}{
		{
			name:           "successful resolution",
			streamName:     "test-stream:latest",
			namespace:      "test-namespace",
			mockError:      nil,
			expectedImage:  "registry.example.com/image:latest",
			expectError:    false,
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

// Helper function to create string pointers
func stringPtr(s string) *string {
	return &s
} 