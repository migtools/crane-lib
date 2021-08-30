package stunnel

import (
	"bytes"
	"fmt"

	"github.com/konveyor/crane-lib/state_transfer/endpoint"
	"github.com/konveyor/crane-lib/state_transfer/meta"
	"github.com/konveyor/crane-lib/state_transfer/transport"
	"k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"

	corev1 "k8s.io/api/core/v1"
)

const (
	defaultStunnelImage        = "quay.io/konveyor/rsync-transfer:latest"
	defaultStunnelServerConfig = "crane2-stunnel-server-config"
	defaultStunnelServerSecret = "crane2-stunnel-server-secret"
	defaultStunnelClientConfig = "crane2-stunnel-client-config"
	defaultStunnelClientSecret = "crane2-stunnel-client-secret"
)

const (
	TransportTypeStunnel = "stunnel"
	StunnelContainer     = "stunnel"
)

type StunnelTransport struct {
	crt              *bytes.Buffer
	key              *bytes.Buffer
	ca               *bytes.Buffer
	port             int32
	serverContainers []corev1.Container
	serverVolumes    []corev1.Volume
	clientContainers []corev1.Container
	clientVolumes    []corev1.Volume
	direct           bool
	options          *transport.Options
	noVerifyCA       bool
	caVerifyLevel    string
	nsNamePair       meta.NamespacedNamePair
}

func NewTransport(nsNamePair meta.NamespacedNamePair, options *transport.Options) transport.Transport {
	return &StunnelTransport{
		nsNamePair: nsNamePair,
		options:    options,
	}
}

func (s *StunnelTransport) CA() *bytes.Buffer {
	return s.ca
}

func (s *StunnelTransport) Crt() *bytes.Buffer {
	return s.crt
}

func (s *StunnelTransport) Key() *bytes.Buffer {
	return s.key
}

func (s *StunnelTransport) Port() int32 {
	return s.port
}

func (s *StunnelTransport) ExposedPort() int32 {
	return int32(2222)
}

func (s *StunnelTransport) ClientContainers() []corev1.Container {
	return s.clientContainers
}

func (s *StunnelTransport) ServerContainers() []corev1.Container {
	return s.serverContainers
}

func (s *StunnelTransport) ClientVolumes() []corev1.Volume {
	return s.clientVolumes
}

func (s *StunnelTransport) ServerVolumes() []corev1.Volume {
	return s.serverVolumes
}

func (s *StunnelTransport) Direct() bool {
	return s.direct
}

func (s *StunnelTransport) NamespacedNamePair() meta.NamespacedNamePair {
	return s.nsNamePair
}

func (s *StunnelTransport) Type() transport.TransportType {
	return transport.TransportType(TransportTypeStunnel)
}

func (s *StunnelTransport) getStunnelServerImage() string {
	if s.options != nil && s.options.StunnelServerImage != "" {
		return s.options.StunnelServerImage
	} else {
		return defaultStunnelImage
	}
}

func (s *StunnelTransport) getStunnelClientImage() string {
	if s.options != nil && s.options.StunnelClientImage != "" {
		return s.options.StunnelClientImage
	} else {
		return defaultStunnelImage
	}
}

// GetTransportFromKubeObjects checks if the required configmaps and secretes are created for the transport
//. It populates the fields for the Transport needed for transfer object.
func GetTransportFromKubeObjects(srcClient client.Client, destClient client.Client, nnPair meta.NamespacedNamePair, e endpoint.Endpoint) (transport.Transport, error) {
	_, err := getClientConfig(srcClient, nnPair.Source())
	switch {
	case errors.IsNotFound(err):
		fmt.Printf("transport: %s Client Config is not created", nnPair.Source())
		return nil, err
	case err != nil:
		return nil, err
	}

	_, err = getServerConfig(destClient, nnPair.Destination())
	switch {
	case errors.IsNotFound(err):
		fmt.Printf("transport: %s Server Config is not created", nnPair.Destination())
		return nil, err
	case err != nil:
		return nil, err
	}

	clientSecretCreated, err := getClientSecret(srcClient, nnPair.Source())
	switch {
	case errors.IsNotFound(err):
		fmt.Printf("transport: %s Client secret is not created", nnPair.Source())
		return nil, err
	case err != nil:
		return nil, err
	}

	_, err = getServerSecret(destClient, nnPair.Destination())
	switch {
	case errors.IsNotFound(err):
		fmt.Printf("transport: %s Server secret is not created", nnPair.Destination())
		return nil, err
	case err != nil:
		return nil, err
	}

	s := &StunnelTransport{
		port: e.Port(),
	}

	key, ok := clientSecretCreated.Data["tls.key"]
	if !ok {
		fmt.Printf("invalid secret for transport %s, tls.key key not found", nnPair.Source())
		return nil, fmt.Errorf("invalid secret for transport %s, tls.key key not found", nnPair.Source())
	}

	crt, ok := clientSecretCreated.Data["tls.crt"]
	if !ok {
		fmt.Printf("invalid secret for transport %s, tls.crt key not found", nnPair.Source())
		return nil, fmt.Errorf("invalid secret for transport %s, tls.crt key not found", nnPair.Source())
	}

	s.key = bytes.NewBuffer(key)
	s.crt = bytes.NewBuffer(crt)

	createStunnelServerVolumes(s)
	createClientVolumes(s)
	setClientContainers(s, e)
	createStunnelServerContainers(s, e)
	s.nsNamePair = nnPair
	return s, nil
}

func (s *StunnelTransport) Options() *transport.Options {
	return s.options
}
