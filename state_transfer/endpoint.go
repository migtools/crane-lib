package state_transfer

import "sigs.k8s.io/controller-runtime/pkg/client"

type Endpoint interface {
	createEndpointResources(client.Client, Transfer) error
	SetHostname(string)
	Hostname() string
	SetPort(int32)
	Port() int32
}

func CreateEndpoint(e Endpoint, c client.Client, t Transfer) (Endpoint, error) {
	err := e.createEndpointResources(c, t)
	if err != nil {
		return nil, err
	}
	return e, nil
}

func DestroyEndpoint(e Endpoint) error {
	return nil
}
