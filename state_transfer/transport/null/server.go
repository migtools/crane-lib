package null

import (
	"github.com/konveyor/crane-lib/state_transfer/endpoint"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func (s *NullTransport) CreateServer(c client.Client, prefix string, e endpoint.Endpoint) error {
	s.direct = true
	s.port = e.Port()
	return nil
}
