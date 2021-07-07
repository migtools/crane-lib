package null

import (
	"github.com/konveyor/crane-lib/state_transfer/endpoint"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func (s *NullTransport) CreateServer(c client.Client, e endpoint.Endpoint) error {
	s.direct = true
	s.SetPort(e.Port())
	return nil
}
