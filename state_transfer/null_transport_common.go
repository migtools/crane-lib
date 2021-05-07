package state_transfer

import (
	"bytes"

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
}

func (s *NullTransport) SetCA(b *bytes.Buffer) {
	s.ca = b
}

func (s *NullTransport) CA() *bytes.Buffer {
	return s.ca
}

func (s *NullTransport) SetCrt(b *bytes.Buffer) {
	s.crt = b
}

func (s *NullTransport) Crt() *bytes.Buffer {
	return s.crt
}

func (s *NullTransport) SetKey(b *bytes.Buffer) {
	s.key = b
}

func (s *NullTransport) Key() *bytes.Buffer {
	return s.key
}

func (s *NullTransport) SetPort(transportPort int32) {
	s.port = transportPort
}

func (s *NullTransport) Port() int32 {
	return s.port
}

func (s *NullTransport) SetClientContainers(containers []v1.Container) {
	s.clientContainers = containers
}

func (s *NullTransport) ClientContainers() []v1.Container {
	return s.clientContainers
}

func (s *NullTransport) SetServerContainers(containers []v1.Container) {
	s.serverContainers = containers
}

func (s *NullTransport) ServerContainers() []v1.Container {
	return s.serverContainers
}

func (s *NullTransport) SetClientVolumes(volumes []v1.Volume) {
	s.clientVolumes = volumes
}

func (s *NullTransport) ClientVolumes() []v1.Volume {
	return s.clientVolumes
}

func (s *NullTransport) SetServerVolumes(volumes []v1.Volume) {
	s.serverVolumes = volumes
}

func (s *NullTransport) ServerVolumes() []v1.Volume {
	return s.serverVolumes
}

func (s *NullTransport) Direct() bool {
	return s.direct
}
