package state_transfer

import (
	"bytes"

	v1 "k8s.io/api/core/v1"
)

const (
	socatImage = "quay.io/jmontleon/socat:latest"
	socatPort  = int32(1025)
)

type NullTransport struct {
	Crt              *bytes.Buffer
	Key              *bytes.Buffer
	CA               *bytes.Buffer
	TransportPort    int32
	ServerContainers []v1.Container
	ServerVolumes    []v1.Volume
	ClientContainers []v1.Container
	ClientVolumes    []v1.Volume
}

func (s *NullTransport) SetCA(b *bytes.Buffer) {
	s.CA = b
}

func (s *NullTransport) GetCA() *bytes.Buffer {
	return s.CA
}

func (s *NullTransport) SetCrt(b *bytes.Buffer) {
	s.Crt = b
}

func (s *NullTransport) GetCrt() *bytes.Buffer {
	return s.Crt
}

func (s *NullTransport) SetKey(b *bytes.Buffer) {
	s.Key = b
}

func (s *NullTransport) GetKey() *bytes.Buffer {
	return s.Key
}

func (s *NullTransport) SetTransportPort(transportPort int32) {
	s.TransportPort = transportPort
}

func (s *NullTransport) GetTransportPort() int32 {
	return s.TransportPort
}

func (s *NullTransport) SetClientContainers(containers []v1.Container) {
	s.ClientContainers = containers
}

func (s *NullTransport) GetClientContainers() []v1.Container {
	return s.ClientContainers
}

func (s *NullTransport) SetServerContainers(containers []v1.Container) {
	s.ServerContainers = containers
}

func (s *NullTransport) GetServerContainers() []v1.Container {
	return s.ServerContainers
}

func (s *NullTransport) SetClientVolumes(volumes []v1.Volume) {
	s.ClientVolumes = volumes
}

func (s *NullTransport) GetClientVolumes() []v1.Volume {
	return s.ClientVolumes
}

func (s *NullTransport) SetServerVolumes(volumes []v1.Volume) {
	s.ServerVolumes = volumes
}

func (s *NullTransport) GetServerVolumes() []v1.Volume {
	return s.ServerVolumes
}
