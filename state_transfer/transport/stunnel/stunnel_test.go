package stunnel

import (
	"testing"

	"github.com/konveyor/crane-lib/state_transfer/transport"
	"k8s.io/apimachinery/pkg/types"
)

const (
	sourceName      = "source"
	sourceNamespace = "source-namespace"
	destName        = "dest"
	destNamespace   = "dest-namespace"

	clientImage = "custom-client-image"
	serverImage = "custom-server-image"
)

func TestGetTransportFromKubeObjects(t *testing.T) {
	srcClient := buildTestClient()
	destClient := buildTestClient()

	e := createEndpoint(t, testRouteName, testNamespace, destClient)
	if e == nil {
		t.Fatalf("unable to create endpoint")
	}
	nnPair := &testNamespacedPair{
		src:  types.NamespacedName{Name: sourceName, Namespace: sourceNamespace},
		dest: types.NamespacedName{Name: destName, Namespace: destNamespace},
	}

	stunnelTransport := createStunnel(sourceName, sourceNamespace, destName, destNamespace)

	t.Run("GetTransportFromKubeObjectsNoClientConfig", func(t *testing.T) {
		_, err := GetTransportFromKubeObjects(srcClient, destClient, "fs", nnPair, e, nil)
		if err == nil {
			t.Fatalf("No client config set, should get error")
		}
	})
	// Create client and server config maps
	if err := createClientConfig(srcClient, stunnelTransport, "fs", e); err != nil {
		t.Fatalf("unable to create client config: %v", err)
	}
	t.Run("GetTransportFromKubeObjectsNoServerConfig", func(t *testing.T) {
		_, err := GetTransportFromKubeObjects(srcClient, destClient, "fs", nnPair, e, nil)
		if err == nil {
			t.Fatalf("No server config set, should get error")
		}
	})
	if err := createStunnelServerConfig(destClient, stunnelTransport, "fs", e); err != nil {
		t.Fatalf("unable to create server config: %v", err)
	}
	t.Run("GetTransportFromKubeObjectsNoClientSecret", func(t *testing.T) {
		_, err := GetTransportFromKubeObjects(srcClient, destClient, "fs", nnPair, e, nil)
		if err == nil {
			t.Fatalf("No client secret set, should get error")
		}
	})
	// Create client and server secrets
	if err := createClientSecret(srcClient, stunnelTransport, "fs", e); err != nil {
		t.Fatalf("unable to create client secret: %v", err)
	}
	t.Run("GetTransportFromKubeObjectsNoServerSecret", func(t *testing.T) {
		_, err := GetTransportFromKubeObjects(srcClient, destClient, "fs", nnPair, e, nil)
		if err == nil {
			t.Fatalf("No server secret set, should get error")
		}
	})
	if err := createStunnelServerSecret(destClient, stunnelTransport, "fs", e); err != nil {
		t.Fatalf("unable to create server secret: %v", err)
	}
	tr, err := GetTransportFromKubeObjects(srcClient, destClient, "fs", nnPair, e, nil)
	if err != nil {
		t.Fatalf("unable to get transport: %v", err)
	}
	if tr, ok := tr.(*StunnelTransport); ok {
		verifyDefaultTransport(tr, defaultStunnelImage, defaultStunnelImage, t)
	} else {
		t.Fatalf("unable to convert transport to *StunnelTransport")
	}

	t.Run("GetTransportFromKubeObjectsWithCustomImages", func(t *testing.T) {
		options := &transport.Options{
			StunnelClientImage: clientImage,
			StunnelServerImage: serverImage,
		}
		tr, err := GetTransportFromKubeObjects(srcClient, destClient, "fs", nnPair, e, options)
		if err != nil {
			t.Fatalf("unable to get transport: %v", err)
		}
		if tr, ok := tr.(*StunnelTransport); ok {
			verifyDefaultTransport(tr, clientImage, serverImage, t)
		} else {
			t.Fatalf("unable to convert transport to *StunnelTransport")
		}
	})
}

func verifyDefaultTransport(tr *StunnelTransport, clientImage, serverImage string, t *testing.T) {
	if tr == nil {
		t.Fatalf("transport is nil")
	}
	if tr.CA() != nil {
		t.Fatalf("CA is not nil")
	}
	if tr.Crt() == nil {
		t.Fatalf("Crt is nil")
	}
	if tr.Key() == nil {
		t.Fatalf("Key is nil")
	}
	if tr.ExposedPort() != int32(2222) {
		t.Fatalf("ExposedPort is not 2222")
	}
	if tr.Port() != int32(6443) {
		t.Fatalf("Port is not 6443, %d", tr.Port())
	}
	if len(tr.ClientContainers()) != 1 {
		t.Fatalf("Number of client containers is not the expected 1, %d", len(tr.ClientContainers()))
	}
	if len(tr.ServerContainers()) != 1 {
		t.Fatalf("Number of server containers is not the expected 1, %d", len(tr.ServerContainers()))
	}
	if len(tr.ClientVolumes()) != 2 {
		t.Fatalf("Number of client volumes is not the expected 2, %d", len(tr.ClientVolumes()))
	}
	if len(tr.ServerVolumes()) != 2 {
		t.Fatalf("Number of server volumes is not the expected 2, %d", len(tr.ServerVolumes()))
	}
	if tr.Direct() {
		t.Fatalf("Direct is true")
	}
	if tr.Type() != TransportTypeStunnel {
		t.Fatalf("Type is not TransportTypeStunnel")
	}
	if tr.NamespacedNamePair().Source().Name != sourceName {
		t.Fatalf("Source name is not %s", sourceName)
	}
	if tr.NamespacedNamePair().Source().Namespace != sourceNamespace {
		t.Fatalf("Source namespace is not %s", sourceNamespace)
	}
	if tr.NamespacedNamePair().Destination().Name != destName {
		t.Fatalf("Destination name is not %s", destName)
	}
	if tr.NamespacedNamePair().Destination().Namespace != destNamespace {
		t.Fatalf("Destination namespace is not %s", destNamespace)
	}
	if tr.getStunnelServerImage() != serverImage {
		t.Fatalf("Server image is not %s", serverImage)
	}
	if tr.getStunnelClientImage() != clientImage {
		t.Fatalf("Client image is not %s", clientImage)
	}
}

type testNamespacedPair struct {
	src  types.NamespacedName
	dest types.NamespacedName
}

func (t *testNamespacedPair) Source() types.NamespacedName {
	return t.src
}

func (t *testNamespacedPair) Destination() types.NamespacedName {
	return t.dest
}
