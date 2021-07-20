package stunnel

import (
	"bytes"
	"fmt"

	"github.com/konveyor/crane-lib/state_transfer/transport"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	corev1 "k8s.io/api/core/v1"
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
	serverContainers []corev1.Container
	serverVolumes    []corev1.Volume
	clientContainers []corev1.Container
	clientVolumes    []corev1.Volume
	direct           bool
}

func NewTransport() transport.Transport {
	return &StunnelTransport{}
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

// GetTransportFromKubeObjects checks if the required configmaps and secretes are created for the transport
//. It populates the fields for the Transport needed for transfer object.
func GetTransportFromKubeObjects(srcClient client.Client, destClient client.Client, obj types.NamespacedName) (transport.Transport, error) {
	_, err := getClientConfig(srcClient, obj)
	switch {
	case errors.IsNotFound(err):
		fmt.Printf("transport: %s Client Config is not created", obj)
		return nil, err
	case err != nil:
		return nil, err
	}

	_, err = getServerConfig(destClient, obj)
	switch {
	case errors.IsNotFound(err):
		fmt.Printf("transport: %s Server Config is not created", obj)
		return nil, err
	case err != nil:
		return nil, err
	}

	clientSecretCreated, err := getClientSecret(srcClient, obj)
	switch {
	case errors.IsNotFound(err):
		fmt.Printf("transport: %s Client secret is not created", obj)
		return nil, err
	case err != nil:
		return nil, err
	}

	_, err = getServerSecret(destClient, obj)
	switch {
	case errors.IsNotFound(err):
		fmt.Printf("transport: %s Server secret is not created", obj)
		return nil, err
	case err != nil:
		return nil, err
	}

	s := &StunnelTransport{}

	key, ok := clientSecretCreated.Data["tls.key"]
	if !ok {
		fmt.Printf("invalid secret for transport %s, tls.key key not found", obj)
		return nil, fmt.Errorf("invalid secret for transport %s, tls.key key not found", obj)
	}

	crt, ok := clientSecretCreated.Data["tls.crt"]
	if !ok {
		fmt.Printf("invalid secret for transport %s, tls.crt key not found", obj)
		return nil, fmt.Errorf("invalid secret for transport %s, tls.crt key not found", obj)
	}

	s.key = bytes.NewBuffer(key)
	s.crt = bytes.NewBuffer(crt)

	setClientContainers(s, obj)
	createClientVolumes(s, obj)
	s.port = stunnelPort
	return s, nil
}
