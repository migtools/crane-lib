package endpoint

import (
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type Endpoint interface {
	Create(client.Client) error
	Hostname() string
	Port() int32
	Name() string
	Namespace() string
	Labels() map[string]string
	IsEndpointHealthy(c client.Client) (bool, error)
}

func CreateEndpoint(e Endpoint, c client.Client) (Endpoint, error) {
	err := e.Create(c)
	if err != nil {
		return nil, err
	}
	return e, nil
}

func DestroyEndpoint(e Endpoint) error {
	return nil
}
