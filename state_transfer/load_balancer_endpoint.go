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
	Hostname string
	Port     int32
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
	serviceSelector["pvc"] = t.GetPVC().Name

	service := v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      t.GetPVC().Name,
			Namespace: t.GetPVC().Namespace,
			Labels:    labels,
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{
				{
					Name:       t.GetPVC().Name,
					Protocol:   v1.ProtocolTCP,
					Port:       t.GetTransport().GetTransportPort(),
					TargetPort: intstr.IntOrString{Type: intstr.Int, IntVal: t.GetTransport().GetTransportPort()},
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
	err = c.Get(context.TODO(), types.NamespacedName{Name: t.GetPVC().Name, Namespace: t.GetPVC().Namespace}, &service)
	if err != nil {
		return err
	}

	r.SetHostname(service.Status.LoadBalancer.Ingress[0].Hostname)
	r.SetPort(t.GetTransport().GetTransportPort())
	return nil

}

func (r *LoadBalancerEndpoint) SetHostname(hostname string) {
	r.Hostname = hostname
}

func (r *LoadBalancerEndpoint) GetHostname() string {
	return r.Hostname
}

func (r *LoadBalancerEndpoint) SetPort(port int32) {
	r.Port = port
}

func (r *LoadBalancerEndpoint) GetPort() int32 {
	return r.Port
}
