package conversion

import (
	"fmt"

	"github.com/openperouter/openperouter/api/v1alpha1"
)

func ValidatePassthrough(l3Passthrough []v1alpha1.L3Passthrough) error {
	if len(l3Passthrough) > 1 {
		return fmt.Errorf("can't have more than one l3passthrough")
	}
	if l3Passthrough[0].Spec.HostSession == nil {
		return fmt.Errorf("host session is required for l3passthrough")
	}
	// host sessions are validated in ValidateHostSessions
	return nil
}
