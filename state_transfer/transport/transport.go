package transport

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"time"

	"github.com/konveyor/crane-lib/state_transfer/endpoint"
	"github.com/konveyor/crane-lib/state_transfer/meta"

	v1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Transport knows how to create an end to end tunnel for a transfer to work on
type Transport interface {
	// NamespacedNamePair returns a source and a destination pair to identify this transport
	NamespacedNamePair() meta.NamespacedNamePair
	//.CA returns CA used by the transport
	CA() *bytes.Buffer
	// Crt returns certificate used by the transport for encryption
	Crt() *bytes.Buffer
	Key() *bytes.Buffer
	// Port returns a port on which the transport listens for connections
	Port() int32
	// ExposedPort returns an exposed port for transfers to use
	ExposedPort() int32
	// ClientContainers returns a list of containers transfers can add to their client Pods
	ClientContainers() []v1.Container
	// ClientVolumes returns a list of volumes transfers can add to their client Pods
	ClientVolumes() []v1.Volume
	// ServerContainers returns a list of containers transfers can add to their server Pods
	ServerContainers() []v1.Container
	// ServerVolumes returns a list of volumes transfers can add to their server Pods
	ServerVolumes() []v1.Volume
	Direct() bool
	CreateServer(client.Client, string, endpoint.Endpoint) error
	CreateClient(client.Client, string, endpoint.Endpoint) error
	Options() *Options
	// Type
	Type() TransportType
}

type Options struct {
	ProxyURL           string
	ProxyUsername      string
	ProxyPassword      string
	NoVerifyCA         bool
	CAVerifyLevel      string
	StunnelClientImage string
	StunnelServerImage string
}

type TransportType string

func CreateServer(t Transport, c client.Client, prefix string, e endpoint.Endpoint) (Transport, error) {
	err := t.CreateServer(c, prefix, e)
	if err != nil {
		return nil, err
	}

	return t, nil
}

func CreateClient(t Transport, c client.Client, prefix string, e endpoint.Endpoint) (Transport, error) {
	err := t.CreateClient(c, prefix, e)
	if err != nil {
		return nil, err
	}

	return t, nil
}

func DestroyServer(t Transport) error {
	return nil
}

func DestroyClient(t Transport) error {
	return nil
}

func GenerateSSLCert() (*bytes.Buffer, *bytes.Buffer, *bytes.Buffer, error) {
	caPrivKey, err := rsa.GenerateKey(rand.Reader, 4096)
	if err != nil {
		return nil, nil, nil, err
	}

	subj := pkix.Name{
		CommonName:         "openshift.io",
		Country:            []string{"US"},
		Province:           []string{"NC"},
		Locality:           []string{"RDU"},
		Organization:       []string{"Migration Engineering"},
		OrganizationalUnit: []string{"Engineering"},
	}

	certTemp := x509.Certificate{
		SerialNumber:          big.NewInt(2020),
		Subject:               subj,
		NotBefore:             time.Now(),
		NotAfter:              time.Now().AddDate(10, 0, 0),
		IsCA:                  true,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth},
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		BasicConstraintsValid: true,
	}

	caBytes, err := x509.CreateCertificate(
		rand.Reader,
		&certTemp,
		&certTemp,
		&caPrivKey.PublicKey,
		caPrivKey,
	)
	if err != nil {
		return nil, nil, nil, err
	}
	crt := new(bytes.Buffer)
	err = pem.Encode(crt, &pem.Block{
		Type:  "CERTIFICATE",
		Bytes: caBytes,
	})
	if err != nil {
		return nil, nil, nil, err
	}
	key := new(bytes.Buffer)
	err = pem.Encode(key, &pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(caPrivKey),
	})
	if err != nil {
		return nil, nil, nil, err
	}

	return crt, crt, key, nil
}
