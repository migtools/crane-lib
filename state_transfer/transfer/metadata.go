package transfer

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ResourceMetadata defines any metadata used to create intermediary resources for state transfer
type ResourceMetadata struct {
	Annotations     map[string]string
	Labels          map[string]string
	OwnerReferences []metav1.OwnerReference
}
