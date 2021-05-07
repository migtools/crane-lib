package state_transfer

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"time"

	v1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type Transport interface {
	SetCA(*bytes.Buffer)
	CA() *bytes.Buffer
	SetCrt(*bytes.Buffer)
	Crt() *bytes.Buffer
	SetKey(*bytes.Buffer)
	Key() *bytes.Buffer
	SetPort(int32)
	Port() int32
	SetClientContainers([]v1.Container)
	ClientContainers() []v1.Container
	SetClientVolumes([]v1.Volume)
	ClientVolumes() []v1.Volume
	SetServerContainers([]v1.Container)
	ServerContainers() []v1.Container
	SetServerVolumes([]v1.Volume)
	ServerVolumes() []v1.Volume
	Direct() bool
	createTransportServerResources(client.Client, Transfer) error
	createTransportClientResources(client.Client, Transfer) error
}

func CreateTransportServer(t Transport, c client.Client, transfer Transfer) (Transport, error) {
	err := t.createTransportServerResources(c, transfer)
	if err != nil {
		return nil, err
	}

	return t, nil
}

func CreateTransportClient(t Transport, c client.Client, transfer Transfer) (Transport, error) {
	err := t.createTransportClientResources(c, transfer)
	if err != nil {
		return nil, err
	}

	return t, nil
}

func DestroyTransportServer(t Transport) error {
	return nil
}

func DestroyTransportClient(t Transport) error {
	return nil
}

func generateSSLCert() (*bytes.Buffer, *bytes.Buffer, *bytes.Buffer, error) {
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
