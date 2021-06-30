package transfer

import (
	"github.com/konveyor/crane-lib/state_transfer/endpoint"
	"github.com/konveyor/crane-lib/state_transfer/transport"
	routev1 "github.com/openshift/api/route/v1"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type Transfer interface {
	SetSource(*rest.Config)
	Source() *rest.Config
	SetDestination(*rest.Config)
	Destination() *rest.Config
	SetPVC(v1.PersistentVolumeClaim)
	PVC() v1.PersistentVolumeClaim
	SetEndpoint(endpoint.Endpoint)
	Endpoint() endpoint.Endpoint
	SetTransport(transport.Transport)
	Transport() transport.Transport
	SetUsername(string)
	Username() string
	SetPassword(string)
	Password() string
	SetPort(int32)
	Port() int32
	CreateServer(client.Client) error
	CreateClient(client.Client) error
}

func CreateServer(t Transfer) error {
	scheme := runtime.NewScheme()
	if err := routev1.AddToScheme(scheme); err != nil {
		return err
	}
	if err := appsv1.AddToScheme(scheme); err != nil {
		return err
	}
	if err := v1.AddToScheme(scheme); err != nil {
		return err
	}
	c, err := client.New(t.Source(), client.Options{Scheme: scheme})
	if err != nil {
		return err
	}

	err = t.CreateServer(c)
	if err != nil {
		return err
	}

	return nil
}

func DeleteServer(t Transfer) error {
	return nil
}

func CreateClient(t Transfer) error {
	c, err := client.New(t.Destination(), client.Options{})
	if err != nil {
		return err
	}

	err = t.CreateClient(c)
	if err != nil {
		return err
	}

	return nil
}

func DeleteClient(t Transfer) error {
	return nil
}

func ConnectionHostname(t Transfer) string {
	if t.Transport().Direct() {
		return t.Endpoint().Hostname()
	}
	return "localhost"
}

func ConnectionPort(t Transfer) int32 {
	if t.Transport().Direct() {
		return t.Endpoint().Port()
	}
	return t.Transport().Port()
}
