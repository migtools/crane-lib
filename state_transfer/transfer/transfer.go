package transfer

import (
	"context"
	"fmt"

	"github.com/konveyor/crane-lib/state_transfer/endpoint"
	"github.com/konveyor/crane-lib/state_transfer/transport"
	routev1 "github.com/openshift/api/route/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/runtime"
	errorsutil "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Transfer knows how to transfer PV data from a source to a destination
type Transfer interface {
	// Source returns a source client
	Source() *rest.Config
	// Destination returns a destination client
	Destination() *rest.Config
	// Endpoint returns the endpoint used by the transfer
	Endpoint() endpoint.Endpoint
	// Transport returns the transport used by the transfer
	Transport() transport.Transport
	// CreateServer creates a transfer server either on source or the destination
	CreateServer(client.Client) error
	// CreateClient creates a transfer client either on source or the destination
	CreateClient(client.Client) error
	IsServerHealthy(c client.Client) (bool, error)
	// PVCs returns the list of PVCs the transfer will migrate
	PVCs() PVCPairList
}

func CreateServer(t Transfer) error {
	scheme := runtime.NewScheme()
	if err := routev1.AddToScheme(scheme); err != nil {
		return err
	}
	if err := appsv1.AddToScheme(scheme); err != nil {
		return err
	}
	if err := corev1.AddToScheme(scheme); err != nil {
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
		return t.Endpoint().ExposedPort()
	}
	return t.Transport().Port()
}

// IsPodHealthy is a utility function that can be used by various
// implementations to check if the server pod deployed is healthy
func IsPodHealthy(c client.Client, pod client.ObjectKey) (bool, error) {
	p := &corev1.Pod{}

	err := c.Get(context.Background(), pod, p)
	if err != nil {
		return false, err
	}

	return areContainersReady(p)
}

func areContainersReady(pod *corev1.Pod) (bool, error) {
	//TODO: We should consider delegating some of this to either the endpoint or transfer as well, if they are providing containers.
	for _, containerStatus := range pod.Status.ContainerStatuses {
		if !containerStatus.Ready {
			return false, fmt.Errorf("container %s in pod %s is not ready", containerStatus.Name, client.ObjectKey{Namespace: pod.Namespace, Name: pod.Name})
		}
	}
	return true, nil
}

// AreFilteredPodsHealthy is a utility function that can be used by various
// implementations to check if the server pods deployed with some label selectors
// are healthy. If atleast 1 replica will be healthy the function will return true
func AreFilteredPodsHealthy(c client.Client, namespace string, labels fields.Set) (bool, error) {
	pList := &corev1.PodList{}

	err := c.List(context.Background(), pList, client.InNamespace(namespace), client.MatchingFields(labels))
	if err != nil {
		return false, err
	}

	errs := []error{}

	for _, p := range pList.Items {
		podReady, err := areContainersReady(&p)
		if err != nil {
			errs = append(errs, err)
		}
		if podReady {
			return true, nil
		}
	}

	return false, errorsutil.NewAggregate(errs)
}
