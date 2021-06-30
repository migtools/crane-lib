package endpoint

import (
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	EndpointTypeRoutePassthrough  = "EndpointTypeRoutePassthrough"
	EndpointTypeRouteInsecureEdge = "EndpointTypeRouteInsecureEdge"
)

type EndpointType string

type Endpoint interface {
	Create(client.Client) error
	SetHostname(string)
	Hostname() string
	SetPort(int32)
	Port() int32
	Name() string
	Namespace() string
	Labels() map[string]string
	Type() EndpointType
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
