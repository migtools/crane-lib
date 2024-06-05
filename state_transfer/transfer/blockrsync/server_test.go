package blockrsync

import (
	"context"
	"fmt"
	"testing"

	routev1 "github.com/openshift/api/route/v1"
	"k8s.io/klog/v2/klogr"

	"github.com/konveyor/crane-lib/state_transfer/endpoint"
	"github.com/konveyor/crane-lib/state_transfer/endpoint/route"
	statetransfermeta "github.com/konveyor/crane-lib/state_transfer/meta"
	"github.com/konveyor/crane-lib/state_transfer/transfer"
	"github.com/konveyor/crane-lib/state_transfer/transport/null"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	corev1 "k8s.io/api/core/v1"
)

const (
	testNamespace = "test-namespace"
	testRouteName = "test-route"
)

func TestCreateServer(t *testing.T) {
	transferOptions := &TransferOptions{
		blockrsyncServerImage: "does.io/serverimage:latest",
	}

	tr, _, destClient := createTransfer(transferOptions, t)
	if err := tr.CreateServer(destClient); err != nil {
		t.Fatalf("CreateServer should not return an error\n %v", err)
	}
	// Do it again, should create an error this time due to already existing resource.
	if err := tr.CreateServer(destClient); err == nil {
		t.Fatalf("CreateServer should return an error")
	}

	serverPod := &corev1.Pod{}
	if err := destClient.Get(context.TODO(), client.ObjectKey{Namespace: testNamespace, Name: blockrsyncServerPodName}, serverPod); err != nil {
		t.Fatalf("unable to get server pod: %v", err)
	}
	if serverPod.Spec.Containers[0].Image != transferOptions.blockrsyncServerImage {
		t.Fatalf("server pod image not set correctly")
	}

	// This will return an error since the pod status is not set in a unit test
	if _, err := tr.IsServerHealthy(destClient); err == nil {
		t.Fatalf("IsServerHealthy should return an error\n")
	}
}

func buildTestClient(objects ...runtime.Object) client.Client {
	s := scheme.Scheme
	schemeInitFuncs := []func(*runtime.Scheme) error{
		corev1.AddToScheme,
		routev1.AddToScheme,
	}
	for _, f := range schemeInitFuncs {
		if err := f(s); err != nil {
			panic(fmt.Errorf("failed to initiate the scheme %w", err))
		}
	}

	return fake.NewClientBuilder().WithScheme(s).WithRuntimeObjects(objects...).Build()
}

func createEndpoint(t *testing.T, name, namespace string, c client.Client) endpoint.Endpoint {
	// create a route for data transfer
	r := route.NewEndpoint(
		types.NamespacedName{
			Namespace: namespace,
			Name:      name,
		}, route.EndpointTypePassthrough, statetransfermeta.Labels, "test.domain")
	e, err := endpoint.Create(r, c)
	if err != nil {
		t.Fatalf("unable to create route endpoint: %v", err)
	}

	route := &routev1.Route{}
	// Mark the route as admitted.
	err = c.Get(context.TODO(), client.ObjectKey{Namespace: namespace, Name: name}, route)
	if err != nil {
		t.Fatalf("unable to get route: %v, %s/%s", err, namespace, name)
	}
	route.Status = routev1.RouteStatus{
		Ingress: []routev1.RouteIngress{
			{
				Conditions: []routev1.RouteIngressCondition{
					{
						Type:   routev1.RouteAdmitted,
						Status: corev1.ConditionTrue,
					},
				},
			},
		},
	}
	err = c.Status().Update(context.TODO(), route)
	if err != nil {
		t.Fatalf("unable to update route status: %v", err)
	}

	ready, err := e.IsHealthy(c)
	if err != nil {
		t.Fatalf("unable to check route health: %v", err)
	}
	if !ready {
		t.Fatalf("route is not ready")
	}
	return r
}

func createTransfer(transferOptions *TransferOptions, t *testing.T) (*BlockrsyncTransfer, client.Client, client.Client) {
	srcClient := buildTestClient()
	destClient := buildTestClient()
	e := createEndpoint(t, testRouteName, testNamespace, destClient)
	if e == nil {
		t.Fatalf("unable to create endpoint")
	}
	transport := null.NewTransport(&testNamespacedNamePair{})
	log := klogr.New()
	pvcList := transfer.PVCPairList{
		&testPVCPair{
			source: &testPVC{
				label: testPVCName,
				pvc:   createPVC(testPVCName, testNamespace, &block),
			},
			dest: &testPVC{
				label: testPVCName,
				pvc:   createPVC(testPVCName, testNamespace, &block),
			},
		},
	}
	tr, err := NewTransfer(transport, e, srcClient, destClient, pvcList, log, transferOptions)
	if err != nil {
		t.Fatalf("NewTransfer should not return an error\n %v", err)
	}
	if tr == nil {
		t.Fatalf("NewTransfer should return a valid transfer")
	}

	return tr.(*BlockrsyncTransfer), srcClient, destClient
}

type testNamespacedNamePair struct {
	src types.NamespacedName
	dst types.NamespacedName
}

func (t *testNamespacedNamePair) Source() types.NamespacedName {
	return t.src
}

func (t *testNamespacedNamePair) Destination() types.NamespacedName {
	return t.dst
}
