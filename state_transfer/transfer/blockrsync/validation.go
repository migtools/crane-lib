package blockrsync

import (
	"fmt"

	"github.com/konveyor/crane-lib/state_transfer/transfer"
	corev1 "k8s.io/api/core/v1"
	errorsutil "k8s.io/apimachinery/pkg/util/errors"
	validation "k8s.io/apimachinery/pkg/util/validation"
)

const (
	kubeVirtAnnKey      = "cdi.kubevirt.io/storage.contentType"
	kubevirtContentType = "kubevirt"
)

// validatePVCList validates list of PVCs provided to blockrsync transfer
// list cannot contain pvcs belonging to two or more source/destination namespaces
// list must contain at exactly one pvc
// labelSafeNames of all pvcs must be valid label values
// labelSafeNames must be unique within the namespace of the pvc
// volume mode must be block or filesystem if the pvc has an annotation that indicates
// it is a kubevirt disk pvc.
func validatePVCList(pvcList transfer.PVCPairList) error {
	validationErrors := []error{}

	srcNamespaces := pvcList.GetSourceNamespaces()
	destNamespaces := pvcList.GetDestinationNamespaces()
	if len(srcNamespaces) > 1 || len(destNamespaces) > 1 {
		validationErrors = append(validationErrors,
			fmt.Errorf("rsync transfer does not support migrating PVCs belonging to multiple source/destination namespaces"))
	}

	if len(pvcList) == 0 {
		validationErrors = append(validationErrors, fmt.Errorf("at least one pvc must be provided"))
	} else {
		if err := validatePVCName(pvcList[0]); err != nil {
			validationErrors = append(
				validationErrors,
				errorsutil.NewAggregate([]error{
					fmt.Errorf("pvc name validation failed for pvc %s with error", pvcList[0].Source().Claim().Name),
					err,
				}))
		}
	}
	return errorsutil.NewAggregate(validationErrors)
}

// validatePVCName validates pvc names for blockrsync transfer
func validatePVCName(pvcPair transfer.PVCPair) error {
	validationErrors := []error{}
	if errs := validation.IsValidLabelValue(pvcPair.Source().LabelSafeName()); len(errs) > 0 {
		validationErrors = append(validationErrors,
			fmt.Errorf("labelSafeName() for %s must be a valid label value", pvcPair.Source().Claim().Name))
	}
	if errs := validation.IsValidLabelValue(pvcPair.Destination().LabelSafeName()); len(errs) > 0 {
		validationErrors = append(validationErrors,
			fmt.Errorf("labelSafeName() for %s must be a valid label value", pvcPair.Destination().Claim().Name))
	}
	if err := isBlockOrKubeVirtDisk(pvcPair.Source().Claim()); err != nil {
		validationErrors = append(validationErrors, err)
	}
	if err := isBlockOrKubeVirtDisk(pvcPair.Destination().Claim()); err != nil {
		validationErrors = append(validationErrors, err)
	}
	pvcPair.Source().Claim()
	return errorsutil.NewAggregate(validationErrors)
}

func isPVCBlock(pvc *corev1.PersistentVolumeClaim) bool {
	return pvc.Spec.VolumeMode != nil && *pvc.Spec.VolumeMode == corev1.PersistentVolumeBlock
}

func isBlockOrKubeVirtDisk(pvc *corev1.PersistentVolumeClaim) error {
	if !isPVCBlock(pvc) {
		if v, ok := pvc.GetAnnotations()[kubeVirtAnnKey]; !ok || v != kubevirtContentType {
			return fmt.Errorf("%s is not a block, or VM disk volume", pvc.Name)
		}
	}
	return nil
}
