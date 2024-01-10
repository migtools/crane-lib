package blockrsync

import (
	"context"
	"testing"

	"github.com/konveyor/crane-lib/state_transfer/transfer"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func TestCreateClient(t *testing.T) {
	transferOptions := &TransferOptions{
		SourcePodMeta: transfer.ResourceMetadata{
			Labels: map[string]string{},
		},
		blockrsyncClientImage: "does.io/clientimage:latest",
	}

	tr, srcClient, _ := createTransfer(transferOptions, t)
	if err := tr.CreateClient(srcClient); err != nil {
		t.Fatalf("unable to create client: %v", err)
	}

	clientPodList := &corev1.PodList{}
	if err := srcClient.List(context.TODO(), clientPodList, &client.ListOptions{
		Namespace: testNamespace,
		LabelSelector: labels.SelectorFromSet(labels.Set{
			"pvc": "test-pvc",
		}),
	}); err != nil {
		t.Fatalf("unable to get server pod: %v", err)
	}
	if len(clientPodList.Items) != 1 {
		t.Fatalf("client pod not found")
	}
	clientPod := clientPodList.Items[0]
	if clientPod.Spec.Containers[0].Image != transferOptions.blockrsyncClientImage {
		t.Fatalf("client pod image not set correctly")
	}
}
