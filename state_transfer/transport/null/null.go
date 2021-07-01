package null

import (
	"bytes"

	"github.com/konveyor/crane-lib/state_transfer/transport"

	v1 "k8s.io/api/core/v1"
)

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
	proxyOptions     *transport.ProxyOptions
	noVerifyCA       bool
	caVerifyLevel    string
}

func NewTransport() transport.Transport {
	return &NullTransport{}
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

func (s *NullTransport) SetProxyOptions(proxyOptions *transport.ProxyOptions) {
	s.proxyOptions = proxyOptions
}

func (s *NullTransport) ProxyOptions() *transport.ProxyOptions {
	return s.proxyOptions
}

func (s *NullTransport) SetNoVerifyCA(noVerifyCA bool) {
	s.noVerifyCA = noVerifyCA
}

func (s *NullTransport) NoVerifyCA() bool {
	return s.noVerifyCA
}

func (s *NullTransport) SetCAVerifyLevel(caVerifyLevel string) {
	s.caVerifyLevel = caVerifyLevel
}

func (s *NullTransport) CAVerifyLevel() string {
	return s.caVerifyLevel
}
