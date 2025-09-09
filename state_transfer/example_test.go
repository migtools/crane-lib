package state_transfer_test

import (
	"context"
	"fmt"
	"log"
	"testing"
	"time"

	"k8s.io/apimachinery/pkg/types"
	"k8s.io/klog/v2/klogr"

	"github.com/konveyor/crane-lib/state_transfer/endpoint"
	"github.com/konveyor/crane-lib/state_transfer/endpoint/route"
	"github.com/konveyor/crane-lib/state_transfer/meta"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	statetransfermeta "github.com/konveyor/crane-lib/state_transfer/meta"
	"github.com/konveyor/crane-lib/state_transfer/transfer"
	"github.com/konveyor/crane-lib/state_transfer/transfer/rclone"
	"github.com/konveyor/crane-lib/state_transfer/transfer/rsync"
	"github.com/konveyor/crane-lib/state_transfer/transport"
	"github.com/konveyor/crane-lib/state_transfer/transport/stunnel"
	routev1 "github.com/openshift/api/route/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	srcCfg       = &rest.Config{}
	destCfg      = &rest.Config{}
	srcNamespace = "src-namespace"
	srcPVC       = "example-pvc"
)

// This example shows how to wire up the components of the lib to
// transfer data from one PVC to another
func TestExample_basicTransfer(t *testing.T) {
	srcClient := buildTestClient(createPvc(srcPVC, srcNamespace))
	destClient := buildTestClient(createNamespace(srcNamespace))

	// set up the PVC on destination to receive the data
	pvc := &corev1.PersistentVolumeClaim{}
	err := srcClient.Get(context.TODO(), client.ObjectKey{Namespace: srcNamespace, Name: srcPVC}, pvc)
	if err != nil {
		t.Fatalf("unable to get source PVC: %v", err)
	}

	destPVC := pvc.DeepCopy()

	destPVC.ResourceVersion = ""
	destPVC.Spec.VolumeName = ""
	destPVC.Annotations = map[string]string{}
	err = destClient.Create(context.TODO(), destPVC, &client.CreateOptions{})
	if err != nil {
		t.Fatalf("unable to create destination PVC: %v", err)
	}

	pvcList, err := transfer.NewFilesystemPVCPairList(
		transfer.NewPVCPair(pvc, destPVC),
	)
	if err != nil {
		t.Fatalf("invalid pvc list: %v", err)
	}

	// create a route for data transfer
	r := route.NewEndpoint(
		types.NamespacedName{
			Namespace: pvc.Namespace,
			Name:      pvc.Name,
		}, route.EndpointTypePassthrough, statetransfermeta.Labels, "test.domain")
	e, err := endpoint.Create(r, destClient)
	if err != nil {
		t.Fatalf("unable to create route endpoint: %v", err)
	}

	route := &routev1.Route{}
	// Mark the route as admitted.
	err = destClient.Get(context.TODO(), client.ObjectKey{Namespace: pvc.Namespace, Name: pvc.Name}, route)
	if err != nil {
		t.Fatalf("unable to get route: %v, %s/%s", err, pvc.Namespace, pvc.Name)
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
	err = destClient.Update(context.TODO(), route)
	if err != nil {
		t.Fatalf("unable to update route status: %v", err)
	}

	ready, err := e.IsHealthy(destClient)
	if err != nil {
		t.Fatalf("unable to check route health: %v", err)
	}
	if !ready {
		t.Fatalf("route is not ready")
	}

	// create an stunnel transport to carry the data over the route
	s := stunnel.NewTransport(statetransfermeta.NewNamespacedPair(
		types.NamespacedName{
			Name: pvc.Name, Namespace: pvc.Namespace},
		types.NamespacedName{
			Name: destPVC.Name, Namespace: destPVC.Namespace},
	), &transport.Options{})
	_, err = transport.CreateServer(s, destClient, "fs", e)
	if err != nil {
		t.Fatalf("error creating stunnel server: %v", err)
	}

	s, err = transport.CreateClient(s, srcClient, "fs", e)
	if err != nil {
		t.Fatalf("error creating stunnel client: %v", err)
	}

	// Create Rclone Transfer Pod
	tr, err := rclone.NewTransfer(s, r, srcClient, destClient, pvcList)
	if err != nil {
		t.Fatalf("errror creating rclone transfer: %v", err)
	}

	err = transfer.CreateServer(tr)
	if err != nil {
		t.Fatalf("error creating rclone server: %v", err)
	}

	// Rsync Example
	rsyncTransferOptions := rsync.GetRsyncCommandDefaultOptions()
	customTransferOptions := []rsync.TransferOption{
		rsync.Username("username"),
		rsync.Password("password"),
	}
	rsyncTransferOptions = append(rsyncTransferOptions, customTransferOptions...)

	rsyncTransfer, err := rsync.NewTransfer(s, r, srcClient, destClient, pvcList, klogr.New(), rsyncTransferOptions...)
	if err != nil {
		log.Fatal(err, "error creating rsync transfer")
	} else {
		log.Printf("rsync transfer created for pvc %s\n", rsyncTransfer.PVCs()[0].Source().Claim().Name)
	}

	// Create Rclone Client Pod
	err = transfer.CreateClient(tr)
	if err != nil {
		log.Fatal(err, "error creating rclone client")
	}

	// TODO: check if the client is completed
}

// This example shows how to get the endpoint and transport objects for creating the transfer after endpoint and
// transport are created in previous reconcile attempt
func Example_getFromCreatedObjects() {
	srcClient, err := client.New(srcCfg, client.Options{Scheme: runtime.NewScheme()})
	if err != nil {
		log.Fatal(err, "unable to create source client")
	}

	destClient, err := client.New(destCfg, client.Options{Scheme: runtime.NewScheme()})
	if err != nil {
		log.Fatal(err, "unable to create destination client")
	}

	// set up the PVC on destination to receive the data
	pvc := &corev1.PersistentVolumeClaim{}
	err = srcClient.Get(context.TODO(), client.ObjectKey{Namespace: srcNamespace, Name: srcPVC}, pvc)
	if err != nil {
		log.Fatal(err, "unable to get source PVC")
	}

	destPVC := pvc.DeepCopy()

	pvcList, err := transfer.NewFilesystemPVCPairList(
		transfer.NewPVCPair(pvc, destPVC),
	)
	if err != nil {
		log.Fatal(err, "invalid pvc list")
	}

	e, err := route.GetEndpointFromKubeObjects(destClient, types.NamespacedName{Namespace: srcNamespace, Name: srcPVC})
	if err != nil {
		log.Fatal(err, "error getting route endpoint")
	}

	nnPair := meta.NewNamespacedPair(
		types.NamespacedName{Namespace: srcNamespace, Name: srcPVC},
		types.NamespacedName{Namespace: srcNamespace, Name: srcPVC},
	)
	s, err := stunnel.GetTransportFromKubeObjects(srcClient, destClient, "fs", nnPair, e, &transport.Options{})
	if err != nil {
		log.Fatal(err, "error getting stunnel transport")
	}

	pvcList, err = transfer.NewFilesystemPVCPairList(
		transfer.NewPVCPair(pvc, nil),
	)
	if err != nil {
		log.Fatal(err, "invalid pvc list")
	}

	// Create Rclone Transfer Pod
	t, err := rclone.NewTransfer(s, e, srcClient, destClient, pvcList)
	if err != nil {
		log.Fatal(err, "errror creating rclone transfer")
	}
	err = transfer.CreateServer(t)
	if err != nil {
		log.Fatal(err, "error creating rclone server")
	}

	// check if the server is healthy before creating the client
	_ = wait.PollUntil(time.Second*5, func() (done bool, err error) {
		isHealthy, err := t.IsServerHealthy(destClient)
		if err != nil {
			log.Println(err, "unable to check server health, retrying...")
			return false, nil
		}
		return isHealthy, nil
	}, make(<-chan struct{}))

	// Create Rclone Client Pod
	err = transfer.CreateClient(t)
	if err != nil {
		log.Fatal(err, "error creating rclone client")
	}

	// TODO: check if the client is completed
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

func createPvc(name, namespace string) *corev1.PersistentVolumeClaim {
	return &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
	}
}

func createNamespace(name string) *corev1.Namespace {
	return &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
	}
}
