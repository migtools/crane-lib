package stunnel

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/konveyor/crane-lib/state_transfer/endpoint"
	"github.com/konveyor/crane-lib/state_transfer/endpoint/route"
	"github.com/konveyor/crane-lib/state_transfer/transport"
	routev1 "github.com/openshift/api/route/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	statetransfermeta "github.com/konveyor/crane-lib/state_transfer/meta"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

const (
	testNamespace = "test-namespace"
	stunnelCMKey  = "stunnel.conf"
	crtKey        = "tls.crt"
	keyKey        = "tls.key"
)

func TestCreateClientConfig(t *testing.T) {
	client := buildTestClient()
	e := createEndpoint(t, testRouteName, testNamespace, client)
	if e == nil {
		t.Fatalf("unable to create endpoint")
	}
	stunnelTransport := createStunnel(testTunnelName, testNamespace, testRouteName, testNamespace)
	if err := createClientConfig(client, stunnelTransport, "fs", e); err != nil {
		t.Fatalf("unable to create client config: %v", err)
	}
	cm, err := getClientConfig(client, types.NamespacedName{
		Namespace: testNamespace,
		Name:      "test-tunnel",
	}, "fs")
	if err != nil {
		t.Fatalf("unable to get client config: %v", err)
	}
	if cm == nil {
		t.Fatalf("client config not found")
	}
	if len(e.Labels()) != len(cm.Labels) {
		t.Fatalf("client config labels length does not match, on new CM")
	}
	for k, v := range e.Labels() {
		if cm.Labels[k] != v {
			t.Fatalf("client config labels do not match, on new CM")
		}
	}
	if !strings.Contains(cm.Data[stunnelCMKey], "test-route-test-namespace.test.domain:443") {
		t.Fatalf("client config does not contain the correct route")
	}

	t.Run("CreateClientConfigUpdate", func(t *testing.T) {
		// Ensure that if the config map already exists, the contents are updated.
		cm.Labels = map[string]string{"test": "label"}
		cm.Data[stunnelCMKey] = "test"
		err = client.Update(context.Background(), cm)
		if err != nil {
			t.Fatalf("unable to update client config map with old data: %v", err)
		}
		stunnelTransport.Options().CAVerifyLevel = "5"
		stunnelTransport.Options().NoVerifyCA = true

		if err := createClientConfig(client, stunnelTransport, "fs", e); err != nil {
			t.Fatalf("unable to create client config: %v", err)
		}
		cm, err := getClientConfig(client, types.NamespacedName{
			Namespace: testNamespace,
			Name:      "test-tunnel",
		}, "fs")
		if err != nil {
			t.Fatalf("unable to get client config: %v", err)
		}
		if cm == nil {
			t.Fatalf("client config not found")
		}
		if len(e.Labels()) != len(cm.Labels) {
			t.Fatalf("client config labels do not match")
		}
		for k, v := range e.Labels() {
			if cm.Labels[k] != v {
				t.Fatalf("client config labels do not match")
			}
		}
		if !strings.Contains(cm.Data[stunnelCMKey], "test-route-test-namespace.test.domain:443") {
			t.Fatalf("client config does not contain the correct route")
		}
		if !strings.Contains(cm.Data[stunnelCMKey], "verify = 5") {
			t.Fatalf("client config does not contain the correct caVerifyLevel %s", cm.Data[stunnelCMKey])
		}
	})
}

func TestCreateClientSecret(t *testing.T) {
	client := buildTestClient()
	e := createEndpoint(t, testRouteName, testNamespace, client)
	if e == nil {
		t.Fatalf("unable to create endpoint")
	}
	stunnelTransport := createStunnel(testTunnelName, testNamespace, testRouteName, testNamespace)

	if err := createClientSecret(client, stunnelTransport, "fs", e); err != nil {
		t.Fatalf("unable to create client secret: %v", err)
	}
	secret, err := getClientSecret(client, types.NamespacedName{
		Namespace: testNamespace,
	}, "fs")
	if err != nil {
		t.Fatalf("unable to get client secret: %v", err)
	}
	if secret == nil {
		t.Fatalf("client secret not found")
	}
	if len(e.Labels()) != len(secret.Labels) {
		t.Fatalf("client secret labels length does not match, on new secret")
	}
	for k, v := range e.Labels() {
		if secret.Labels[k] != v {
			t.Fatalf("client secret labels do not match, on new secret")
		}
	}
	if len(secret.Data) != 2 {
		t.Fatalf("client secret does not contain the correct number of keys")
	}
	if _, ok := secret.Data[crtKey]; !ok {
		t.Fatalf("client secret does not contain the correct keys")
	}
	if _, ok := secret.Data[keyKey]; !ok {
		t.Fatalf("client secret does not contain the correct keys")
	}

}

func TestCreateClient(t *testing.T) {
	client := buildTestClient()
	e := createEndpoint(t, testRouteName, testNamespace, client)
	if e == nil {
		t.Fatalf("unable to create endpoint")
	}
	stunnelTransport := createStunnel(testTunnelName, testNamespace, testRouteName, testNamespace)
	if err := stunnelTransport.CreateClient(client, "", e); err != nil {
		t.Fatalf("unable to create client: %v", err)
	}

	containers := stunnelTransport.clientContainers
	if len(containers) != 1 {
		t.Fatalf("Number of client containers is not the expected 1, %d", len(containers))
	}
	volumes := stunnelTransport.clientVolumes
	if len(volumes) != 2 {
		t.Fatalf("Number of client volumes is not the expected 2, %d", len(volumes))
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

	// Add the test namespace
	namespace := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: testNamespace,
		},
	}
	allObjects := append([]runtime.Object{namespace}, objects...)

	return fake.NewClientBuilder().WithScheme(s).WithRuntimeObjects(allObjects...).Build()
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
	err = c.Update(context.TODO(), route)
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

func createStunnel(name, namespace, destName, destNamespace string) *StunnelTransport {
	// create an stunnel transport to carry the data over the route
	s := NewTransport(statetransfermeta.NewNamespacedPair(
		types.NamespacedName{
			Name: name, Namespace: namespace},
		types.NamespacedName{
			Name: destName, Namespace: destNamespace},
	), &transport.Options{})

	crt, _, key, err := transport.GenerateSSLCert()
	if err != nil {
		return nil
	}
	s.(*StunnelTransport).crt = crt
	s.(*StunnelTransport).key = key

	return s.(*StunnelTransport) // Type assertion to convert s to *StunnelTransport
}
