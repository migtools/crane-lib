package binary_plugin

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"reflect"
	"testing"

	"github.com/konveyor/crane-lib/transform"
	"github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

type fakeCommandRunner struct {
	stdout, stderr                            []byte
	errorRunningMetadata, errorRunningCommand error
	metadataStdout, metadataStderr            []byte
}

func (f *fakeCommandRunner) Run(_ *unstructured.Unstructured, _ logrus.FieldLogger) ([]byte, []byte, error) {
	return f.stdout, f.stderr, f.errorRunningCommand
}

func (f *fakeCommandRunner) Metadata(_ logrus.FieldLogger) ([]byte, []byte, error) {
	return f.metadataStdout, f.metadataStderr, f.errorRunningMetadata

}

// TestShellMetadataSuccess is a method that is called as a substitute for a shell command,
// the GO_TEST_PROCESS flag ensures that if it is called as part of the test suite, it is
// skipped.
func TestShellMetadataSuccess(t *testing.T) {
	if os.Getenv("GO_TEST_PROCESS") != "1" {
		return
	}

	var s string
	_, err := fmt.Scanln(&s)
	if err != nil {
		os.Exit(1)
	}

	if s != transform.MetadataString {
		os.Exit(1)
	}

	//TODO: Validate stdin is correct.
	res, err := json.Marshal(transform.PluginMetadata{
		Name:            "fakeShellMetadata",
		Version:         "v1",
		RequestVersion:  []transform.Version{transform.V1},
		ResponseVersion: []transform.Version{transform.V1},
		OptionalFields:  []string{},
	})

	if err != nil {
		fmt.Fprint(os.Stderr, err.Error())
		os.Exit(1)
	}

	fmt.Fprint(os.Stdout, string(res))
	os.Exit(0)
}

func TestShellProcessFail(t *testing.T) {
	if os.Getenv("GO_TEST_PROCESS") != "1" {
		return
	}
	os.Exit(1)
}

func TestNewBinaryPlugin(t *testing.T) {
	tests := []struct {
		name           string
		stdout, stderr []byte
		runErr         error
		want           transform.PluginMetadata
		wantErr        bool
		cliContext     execContext
	}{
		{
			name:   "ValidStdoutNoStderr",
			stdout: []byte(`{"version": "v1", "isWhiteOut": true}`),
			runErr: nil,
			want: transform.PluginMetadata{
				Name:            "fakeShellMetadata",
				Version:         "v1",
				RequestVersion:  []transform.Version{transform.V1},
				ResponseVersion: []transform.Version{transform.V1},
			},
			cliContext: func(name string, args ...string) *exec.Cmd {
				cs := []string{"-test.run=TestShellMetadataSuccess", "--", name}
				cs = append(cs, args...)
				cmd := exec.Command(os.Args[0], cs...)
				cmd.Env = []string{"GO_TEST_PROCESS=1"}
				return cmd
			},
			wantErr: false,
		},
		// {
		// 	name:    "InValidStdoutNoStderr",
		// 	stdout:  []byte(`{"version": v1", "isWhiteOut": true}`),
		// 	runErr:  nil,
		// 	want:    transform.PluginResponse{},
		// 	wantErr: true,
		// },
		// {
		// 	name:    "NoStdoutSomeStderr",
		// 	stderr:  []byte("panic: invalid reference"),
		// 	runErr:  nil,
		// 	want:    transform.PluginResponse{},
		// 	wantErr: true,
		// },
		// {
		// 	name:    "RunError",
		// 	runErr:  fmt.Errorf("error running the plugin"),
		// 	want:    transform.PluginResponse{},
		// 	wantErr: true,
		// },
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cliContext = tt.cliContext
			b, err := NewBinaryPlugin(tt.name)
			if (err != nil) != tt.wantErr {
				t.Errorf("Run() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !reflect.DeepEqual(b.Metadata(), tt.want) {
				t.Errorf("Run() got = %v, want %v", b.Metadata(), tt.want)
			}
		})
	}
}

func TestBinaryPlugin_Run(t *testing.T) {
	tests := []struct {
		name           string
		stdout, stderr []byte
		runErr         error
		want           transform.PluginResponse
		wantErr        bool
	}{
		{
			name:   "ValidStdoutNoStderr",
			stdout: []byte(`{"version": "v1", "isWhiteOut": true}`),
			runErr: nil,
			want: transform.PluginResponse{
				Version:    "v1",
				IsWhiteOut: true,
				Patches:    nil,
			},
			wantErr: false,
		},
		{
			name:    "InValidStdoutNoStderr",
			stdout:  []byte(`{"version": v1", "isWhiteOut": true}`),
			runErr:  nil,
			want:    transform.PluginResponse{},
			wantErr: true,
		},
		{
			name:   "NoStdoutSomeStderr",
			stdout: []byte(`{"version": "v1", "isWhiteOut": true}`),
			stderr: []byte("panic: invalid reference"),
			runErr: nil,
			want: transform.PluginResponse{
				Version:    "v1",
				IsWhiteOut: true,
				Patches:    nil,
			},
			wantErr: false,
		},
		{
			name:    "RunError",
			runErr:  fmt.Errorf("error running the plugin"),
			want:    transform.PluginResponse{},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := &BinaryPlugin{
				commandRunner: &fakeCommandRunner{
					stdout:              tt.stdout,
					stderr:              tt.stderr,
					errorRunningCommand: tt.runErr,
				},
				log: logrus.New().WithField("test", tt.name),
			}
			got, err := b.Run(&unstructured.Unstructured{}, nil)
			if (err != nil) != tt.wantErr {
				t.Errorf("Run() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Run() got = %v, want %v", got, tt.want)
			}
		})
	}
}
