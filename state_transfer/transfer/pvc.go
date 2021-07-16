package transfer

import (
	"crypto/md5"
	"encoding/hex"
	"fmt"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
)

// PVCPairList defines a managed list of PVCPair
type PVCPairList []PVCPair

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
	// the label safe name may be validated differently by different transfers
	LabelSafeName() string
	// NamespacedName returns namespaced name of the claim
	NamespacedName() types.NamespacedName
}

type pvc struct {
	p *v1.PersistentVolumeClaim
}

func (p pvc) Claim() *v1.PersistentVolumeClaim {
	return p.p
}

func (p pvc) LabelSafeName() string {
	return getMD5Hash(p.p.Name)
}

func (p pvc) NamespacedName() types.NamespacedName {
	return types.NamespacedName{
		Name:      p.p.Name,
		Namespace: p.p.Namespace,
	}
}

type pvcPair struct {
	src  PVC
	dest PVC
}

func (p pvcPair) Source() PVC {
	return p.src
}

func (p pvcPair) Destination() PVC {
	return p.dest
}

// NewPVCPair when given references to a source and a destination PersistentVolumeClaim,
// returns a PVCPair to be used in transfers
func NewPVCPair(src *v1.PersistentVolumeClaim, dest *v1.PersistentVolumeClaim) PVCPair {
	srcPvc := pvc{p: src}
	destPvc := pvc{p: dest}
	newPvcPair := pvcPair{
		src:  srcPvc,
		dest: destPvc,
	}
	if dest == nil {
		newPvcPair.dest = srcPvc
	} else {
		newPvcPair.dest = destPvc
	}
	return newPvcPair
}

// NewPVCPairList when given a list of PVCPair, returns a managed list
func NewPVCPairList(pvcs ...PVCPair) (PVCPairList, error) {
	pvcList := PVCPairList{}
	for _, p := range pvcs {
		newPvc := pvcPair{}
		if p.Source() == nil {
			return nil, fmt.Errorf("source pvc definition cannot be nil")
		}
		newPvc.src = p.Source()
		if p.Destination() == nil {
			newPvc.dest = p.Source()
		} else {
			newPvc.dest = p.Destination()
		}
		pvcList = append(pvcList, &newPvc)
	}
	return pvcList, nil
}

// GetSourceNamespaces returns all source namespaces present in the list of pvcs
func (p PVCPairList) GetSourceNamespaces() (namespaces []string) {
	nsSet := map[string]bool{}
	for i := range p {
		pvcPair := p[i]
		if _, exists := nsSet[pvcPair.Source().Claim().Namespace]; !exists {
			nsSet[pvcPair.Source().Claim().Namespace] = true
			namespaces = append(namespaces, pvcPair.Source().Claim().Namespace)
		}
	}
	return
}

// GetDestinationNamespaces returns all destination namespaces present in the list of pvcs
func (p PVCPairList) GetDestinationNamespaces() (namespaces []string) {
	nsSet := map[string]bool{}
	for i := range p {
		pvcPair := p[i]
		if _, exists := nsSet[pvcPair.Destination().Claim().Namespace]; !exists {
			nsSet[pvcPair.Destination().Claim().Namespace] = true
			namespaces = append(namespaces, pvcPair.Destination().Claim().Namespace)
		}
	}
	return
}

// InSourceNamespace given a source namspace, returns a list of pvcs belonging to that namespace
func (p PVCPairList) InSourceNamespace(ns string) []PVCPair {
	pvcList := []PVCPair{}
	for i := range p {
		pvcPair := p[i]
		if pvcPair.Source().Claim().Namespace == ns {
			pvcList = append(pvcList, pvcPair)
		}
	}
	return pvcList
}

// InDestinationNamespace given a destination namespace, returns a list of pvcs that will be migrated to it
func (p PVCPairList) InDestinationNamespace(ns string) []PVCPair {
	pvcList := []PVCPair{}
	for i := range p {
		pvcPair := p[i]
		if pvcPair.Destination().Claim().Namespace == ns {
			pvcList = append(pvcList, pvcPair)
		}
	}
	return pvcList
}

// GetSourcePVC returns matching PVC from the managed list
func (p PVCPairList) GetSourcePVC(nsName types.NamespacedName) *PVCPair {
	for i := range p {
		pvcPair := p[i]
		if pvcPair.Source().Claim().Namespace == nsName.Namespace &&
			pvcPair.Source().Claim().Name == nsName.Name {
			return &pvcPair
		}
	}
	return nil
}

// GroupBySourceNamespaces returns lists of PVCs indexed by their source namespaces
func (p PVCPairList) GroupBySourceNamespaces() map[string][]PVCPair {
	nsToPVCMap := make(map[string][]PVCPair)
	for i := range p {
		pvcPair := p[i]
		if _, exists := nsToPVCMap[pvcPair.Source().Claim().Namespace]; !exists {
			nsToPVCMap[pvcPair.Source().Claim().Namespace] = make([]PVCPair, 0)
		} else {
			nsToPVCMap[pvcPair.Source().Claim().Namespace] = append(nsToPVCMap[pvcPair.Source().Claim().Namespace], pvcPair)
		}
	}
	return nsToPVCMap
}

func getMD5Hash(s string) string {
	hash := md5.Sum([]byte(s))
	return hex.EncodeToString(hash[:])
}
