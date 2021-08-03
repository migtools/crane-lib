package endpoint

import (
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Endpoint knows how to connect with a Transport or a Transfer
type Endpoint interface {
	// Create given a client, creates all kube resources
	// required for the endpoint to work and returns err
	Create(client.Client) error
	// Hostname returns a hostname for the endpoint
	Hostname() string
	// Port returns a backend port to which endpoint can connect
	// typically used by a Transport or a Transfer to accept local incoming connections
	Port() int32
	// ExposedPort returns a port which outsiders can use to connect to the endpoint
	// typically used by a Transport or a Transfer's remote counterparts to connect to the endpoint
	ExposedPort() int32
	// NamespacedName returns a ns name to identify this endpoint
	NamespacedName() types.NamespacedName
	// Labels returns labels used by this endpoint
	Labels() map[string]string
	// IsHealthy returns whether or not all Kube resources used by endpoint are healthy
	IsHealthy(c client.Client) (bool, error)
}

// Create creates a new endpoint
func Create(e Endpoint, c client.Client) (Endpoint, error) {
	err := e.Create(c)
	if err != nil {
		return nil, err
	}
	return e, nil
}

// Destroy destroys a given endpoint
func Destroy(e Endpoint) error {
	return nil
}
