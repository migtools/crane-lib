package apigroups

import "strings"

// builtinK8sAPIGroups lists in-tree Kubernetes API groups that are not backed by
// user-defined CRDs. Extend when new first-party groups appear.
var builtinK8sAPIGroups = map[string]struct{}{
	"":                                {},
	"apps":                            {},
	"batch":                           {},
	"autoscaling":                     {},
	"policy":                          {},
	"networking.k8s.io":               {},
	"rbac.authorization.k8s.io":       {},
	"storage.k8s.io":                  {},
	"admissionregistration.k8s.io":    {},
	"certificates.k8s.io":             {},
	"coordination.k8s.io":             {},
	"discovery.k8s.io":                {},
	"events.k8s.io":                   {},
	"flowcontrol.apiserver.k8s.io":    {},
	"node.k8s.io":                     {},
	"scheduling.k8s.io":               {},
	"apiextensions.k8s.io":            {},
	"apiregistration.k8s.io":          {},
	"resource.k8s.io":                 {},
	"authentication.k8s.io":           {},
	"authorization.k8s.io":            {},
	"extensions":                      {},
	"metrics.k8s.io":                  {},
	"imagepolicy.k8s.io":              {},
	"internal.apiserver.k8s.io":       {},
	"operators.coreos.com":            {},
	"packages.operators.coreos.com":   {},
	"monitoring.coreos.com":           {},
}

// IsDefaultBuiltinAPIGroup reports whether group is treated as built-in by default
// for CRD export filtering.
func IsDefaultBuiltinAPIGroup(group string) bool {
	if _, ok := builtinK8sAPIGroups[group]; ok {
		return true
	}
	if strings.HasSuffix(group, ".openshift.io") {
		return true
	}
	return false
}
