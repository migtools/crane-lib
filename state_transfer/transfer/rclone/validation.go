package rclone

import (
	"fmt"

	"github.com/konveyor/crane-lib/state_transfer/transfer"
)

// validatePVCList validates list of PVCs provided to rclone transfer
func validatePVCList(pvcList transfer.PersistentVolumeClaimList) error {
	if len(pvcList) > 1 {
		return fmt.Errorf("unimplemented: rclone transfer does not support multiple pvcs")
	}
	if len(pvcList) == 0 {
		return fmt.Errorf("at least one pvc must be provided")
	}
	return nil
}
