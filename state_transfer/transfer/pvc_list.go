package transfer

import (
	"crypto/md5"
	"encoding/hex"
	"fmt"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
)

const (
	kubeVirtAnnKey      = "cdi.kubevirt.io/storage.contentType"
	kubevirtContentType = "kubevirt"
)

// PVCPairList defines a managed list of PVCPair
type PVCPairList []PVCPair

// pvc represents a PersistentVolumeClaim
type pvc struct {
	p *v1.PersistentVolumeClaim
}

// Claim returns ref to associated PersistentVolumeClaim
func (p pvc) Claim() *v1.PersistentVolumeClaim {
	return p.p
}

// LabelSafeName returns a name which is guaranteed to be a safe label value
func (p pvc) LabelSafeName() string {
	if p.p == nil {
		return ""
	}
	return getMD5Hash(p.p.Name)
}

// pvcPair defines a source and a destination PersistentVolumeClaim
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

// NewFilesystemPVCPairList when given a list of PVCPair, returns a managed list
func NewBlockOrVMDiskPVCPairList(pvcs ...PVCPair) (PVCPairList, error) {
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

		if isBlockOrVMDisk(newPvc.src.Claim()) && isBlockOrVMDisk(newPvc.dest.Claim()) {
			pvcList = append(pvcList, &newPvc)
		}
		if isBlockOrVMDisk(newPvc.src.Claim()) && !isBlockOrVMDisk(newPvc.dest.Claim()) ||
			!isBlockOrVMDisk(newPvc.src.Claim()) && isBlockOrVMDisk(newPvc.dest.Claim()) {
			return nil, fmt.Errorf("source and destination must be the same type of volume")
		}
	}
	return pvcList, nil
}

// NewFilesystemPVCPairList when given a list of PVCPair, returns a managed list
func NewFilesystemPVCPairList(pvcs ...PVCPair) (PVCPairList, error) {
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

		if !isBlockOrVMDisk(newPvc.src.Claim()) && !isBlockOrVMDisk(newPvc.dest.Claim()) {
			pvcList = append(pvcList, &newPvc)
		}
		if isBlockOrVMDisk(newPvc.src.Claim()) && !isBlockOrVMDisk(newPvc.dest.Claim()) ||
			!isBlockOrVMDisk(newPvc.src.Claim()) && isBlockOrVMDisk(newPvc.dest.Claim()) {
			return nil, fmt.Errorf("source and destination must be the same type of volume")
		}
	}
	return pvcList, nil
}

func isBlockOrVMDisk(pvc *v1.PersistentVolumeClaim) bool {
	if pvc == nil {
		return false
	}
	isBlock := pvc.Spec.VolumeMode != nil && *pvc.Spec.VolumeMode == v1.PersistentVolumeBlock
	if !isBlock {
		if v, ok := pvc.GetAnnotations()[kubeVirtAnnKey]; !ok || v != kubevirtContentType {
			return false
		}
	}
	return isBlock
}

// GetSourceNamespaces returns all source namespaces present in the list of pvcs
func (p PVCPairList) GetSourceNamespaces() (namespaces []string) {
	nsSet := map[string]bool{}
	for i := range p {
		pvcPair := p[i]
		if pvcPair != nil && pvcPair.Source() != nil && pvcPair.Source().Claim() != nil {
			if _, exists := nsSet[pvcPair.Source().Claim().Namespace]; !exists {
				nsSet[pvcPair.Source().Claim().Namespace] = true
				namespaces = append(namespaces, pvcPair.Source().Claim().Namespace)
			}
		}
	}
	return
}

// GetDestinationNamespaces returns all destination namespaces present in the list of pvcs
func (p PVCPairList) GetDestinationNamespaces() (namespaces []string) {
	nsSet := map[string]bool{}
	for i := range p {
		pvcPair := p[i]
		if pvcPair != nil && pvcPair.Source() != nil && pvcPair.Source().Claim() != nil {
			if _, exists := nsSet[pvcPair.Destination().Claim().Namespace]; !exists {
				nsSet[pvcPair.Destination().Claim().Namespace] = true
				namespaces = append(namespaces, pvcPair.Destination().Claim().Namespace)
			}
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
