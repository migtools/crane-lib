package null

import (
	"github.com/konveyor/crane-lib/state_transfer/endpoint"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func (s *NullTransport) CreateClient(c client.Client, endpoint endpoint.Endpoint) error {
	return nil
}
