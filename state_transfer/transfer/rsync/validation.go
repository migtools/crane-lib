package rsync

import (
	"fmt"
	"regexp"

	"github.com/konveyor/crane-lib/state_transfer/transfer"
	errorsutil "k8s.io/apimachinery/pkg/util/errors"
)

const (
	validatedPVCNameMaxLength = 63
	pvcNameBadCharacters      = `[\\.]+`
)

// validatePVCList validates list of PVCs provided to rsync transfer
func validatePVCList(pvcList transfer.PersistentVolumeClaimList) error {
	srcNamespaces := pvcList.GetSourceNamespaces()
	destNamespaces := pvcList.GetDestinationNamespaces()
	if len(srcNamespaces) > 1 || len(destNamespaces) > 1 {
		return fmt.Errorf("rsync transfer does not support migrating PVCs belonging to multiple source/destination namespaces")
	}
	if len(pvcList) == 0 {
		return fmt.Errorf("at least one pvc must be provided")
	}
	validationErrors := []error{}
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
	return errorsutil.NewAggregate(validationErrors)
}

// validatePVCName validates pvc names for rsync transfer
func validatePVCName(pvc transfer.PVC) error {
	if len(pvc.Source().ValidatedName()) > validatedPVCNameMaxLength ||
		len(pvc.Destination().ValidatedName()) > validatedPVCNameMaxLength {
		return fmt.Errorf("validated pvc name cannot be longer than %d characters", validatedPVCNameMaxLength)
	}
	r := regexp.MustCompile(pvcNameBadCharacters)
	if r.MatchString(pvc.Source().ValidatedName()) || r.MatchString(pvc.Destination().ValidatedName()) {
		return fmt.Errorf("validated pvc name must not contain '.' or '\\' ")
	}
	return nil
}
