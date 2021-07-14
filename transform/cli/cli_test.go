package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"reflect"
	"testing"

	jsonpatch "github.com/evanphx/json-patch"
	"github.com/konveyor/crane-lib/transform"
	bplugin "github.com/konveyor/crane-lib/transform/binary-plugin"
	"github.com/konveyor/crane-lib/transform/errors"
	ijsonpath "github.com/konveyor/crane-lib/transform/internal/jsonpatch"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

type fakeReader struct {
	*unstructured.Unstructured
	extras map[string]string
	err error
	readObj bool
	readExtras bool
}

func (f *fakeReader) Read(p []byte) (int, error) {
	if f.err != nil {
		return 0, f.err
	}
	if !f.readObj {
		b, err := f.Unstructured.MarshalJSON()
		for i := range p {
			if i >= len(b) {
				f.readObj = true
				return len(b), err
			}
			p[i] = b[i]
		}
		f.readObj = true
		return len(b), err
	}
	if !f.readExtras {
		b, err := json.Marshal(&f.extras)
		for i := range p {
			if i >= len(b) {
				f.readExtras = true
				return len(b), err
			}
			p[i] = b[i]
		}
		f.readExtras = true
		return len(b), err
	}
	return 0, io.EOF
}

func TestRunAndExit(t *testing.T) {
	tests := []struct {
		name           string
		reader         io.Reader
		response       transform.PluginResponse
		metadata       *transform.PluginMetadata
		wantErr        bool
		wantedErr      errors.PluginError
		fakeFunc       func(*unstructured.Unstructured, map[string]string) (transform.PluginResponse, error)
		version        string
		errCapture     bytes.Buffer
		outCapture     bytes.Buffer
		optionalFields []transform.OptionalFields
	}{
		// TODO: Add test cases.
		{
			name: "ValidJsonPodObject",
			reader: &fakeReader{
				Unstructured: &unstructured.Unstructured{Object: map[string]interface{}{
					"apiVersion": "v1",
					"kind":       "pod",
					"metadata": map[string]interface{}{
						"name":      "foo",
						"namespace": "bar",
					},
					"spec":   map[string]interface{}{},
					"status": map[string]interface{}{},
				}},
				err: nil,
			},
			response: transform.PluginResponse{
				Version:    "v1",
				IsWhiteOut: false,
				Patches:    []jsonpatch.Operation{},
			},
			fakeFunc: func(u *unstructured.Unstructured, extras map[string]string) (transform.PluginResponse, error) {
				return transform.PluginResponse{
					Version:    "v1",
					IsWhiteOut: false,
					Patches:    []jsonpatch.Operation{},
				}, nil
			},
			errCapture: bytes.Buffer{},
			outCapture: bytes.Buffer{},
		},
		{
			name:    "MetadataRequest",
			reader:  bytes.NewBufferString(bplugin.MetadataRequest),
			version: "v2",
			metadata: &transform.PluginMetadata{
				Name:            "MetadataRequest",
				Version:         "v2",
				RequestVersion:  []transform.Version{transform.V1},
				ResponseVersion: []transform.Version{transform.V1},
			},
			errCapture: bytes.Buffer{},
			outCapture: bytes.Buffer{},
		},
		{
			name:    "CanNotReadScanner",
			version: "v1",
			reader: &fakeReader{
				err: fmt.Errorf("Invalid input from stdIn"),
			},
			wantErr: true,
			wantedErr: errors.PluginError{
				Type: errors.PluginInvalidIOError,
			},
			errCapture: bytes.Buffer{},
			outCapture: bytes.Buffer{},
		},
		{
			name:    "InvalidKubernetesObject",
			version: "v1",
			reader:  bytes.NewBufferString(`{"apiVersion": "fake/v1"}`),
			wantErr: true,
			wantedErr: errors.PluginError{
				Type: errors.PluginInvalidInputError,
			},
			errCapture: bytes.Buffer{},
			outCapture: bytes.Buffer{},
		},
		{
			name: "RunError",
			reader: &fakeReader{
				Unstructured: &unstructured.Unstructured{Object: map[string]interface{}{
					"apiVersion": "v1",
					"kind":       "pod",
					"metadata": map[string]interface{}{
						"name":      "foo",
						"namespace": "bar",
					},
					"spec":   map[string]interface{}{},
					"status": map[string]interface{}{},
				}},
				err: nil,
			},
			fakeFunc: func(u *unstructured.Unstructured, extras map[string]string) (transform.PluginResponse, error) {
				return transform.PluginResponse{}, fmt.Errorf("invalid run")
			},
			errCapture: bytes.Buffer{},
			outCapture: bytes.Buffer{},
			wantErr:    true,
			wantedErr:  errors.PluginError{Type: errors.PluginRunError},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			// Create Custom Plugin to test with

			plugin := NewCustomPlugin(tt.name, tt.version, tt.optionalFields, tt.fakeFunc)
			// Set captures
			exitCode := 0
			stdErr = &tt.errCapture
			stdOut = &tt.outCapture
			reader = tt.reader
			exiter = func(i int) {
				exitCode = i
				panic(fmt.Errorf("panic for ext"))
			}

			// Handle panics because this will occure everytime we call exiter
			defer func() {
				if r := recover(); r != nil {
					errOut := tt.errCapture.Bytes()
					if !tt.wantErr {
						t.Errorf("Got error: %v\nwantErr: %v", string(errOut), tt.wantErr)
					}

					pluginError := errors.PluginError{}
					err := json.Unmarshal(errOut, &pluginError)
					if err != nil {
						t.Errorf("unable to get captured data: %v", err)
					}
					if tt.wantedErr.Type != pluginError.Type || exitCode != 1 {
						t.Errorf("Got error: %#v\nexpected error: %#v\nexitcode:%v", pluginError, tt.wantedErr, exitCode)
					}
				}
			}()

			RunAndExit(plugin)

			if tt.wantErr {
				t.Errorf("did not get expected error: %v", tt.wantedErr)
			}
			output := tt.outCapture.Bytes()

			// If no object then metadata should be called.
			if tt.metadata != nil {
				var pluginMetadata transform.PluginMetadata
				err := json.Unmarshal(output, &pluginMetadata)
				if err != nil {
					t.Errorf("unable to get captured data: %v", err)
				}
				if !reflect.DeepEqual(pluginMetadata, *tt.metadata) {
					t.Errorf("unable to get captured data: %v", err)
				}
				return
			}

			pluginOutput := transform.PluginResponse{}

			err := json.Unmarshal(output, &pluginOutput)
			if err != nil {
				t.Errorf("unable to get captured data: %v", err)
				return
			}

			patchesEqual, err := ijsonpath.Equal(tt.response.Patches, pluginOutput.Patches)
			if err != nil {
				t.Errorf("unable to get captured data: %v", err)
				return
			}

			if pluginOutput.IsWhiteOut != tt.response.IsWhiteOut || !patchesEqual || pluginOutput.Version != tt.response.Version {
				t.Errorf("output: %v \nnot equal to what is expected: %v", pluginOutput, tt.response)
				return
			}
		})
	}
}
