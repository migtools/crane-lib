package null

import (
	"bytes"

	"github.com/konveyor/crane-lib/state_transfer/endpoint"
	"github.com/konveyor/crane-lib/state_transfer/meta"
	"github.com/konveyor/crane-lib/state_transfer/transport"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v1 "k8s.io/api/core/v1"
)

const TransportTypeNull = "null"

type NullTransport struct {
	crt              *bytes.Buffer
	key              *bytes.Buffer
	ca               *bytes.Buffer
	port             int32
	serverContainers []v1.Container
	serverVolumes    []v1.Volume
	clientContainers []v1.Container
	clientVolumes    []v1.Volume
	direct           bool
	options          *transport.Options
	nsNamePair       meta.NamespacedNamePair
}

func NewTransport(nsNamePair meta.NamespacedNamePair) transport.Transport {
	return &NullTransport{
		nsNamePair: nsNamePair,
	}
}

func (s *NullTransport) CA() *bytes.Buffer {
	return s.ca
}

func (s *NullTransport) Crt() *bytes.Buffer {
	return s.crt
}

func (s *NullTransport) Key() *bytes.Buffer {
	return s.key
}

func (s *NullTransport) Port() int32 {
	return s.port
}

func (s *NullTransport) ExposedPort() int32 {
	return s.port
}

func (s *NullTransport) ClientContainers() []v1.Container {
	return s.clientContainers
}

func (s *NullTransport) ServerContainers() []v1.Container {
	return s.serverContainers
}

func (s *NullTransport) ClientVolumes() []v1.Volume {
	return s.clientVolumes
}

func (s *NullTransport) ServerVolumes() []v1.Volume {
	return s.serverVolumes
}

func (s *NullTransport) Direct() bool {
	return s.direct
}

func (s *NullTransport) Options() *transport.Options {
	return s.options
}

func (s *NullTransport) NamespacedNamePair() meta.NamespacedNamePair {
	return s.nsNamePair
}

func (s *NullTransport) Type() transport.TransportType {
	return transport.TransportType(TransportTypeNull)
}

func (s *NullTransport) IsHealthy(_, _ client.Client, _ endpoint.Endpoint) (bool, error) {
	return true, nil
}
