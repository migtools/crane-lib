package state_transfer

import (
	routev1 "github.com/openshift/api/route/v1"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var labels = map[string]string{"app": "crane2"}

type Transfer interface {
	SetSource(*rest.Config)
	Source() *rest.Config
	SetDestination(*rest.Config)
	Destination() *rest.Config
	SetPVC(v1.PersistentVolumeClaim)
	PVC() v1.PersistentVolumeClaim
	SetEndpoint(Endpoint)
	Endpoint() Endpoint
	SetTransport(Transport)
	Transport() Transport
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

func connectionHostname(t Transfer) string {
	if t.Transport().Direct() {
		return t.Endpoint().Hostname()
	}
	return "localhost"
}

func connectionPort(t Transfer) int32 {
	if t.Transport().Direct() {
		return t.Endpoint().Port()
	}
	return t.Transport().Port()
}
