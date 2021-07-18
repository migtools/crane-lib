package load_balancer

import (
	"context"
	"fmt"
	"time"

	"github.com/konveyor/crane-lib/state_transfer/endpoint"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type LoadBalancerEndpoint struct {
	namespacedName types.NamespacedName
	hostname       string

	labels map[string]string
	port   int32
}

func NewEndpoint(namespacedName types.NamespacedName, labels map[string]string) endpoint.Endpoint {
	return &LoadBalancerEndpoint{
		namespacedName: namespacedName,
		labels:         labels,
	}
}

func (l *LoadBalancerEndpoint) Create(c client.Client) error {
	err := l.createLoadBalancerService(c)
	if err != nil {
		return err
	}

	return nil
}

func (l *LoadBalancerEndpoint) Hostname() string {
	return l.hostname
}

func (l *LoadBalancerEndpoint) Port() int32 {
	return l.port
}

func (l *LoadBalancerEndpoint) NamespacedName() types.NamespacedName {
	return l.namespacedName
}

func (l *LoadBalancerEndpoint) Labels() map[string]string {
	return l.labels
}

func (l *LoadBalancerEndpoint) TransportPort() int32 {
	return l.port
}

func (l *LoadBalancerEndpoint) ExposedPort() int32 {
	return l.port
}

func (l *LoadBalancerEndpoint) IsHealthy(c client.Client) (bool, error) {
	service := corev1.Service{}
	err := c.Get(context.TODO(), types.NamespacedName{Name: l.NamespacedName().Name, Namespace: l.NamespacedName().Namespace}, &service)
	if err != nil {
		return false, err
	}

	if service.Status.LoadBalancer.Ingress != nil && len(service.Status.LoadBalancer.Ingress) > 0 {
		// TODO: set the hostname here
		//l.hostname = service.Status.LoadBalancer.Ingress[0].Hostname
		return true, nil
	}
	return false, fmt.Errorf("load balancer sevice status is not in valid state: %s", service.Status.String())
}

func (l *LoadBalancerEndpoint) createLoadBalancerService(c client.Client) error {
	serviceSelector := l.Labels()
	serviceSelector["pvc"] = l.NamespacedName().Name

	service := corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      l.NamespacedName().Name,
			Namespace: l.NamespacedName().Namespace,
			Labels:    l.Labels(),
		},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{
				{
					Name:       l.NamespacedName().Name,
					Protocol:   corev1.ProtocolTCP,
					Port:       l.Port(),
					TargetPort: intstr.IntOrString{Type: intstr.Int, IntVal: l.Port()},
				},
			},
			Selector: serviceSelector,
			Type:     corev1.ServiceTypeLoadBalancer,
		},
	}

	err := c.Create(context.TODO(), &service, &client.CreateOptions{})
	if err != nil {
		return err
	}

	//FIXME. Seems to take a moment, probably something better to do than wait 5 seconds
	time.Sleep(5 * time.Second)
	err = c.Get(context.TODO(), types.NamespacedName{Name: l.NamespacedName().Name, Namespace: l.NamespacedName().Namespace}, &service)
	if err != nil {
		return err
	}

	l.setHostname(service.Status.LoadBalancer.Ingress[0].Hostname)
	return nil

}

func (l *LoadBalancerEndpoint) setPort(port int32) {
	l.port = port
}

func (l *LoadBalancerEndpoint) setHostname(hostname string) {
	l.hostname = hostname
}
