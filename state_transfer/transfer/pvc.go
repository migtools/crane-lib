package transfer

import (
	"crypto/md5"
	"encoding/hex"
	"fmt"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
)

type PersistentVolumeClaimList []PVC

type PVC interface {
	Source() PersistentVolumeClaim
	Destination() PersistentVolumeClaim
}

type PersistentVolumeClaim interface {
	Claim() *v1.PersistentVolumeClaim
	ValidatedName() string
}

type pvc struct {
	pvc *v1.PersistentVolumeClaim
}

func (p pvc) Claim() *v1.PersistentVolumeClaim {
	return p.pvc
}

func (p pvc) ValidatedName() string {
	return getMD5Hash(p.pvc.Name)
}

type pvcPair struct {
	src  PersistentVolumeClaim
	dest PersistentVolumeClaim
}

func (p pvcPair) Source() PersistentVolumeClaim {
	return p.src
}

func (p pvcPair) Destination() PersistentVolumeClaim {
	return p.dest
}

func NewPersistentVolumeClaim(src *v1.PersistentVolumeClaim, dest *v1.PersistentVolumeClaim) PVC {
	srcPvc := pvc{pvc: src}
	destPvc := pvc{pvc: dest}
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

func NewPersistentVolumeClaimList(pvcs ...PVC) (PersistentVolumeClaimList, error) {
	pvcList := PersistentVolumeClaimList{}
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
func (p PersistentVolumeClaimList) GetSourceNamespaces() (namespaces []string) {
	nsSet := map[string]bool{}
	for i := range p {
		pvc := p[i]
		if _, exists := nsSet[pvc.Source().Claim().Namespace]; !exists {
			nsSet[pvc.Source().Claim().Namespace] = true
			namespaces = append(namespaces, pvc.Source().Claim().Namespace)
		}
	}
	return
}

// GetDestinationNamespaces returns all destination namespaces present in the list of pvcs
func (p PersistentVolumeClaimList) GetDestinationNamespaces() (namespaces []string) {
	nsSet := map[string]bool{}
	for i := range p {
		pvc := p[i]
		if _, exists := nsSet[pvc.Destination().Claim().Namespace]; !exists {
			nsSet[pvc.Destination().Claim().Namespace] = true
			namespaces = append(namespaces, pvc.Destination().Claim().Namespace)
		}
	}
	return
}

// InSourceNamespace given a source namspace, returns a list of pvcs belonging to that namespace
func (p PersistentVolumeClaimList) InSourceNamespace(ns string) []PVC {
	pvcList := []PVC{}
	for i := range p {
		pvc := p[i]
		if pvc.Source().Claim().Namespace == ns {
			pvcList = append(pvcList, pvc)
		}
	}
	return pvcList
}

// InDestinationNamespace given a destination namespace, returns a list of pvcs that will be migrated to it
func (p PersistentVolumeClaimList) InDestinationNamespace(ns string) []PVC {
	pvcList := []PVC{}
	for i := range p {
		pvc := p[i]
		if pvc.Destination().Claim().Namespace == ns {
			pvcList = append(pvcList, pvc)
		}
	}
	return pvcList
}

// GetSourcePVC returns matching PVC from the managed list
func (p PersistentVolumeClaimList) GetSourcePVC(nsName types.NamespacedName) *PVC {
	for i := range p {
		pvc := p[i]
		if pvc.Source().Claim().Namespace == nsName.Namespace &&
			pvc.Source().Claim().Name == nsName.Name {
			return &pvc
		}
	}
	return nil
}

// GroupBySourceNamespaces returns lists of PVCs indexed by their source namespaces
func (p PersistentVolumeClaimList) GroupBySourceNamespaces() map[string][]PVC {
	nsToPVCMap := make(map[string][]PVC)
	for i := range p {
		pvc := p[i]
		if _, exists := nsToPVCMap[pvc.Source().Claim().Namespace]; !exists {
			nsToPVCMap[pvc.Source().Claim().Namespace] = make([]PVC, 0)
		} else {
			nsToPVCMap[pvc.Source().Claim().Namespace] = append(nsToPVCMap[pvc.Source().Claim().Namespace], pvc)
		}
	}
	return nsToPVCMap
}

func getMD5Hash(s string) string {
	hash := md5.Sum([]byte(s))
	return hex.EncodeToString(hash[:])
}
