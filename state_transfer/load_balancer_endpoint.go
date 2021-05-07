package state_transfer

import (
	"context"
	"time"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type LoadBalancerEndpoint struct {
	hostname string
	port     int32
}

func (r *LoadBalancerEndpoint) createEndpointResources(c client.Client, t Transfer) error {
	err := createLoadBalancerService(c, r, t)
	if err != nil {
		return err
	}

	return nil
}

func createLoadBalancerService(c client.Client, r *LoadBalancerEndpoint, t Transfer) error {
	serviceSelector := labels
	serviceSelector["pvc"] = t.PVC().Name

	service := v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      t.PVC().Name,
			Namespace: t.PVC().Namespace,
			Labels:    labels,
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{
				{
					Name:       t.PVC().Name,
					Protocol:   v1.ProtocolTCP,
					Port:       t.Transport().Port(),
					TargetPort: intstr.IntOrString{Type: intstr.Int, IntVal: t.Transport().Port()},
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
	err = c.Get(context.TODO(), types.NamespacedName{Name: t.PVC().Name, Namespace: t.PVC().Namespace}, &service)
	if err != nil {
		return err
	}

	r.SetHostname(service.Status.LoadBalancer.Ingress[0].Hostname)
	r.SetPort(t.Transport().Port())
	return nil

}

func (r *LoadBalancerEndpoint) SetHostname(hostname string) {
	r.hostname = hostname
}

func (r *LoadBalancerEndpoint) Hostname() string {
	return r.hostname
}

func (r *LoadBalancerEndpoint) SetPort(port int32) {
	r.port = port
}

func (r *LoadBalancerEndpoint) Port() int32 {
	return r.port
}
