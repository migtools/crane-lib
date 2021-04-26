package state_transfer

import (
	"bytes"

	v1 "k8s.io/api/core/v1"
)

const (
	stunnelPort  = int32(1443)
	stunnelImage = "quay.io/konveyor/rsync-transfer:latest"
)

type StunnelTransport struct {
	Crt              *bytes.Buffer
	Key              *bytes.Buffer
	CA               *bytes.Buffer
	TransportPort    int32
	ServerContainers []v1.Container
	ServerVolumes    []v1.Volume
	ClientContainers []v1.Container
	ClientVolumes    []v1.Volume
}

func (s *StunnelTransport) SetCA(b *bytes.Buffer) {
	s.CA = b
}

func (s *StunnelTransport) GetCA() *bytes.Buffer {
	return s.CA
}

func (s *StunnelTransport) SetCrt(b *bytes.Buffer) {
	s.Crt = b
}

func (s *StunnelTransport) GetCrt() *bytes.Buffer {
	return s.Crt
}

func (s *StunnelTransport) SetKey(b *bytes.Buffer) {
	s.Key = b
}

func (s *StunnelTransport) GetKey() *bytes.Buffer {
	return s.Key
}

func (s *StunnelTransport) SetTransportPort(transportPort int32) {
	s.TransportPort = transportPort
}

func (s *StunnelTransport) GetTransportPort() int32 {
	return s.TransportPort
}

func (s *StunnelTransport) SetClientContainers(containers []v1.Container) {
	s.ClientContainers = containers
}

func (s *StunnelTransport) GetClientContainers() []v1.Container {
	return s.ClientContainers
}

func (s *StunnelTransport) SetServerContainers(containers []v1.Container) {
	s.ServerContainers = containers
}

func (s *StunnelTransport) GetServerContainers() []v1.Container {
	return s.ServerContainers
}

func (s *StunnelTransport) SetClientVolumes(volumes []v1.Volume) {
	s.ClientVolumes = volumes
}

func (s *StunnelTransport) GetClientVolumes() []v1.Volume {
	return s.ClientVolumes
}

func (s *StunnelTransport) SetServerVolumes(volumes []v1.Volume) {
	s.ServerVolumes = volumes
}

func (s *StunnelTransport) GetServerVolumes() []v1.Volume {
	return s.ServerVolumes
}
