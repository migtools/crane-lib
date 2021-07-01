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

	v1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type Transport interface {
	CA() *bytes.Buffer
	Crt() *bytes.Buffer
	Key() *bytes.Buffer
	Port() int32
	ClientContainers() []v1.Container
	ClientVolumes() []v1.Volume
	ServerContainers() []v1.Container
	ServerVolumes() []v1.Volume
	Direct() bool
	CreateServer(client.Client, endpoint.Endpoint) error
	CreateClient(client.Client, endpoint.Endpoint) error
	ProxyOptions() *ProxyOptions
	SetProxyOptions(*ProxyOptions)
	NoVerifyCA() bool
	SetNoVerifyCA(bool)
	CAVerifyLevel() string
	SetCAVerifyLevel(string)
}

type ProxyOptions struct {
	URL      string
	Username string
	Password string
}

func CreateServer(t Transport, c client.Client, e endpoint.Endpoint) (Transport, error) {
	err := t.CreateServer(c, e)
	if err != nil {
		return nil, err
	}

	return t, nil
}

func CreateClient(t Transport, c client.Client, e endpoint.Endpoint) (Transport, error) {
	err := t.CreateClient(c, e)
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
