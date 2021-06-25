package state_transfer

import (
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func (s *NullTransport) CreateServer(c client.Client, t Transfer) error {
	s.direct = true
	s.SetPort(t.Port())
	return nil
}
