package errors

import "testing"

func TestPluginError_Error(t *testing.T) {
	tests := []struct {
		name     string
		errorObj PluginError
		want     string
	}{
		{
			name: "valid plugin error",
			errorObj: PluginError{
				Type:         PluginRunError,
				Message:      "some message",
				ErrorMessage: "error occured due to Run function",
			},
			want: `{"type":"PluginRunError","message":"some message","error":"error occured due to Run function"}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.errorObj.Error(); got != tt.want {
				t.Errorf("Error() = %v, want %v", got, tt.want)
			}
		})
	}
}
