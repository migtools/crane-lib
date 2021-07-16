package transfer

import (
	v1 "k8s.io/api/core/v1"
)

// PVCPair knows how to return source and destination
// PVC objects for a state transfer
type PVCPair interface {
	// Source returns PVC representing source PersistentVolumeClaim
	Source() PVC
	// Destination returns PVC representing destination PersistentVolumeClaim
	Destination() PVC
}

// PVC knows how to return v1.PersistentVolumeClaim and an additional validated
// name which can be used by different transfers as per their own requirements
type PVC interface {
	// Claim returns the v1.PersistentVolumeClaim reference this PVC is associated with
	Claim() *v1.PersistentVolumeClaim
	// LabelSafeName returns a name for the PVC that can be used as a label value
	// it may be validated differently by different transfers
	LabelSafeName() string
}
