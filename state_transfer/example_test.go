package state_transfer_test

import (
	"context"
	"log"
	"time"

	"k8s.io/apimachinery/pkg/types"

	"github.com/konveyor/crane-lib/state_transfer"
	"github.com/konveyor/crane-lib/state_transfer/endpoint"
	"github.com/konveyor/crane-lib/state_transfer/endpoint/route"
	"github.com/konveyor/crane-lib/state_transfer/labels"
	"github.com/konveyor/crane-lib/state_transfer/transfer"
	"github.com/konveyor/crane-lib/state_transfer/transfer/rclone"
	"github.com/konveyor/crane-lib/state_transfer/transfer/rsync"
	"github.com/konveyor/crane-lib/state_transfer/transport"
	"github.com/konveyor/crane-lib/state_transfer/transport/stunnel"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
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
func Example_basicTransfer() {
	srcClient, err := client.New(srcCfg, client.Options{Scheme: runtime.NewScheme()})
	if err != nil {
		log.Fatal(err, "unable to create source client")
	}

	destClient, err := client.New(destCfg, client.Options{Scheme: runtime.NewScheme()})
	if err != nil {
		log.Fatal(err, "unable to create destination client")
	}

	// quiesce the applications if needed on the source side
	err = state_transfer.QuiesceApplications(srcCfg, srcNamespace)
	if err != nil {
		log.Fatal(err, "unable to quiesce application on source cluster")
	}

	// set up the PVC on destination to receive the data
	pvc := &corev1.PersistentVolumeClaim{}
	err = srcClient.Get(context.TODO(), client.ObjectKey{Namespace: srcNamespace, Name: srcPVC}, pvc)
	if err != nil {
		log.Fatal(err, "unable to get source PVC")
	}

	destPVC := pvc.DeepCopy()

	destPVC.ResourceVersion = ""
	destPVC.Spec.VolumeName = ""
	pvc.Annotations = map[string]string{}
	err = destClient.Create(context.TODO(), destPVC, &client.CreateOptions{})
	if err != nil {
		log.Fatal(err, "unable to create destination PVC")
	}

	pvcList, err := transfer.NewPVCPairList(
		transfer.NewPVCPair(pvc, destPVC),
	)
	if err != nil {
		log.Fatal(err, "invalid pvc list")
	}

	endpointPort := int32(2222)
	// create a route for data transfer
	r := route.NewEndpoint(
		types.NamespacedName{
			Namespace: pvc.Namespace,
			Name:      pvc.Name,
		}, endpointPort, route.EndpointTypePassthrough, labels.Labels)
	e, err := endpoint.Create(r, destClient)
	if err != nil {
		log.Fatal(err, "unable to create route endpoint")
	}

	_ = wait.PollUntil(time.Second*5, func() (done bool, err error) {
		ready, err := e.IsHealthy(destClient)
		if err != nil {
			log.Println(err, "unable to check route health, retrying...")
			return false, nil
		}
		return ready, nil
	}, make(<-chan struct{}))

	// create an stunnel transport to carry the data over the route
	s := stunnel.NewTransport()
	_, err = transport.CreateServer(s, destClient, e)
	if err != nil {
		log.Fatal(err, "error creating stunnel server")
	}

	_, err = transport.CreateClient(s, srcClient, e)
	if err != nil {
		log.Fatal(err, "error creating stunnel client")
	}

	// Create Rclone Transfer Pod
	t, err := rclone.NewTransfer(s, r, srcCfg, destCfg, pvcList)
	if err != nil {
		log.Fatal(err, "errror creating rclone transfer")
	}

	err = transfer.CreateServer(t)
	if err != nil {
		log.Fatal(err, "error creating rclone server")
	}

	// Rsync Example
	rsyncTransferOptions := []rsync.TransferOption{
		rsync.StandardProgress(true),
		rsync.ArchiveFiles(true),
		rsync.WithSourcePodLabels(map[string]string{}),
		rsync.WithDestinationPodLabels(map[string]string{}),
	}

	rsyncTransfer, err := rsync.NewTransfer(s, r, srcCfg, destCfg, pvcList, rsyncTransferOptions...)
	if err != nil {
		log.Fatal(err, "error creating rsync transfer")
	} else {
		log.Printf("rsync transfer created for user %s\n", rsyncTransfer.Username())
	}

	// Create Rclone Client Pod
	err = transfer.CreateClient(t)
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

	pvcList, err := transfer.NewPVCPairList(
		transfer.NewPVCPair(pvc, destPVC),
	)
	if err != nil {
		log.Fatal(err, "invalid pvc list")
	}

	e, err := route.GetEndpointFromKubeObjects(destClient, types.NamespacedName{Namespace: srcNamespace, Name: srcPVC})
	if err != nil {
		log.Fatal(err, "error getting route endpoint")
	}

	s, err := stunnel.GetTransportFromKubeObjects(srcClient, destClient, types.NamespacedName{Namespace: srcNamespace, Name: srcPVC})
	if err != nil {
		log.Fatal(err, "error getting stunnel transport")
	}

	// Create Rclone Transfer Pod
	t, err := rclone.NewTransfer(s, e, srcCfg, destCfg, pvcList)
	if err != nil {
		log.Fatal(err, "errror creating rclone transfer")
	}
	err = transfer.CreateServer(t)
	if err != nil {
		log.Fatal(err, "error creating rclone server")
	}

	// Create Rclone Client Pod
	err = transfer.CreateClient(t)
	if err != nil {
		log.Fatal(err, "error creating rclone client")
	}

	// TODO: check if the client is completed
}
