package rsync

import (
	"reflect"
	"testing"
)

func Test_filterRsyncExtraOptions(t *testing.T) {
	tests := []struct {
		name                 string
		args                 []string
		wantValidatedOptions []string
		wantError            bool
	}{
		{
			name: "when all provided options are valid, shouldn't return errors",
			args: []string{
				"--valid-one", "--valid-two", "--validoption",
				"-valid", "-v",
			},
			wantValidatedOptions: []string{
				"--valid-one", "--valid-two", "--validoption",
				"-valid", "-v",
			},
			wantError: false,
		},
		{
			name: "when one of the provided options are invalid, should return one error",
			args: []string{
				"--valid-one", "--valid-two", "--validoption",
				"-valid", "-v", "---invalid-option",
				"-- invalid", " ", "--",
				"invalid",
			},
			wantValidatedOptions: []string{
				"--valid-one", "--valid-two", "--validoption",
				"-valid", "-v",
			},
			wantError: true,
		},
		{
			name: "test options with parameters",
			args: []string{
				"--exclude=\"./.snapshot/file\"",
				"--exclude=\"/snapshot/tmp/\"",
				"--exclude=./.snapshot/",
			},
			wantValidatedOptions: []string{
				"--exclude=\"./.snapshot/file\"",
				"--exclude=\"/snapshot/tmp/\"",
				"--exclude=./.snapshot/",
			},
			wantError: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotValidatedOptions, err := filterRsyncExtraOptions(tt.args)
			if !reflect.DeepEqual(gotValidatedOptions, tt.wantValidatedOptions) {
				t.Errorf("filterRsyncExtraOptions() gotValidatedOptions = %v, want %v", gotValidatedOptions, tt.wantValidatedOptions)
			}
			if (err == nil) == tt.wantError {
				t.Errorf("filterRsyncExtraOptions() got error %#v, want error %#v", err, tt.wantError)
			}
		})
	}
}

func Test_filterRsyncInfoOptions(t *testing.T) {
	tests := []struct {
		name                 string
		args                 []string
		wantValidatedOptions []string
		wantError            bool
	}{
		{
			name: "when all provided options are valid, shouldn't return errors",
			args: []string{
				"COPY2", "MOVE1", "DELETE2",
				"ALL", "PROGRESS",
			},
			wantValidatedOptions: []string{
				"COPY2", "MOVE1", "DELETE2",
				"ALL", "PROGRESS",
			},
			wantError: false,
		},
		{
			name: "when one of the provided options are invalid, should return one error",
			args: []string{
				"COPY2", "MOVE1", "DELETE2",
				"ALL", "PROGRESS", "-MOVE1",
				"move1", "", "--ALL",
			},
			wantValidatedOptions: []string{
				"COPY2", "MOVE1", "DELETE2",
				"ALL", "PROGRESS",
			},
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotValidatedOptions, err := filterRsyncInfoOptions(tt.args)
			if !reflect.DeepEqual(gotValidatedOptions, tt.wantValidatedOptions) {
				t.Errorf("filterRsyncExtraOptions() gotValidatedOptions = %v, want %v", gotValidatedOptions, tt.wantValidatedOptions)
			}
			if (err == nil) == tt.wantError {
				t.Errorf("filterRsyncExtraOptions() got error %#v, want error %#v", err, tt.wantError)
			}
		})
	}
}
