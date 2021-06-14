package binary_plugin

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/konveyor/crane-lib/transform"
	"github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

type fakeCommandRunner struct {
	stdout, stderr      []byte
	errorRunningCommand error
}

func (f *fakeCommandRunner) Run(_ *unstructured.Unstructured, _ logrus.FieldLogger) ([]byte, []byte, error) {
	return f.stdout, f.stderr, f.errorRunningCommand
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
			name:    "NoStdoutSomeStderr",
			stderr:  []byte("panic: invalid reference"),
			runErr:  nil,
			want:    transform.PluginResponse{},
			wantErr: true,
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
			got, err := b.Run(&unstructured.Unstructured{})
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
