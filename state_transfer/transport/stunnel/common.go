package stunnel

import (
	"bytes"

	v1 "k8s.io/api/core/v1"
)

const (
	stunnelPort         = int32(1443)
	stunnelImage        = "quay.io/konveyor/rsync-transfer:latest"
	stunnelConfigPrefix = "crane2-stunnel-config-"
	stunnelSecretPrefix = "crane2-stunnel-secret-"
)

type StunnelTransport struct {
	crt              *bytes.Buffer
	key              *bytes.Buffer
	ca               *bytes.Buffer
	port             int32
	serverContainers []v1.Container
	serverVolumes    []v1.Volume
	clientContainers []v1.Container
	clientVolumes    []v1.Volume
	direct           bool
}

func (s *StunnelTransport) SetCA(b *bytes.Buffer) {
	s.ca = b
}

func (s *StunnelTransport) CA() *bytes.Buffer {
	return s.ca
}

func (s *StunnelTransport) SetCrt(b *bytes.Buffer) {
	s.crt = b
}

func (s *StunnelTransport) Crt() *bytes.Buffer {
	return s.crt
}

func (s *StunnelTransport) SetKey(b *bytes.Buffer) {
	s.key = b
}

func (s *StunnelTransport) Key() *bytes.Buffer {
	return s.key
}

func (s *StunnelTransport) SetPort(transportPort int32) {
	s.port = transportPort
}

func (s *StunnelTransport) Port() int32 {
	return s.port
}

func (s *StunnelTransport) SetClientContainers(containers []v1.Container) {
	s.clientContainers = containers
}

func (s *StunnelTransport) ClientContainers() []v1.Container {
	return s.clientContainers
}

func (s *StunnelTransport) SetServerContainers(containers []v1.Container) {
	s.serverContainers = containers
}

func (s *StunnelTransport) ServerContainers() []v1.Container {
	return s.serverContainers
}

func (s *StunnelTransport) SetClientVolumes(volumes []v1.Volume) {
	s.clientVolumes = volumes
}

func (s *StunnelTransport) ClientVolumes() []v1.Volume {
	return s.clientVolumes
}

func (s *StunnelTransport) SetServerVolumes(volumes []v1.Volume) {
	s.serverVolumes = volumes
}

func (s *StunnelTransport) ServerVolumes() []v1.Volume {
	return s.serverVolumes
}

func (s *StunnelTransport) Direct() bool {
	return s.direct
}
