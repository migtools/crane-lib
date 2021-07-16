package rsync

import (
	"fmt"

	"github.com/konveyor/crane-lib/state_transfer/transfer"
	errorsutil "k8s.io/apimachinery/pkg/util/errors"
	validation "k8s.io/apimachinery/pkg/util/validation"
)

// validatePVCList validates list of PVCs provided to rsync transfer
// list cannot contain pvcs belonging to two or more source/destination namespaces
// list must contain at least one pvc
// labelSafeNames of all pvcs must be valid label values
// labelSafeNames must be unique within the namespace of the pvc
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
	}

	for _, pvc := range pvcList {
		if err := validatePVCName(pvc); err != nil {
			validationErrors = append(
				validationErrors,
				errorsutil.NewAggregate([]error{
					fmt.Errorf("pvc name validation failed for pvc %s with error", pvc.Source().Claim().Name),
					err,
				}))
		}
	}

	// TODO: add validation to check uniqueness of label safe pvc names within source/destination namespaces
	return errorsutil.NewAggregate(validationErrors)
}

// validatePVCName validates pvc names for rsync transfer
func validatePVCName(pvc transfer.PVCPair) error {
	validationErrors := []error{}
	if errs := validation.IsValidLabelValue(pvc.Source().LabelSafeName()); len(errs) > 0 {
		validationErrors = append(validationErrors,
			fmt.Errorf("labelSafeName() for %s must be a valid label value", pvc.Source().NamespacedName()))
	}
	if errs := validation.IsValidLabelValue(pvc.Destination().LabelSafeName()); len(errs) > 0 {
		validationErrors = append(validationErrors,
			fmt.Errorf("labelSafeName() for %s must be a valid label value", pvc.Destination().NamespacedName()))
	}
	return errorsutil.NewAggregate(validationErrors)
}
