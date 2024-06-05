package stunnel

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/types"
)

const (
	testTunnelName = "test-tunnel"
	testRouteName  = "test-route"
)

func TestCreateServerConfig(t *testing.T) {
	client := buildTestClient()
	e := createEndpoint(t, testRouteName, testNamespace, client)
	if e == nil {
		t.Fatalf("unable to create endpoint")
	}
	stunnelTransport := createStunnel(testTunnelName, testNamespace, testRouteName, testNamespace)
	if err := createStunnelServerConfig(client, stunnelTransport, "fs", e); err != nil {
		t.Fatalf("unable to create server config: %v", err)
	}
	cm, err := getServerConfig(client, types.NamespacedName{
		Namespace: testNamespace,
		Name:      testTunnelName,
	}, "fs")
	if err != nil {
		t.Fatalf("unable to get server config: %v", err)
	}
	if cm == nil {
		t.Fatalf("server config not found")
	}
	if len(e.Labels()) != len(cm.Labels) {
		t.Fatalf("server config labels length does not match, on new CM")
	}
	for k, v := range e.Labels() {
		if cm.Labels[k] != v {
			t.Fatalf("server config labels do not match, on new CM")
		}
	}
	if !strings.Contains(cm.Data[stunnelCMKey], fmt.Sprintf("connect = %d", stunnelTransport.ExposedPort())) {
		t.Fatalf("server config does not contain the correct connect port %s", cm.Data[stunnelCMKey])
	}
	if !strings.Contains(cm.Data[stunnelCMKey], fmt.Sprintf("accept = %d", e.Port())) {
		t.Fatalf("server config does not contain the correct accept port %s", cm.Data[stunnelCMKey])
	}
	t.Run("CreateServerConfigUpdate", func(t *testing.T) {
		// Ensure that if the config map already exists, the contents are updated.
		cm.Labels = map[string]string{"test": "label"}
		cm.Data[stunnelCMKey] = "test"
		err = client.Update(context.Background(), cm)
		if err != nil {
			t.Fatalf("unable to update server config map with old data: %v", err)
		}
		if err := createStunnelServerConfig(client, stunnelTransport, "fs", e); err != nil {
			t.Fatalf("unable to create server config: %v", err)
		}
		cm, err := getServerConfig(client, types.NamespacedName{
			Namespace: testNamespace,
			Name:      testTunnelName,
		}, "fs")
		if err != nil {
			t.Fatalf("unable to get server config: %v", err)
		}
		if cm == nil {
			t.Fatalf("server config not found")
		}
		if len(e.Labels()) != len(cm.Labels) {
			t.Fatalf("server config labels do not match")
		}
		for k, v := range e.Labels() {
			if cm.Labels[k] != v {
				t.Fatalf("server config labels do not match")
			}
		}
		if !strings.Contains(cm.Data[stunnelCMKey], fmt.Sprintf("connect = %d", stunnelTransport.ExposedPort())) {
			t.Fatalf("server config does not contain the correct connect port %s", cm.Data[stunnelCMKey])
		}
		if !strings.Contains(cm.Data[stunnelCMKey], fmt.Sprintf("accept = %d", e.Port())) {
			t.Fatalf("server config does not contain the correct accept port %s", cm.Data[stunnelCMKey])
		}
	})

}

func TestCreateServerSecret(t *testing.T) {
	client := buildTestClient()
	e := createEndpoint(t, testRouteName, testNamespace, client)
	if e == nil {
		t.Fatalf("unable to create endpoint")
	}
	stunnelTransport := createStunnel("test-stunnel", testNamespace, testRouteName, testNamespace)
	if err := createStunnelServerSecret(client, stunnelTransport, "fs", e); err != nil {
		t.Fatalf("unable to create server secret: %v", err)
	}
	secret, err := getServerSecret(client, types.NamespacedName{
		Namespace: testNamespace,
		Name:      testTunnelName,
	}, "fs")
	if err != nil {
		t.Fatalf("unable to get server secret: %v", err)
	}
	if secret == nil {
		t.Fatalf("server secret not found")
	}
	if len(e.Labels()) != len(secret.Labels) {
		t.Fatalf("server secret labels length does not match, on new secret")
	}
	for k, v := range e.Labels() {
		if secret.Labels[k] != v {
			t.Fatalf("server secret labels do not match, on new secret")
		}
	}
	if len(secret.Data) != 2 {
		t.Fatalf("server secret does not contain the correct number of keys")
	}
	if _, ok := secret.Data[crtKey]; !ok {
		t.Fatalf("server secret does not contain the correct keys")
	}
	if _, ok := secret.Data[keyKey]; !ok {
		t.Fatalf("server secret does not contain the correct keys")
	}
}

func TestCreateServer(t *testing.T) {
	client := buildTestClient()
	e := createEndpoint(t, testRouteName, testNamespace, client)
	if e == nil {
		t.Fatalf("unable to create endpoint")
	}
	stunnelTransport := createStunnel(testTunnelName, testNamespace, testRouteName, testNamespace)

	if err := stunnelTransport.CreateServer(client, "", e); err != nil {
		t.Fatalf("unable to create server: %v", err)
	}

	containers := stunnelTransport.serverContainers
	if len(containers) != 1 {
		t.Fatalf("Number of server containers is not the expected 1, %d", len(containers))
	}
	volumes := stunnelTransport.serverVolumes
	if len(volumes) != 2 {
		t.Fatalf("Number of server volumes is not the expected 2, %d", len(volumes))
	}
}
