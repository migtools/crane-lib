package load_balancer

import (
	"context"
	"time"

	"github.com/konveyor/crane-lib/state_transfer/endpoint"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type LoadBalancerEndpoint struct {
	name      string
	namespace string
	hostname  string

	labels map[string]string
	port   int32
}

func (l *LoadBalancerEndpoint) Create(c client.Client) error {
	err := createLoadBalancerService(c, l)
	if err != nil {
		return err
	}

	return nil
}

func (l *LoadBalancerEndpoint) SetHostname(hostname string) {
	l.hostname = hostname
}

func (l *LoadBalancerEndpoint) Hostname() string {
	return l.hostname
}

func (l *LoadBalancerEndpoint) SetPort(port int32) {
	l.port = port
}

func (l *LoadBalancerEndpoint) Port() int32 {
	return l.port
}

func (l *LoadBalancerEndpoint) Name() string {
	return l.name
}

func (l *LoadBalancerEndpoint) Namespace() string {
	return l.namespace
}

func (l *LoadBalancerEndpoint) Labels() map[string]string {
	return l.labels
}

func (l *LoadBalancerEndpoint) Type() endpoint.EndpointType {
	// endpoint type is not valid for this
	return ""
}

func createLoadBalancerService(c client.Client, e endpoint.Endpoint) error {
	serviceSelector := e.Labels()
	serviceSelector["pvc"] = e.Name()

	service := v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      e.Name(),
			Namespace: e.Namespace(),
			Labels:    e.Labels(),
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{
				{
					Name:       e.Name(),
					Protocol:   v1.ProtocolTCP,
					Port:       e.Port(),
					TargetPort: intstr.IntOrString{Type: intstr.Int, IntVal: e.Port()},
				},
			},
			Selector: serviceSelector,
			Type:     v1.ServiceTypeLoadBalancer,
		},
	}

	err := c.Create(context.TODO(), &service, &client.CreateOptions{})
	if err != nil {
		return err
	}

	//FIXME. Seems to take a moment, probably something better to do than wait 5 seconds
	time.Sleep(5 * time.Second)
	err = c.Get(context.TODO(), types.NamespacedName{Name: e.Name(), Namespace: e.Namespace()}, &service)
	if err != nil {
		return err
	}

	e.SetHostname(service.Status.LoadBalancer.Ingress[0].Hostname)
	return nil

}
