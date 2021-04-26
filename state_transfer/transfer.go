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
	GetSource() *rest.Config
	SetDestination(*rest.Config)
	GetDestination() *rest.Config
	SetPVC(v1.PersistentVolumeClaim)
	GetPVC() v1.PersistentVolumeClaim
	SetEndpoint(Endpoint)
	GetEndpoint() Endpoint
	SetTransport(Transport)
	GetTransport() Transport
	SetUsername(string)
	GetUsername() string
	SetPassword(string)
	GetPassword() string
	SetTransferPort(int32)
	GetTransferPort() int32
	createTransferServer(client.Client) error
	createTransferServerResources(client.Client) error
	createTransferClient(client.Client) error
	createTransferClientResources(client.Client) error
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
	c, err := client.New(t.GetSource(), client.Options{Scheme: scheme})
	if err != nil {
		return err
	}

	err = t.createTransferServerResources(c)
	if err != nil {
		return err
	}

	transport, err := CreateTransportServer(t.GetTransport(), c, t)
	if err != nil {
		return err
	}
	t.SetTransport(transport)

	err = t.createTransferServer(c)
	if err != nil {
		return err
	}

	endpoint, err := CreateEndpoint(t.GetEndpoint(), c, t)
	if err != nil {
		return err
	}
	t.SetEndpoint(endpoint)

	return nil
}

func DeleteServer(t Transfer) error {
	return nil
}

func CreateClient(t Transfer) error {
	c, err := client.New(t.GetDestination(), client.Options{})
	if err != nil {
		return err
	}

	err = t.createTransferClientResources(c)
	if err != nil {
		return err
	}

	transport, err := CreateTransportClient(t.GetTransport(), c, t)
	if err != nil {
		return err
	}

	t.SetTransport(transport)

	err = t.createTransferClient(c)
	if err != nil {
		return err
	}

	return nil
}

func DeleteClient(t Transfer) error {
	return nil
}
