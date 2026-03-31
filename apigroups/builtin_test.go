package apigroups

import "testing"

func TestIsDefaultBuiltinAPIGroup(t *testing.T) {
	tests := []struct {
		group string
		want  bool
	}{
		{"", true},
		{"apps", true},
		{"rbac.authorization.k8s.io", true},
		{"operators.coreos.com", true},
		{"route.openshift.io", true},
		{"config.openshift.io", true},
		{"example.com", false},
		{"widgets.example.com", false},
	}

	for _, tt := range tests {
		t.Run(tt.group, func(t *testing.T) {
			if got := IsDefaultBuiltinAPIGroup(tt.group); got != tt.want {
				t.Fatalf("IsDefaultBuiltinAPIGroup(%q)=%v want %v", tt.group, got, tt.want)
			}
		})
	}
}
