package state_transfer

import (
	"context"

	routev1 "github.com/openshift/api/route/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type RouteEndpoint struct {
	Hostname string
	Port     int32
}

func (r *RouteEndpoint) createEndpointResources(c client.Client, t Transfer) error {
	err := createRouteService(c, r, t)
	if err != nil {
		return err
	}

	err = createRoute(c, r, t)
	if err != nil {
		return err
	}

	return nil
}

func createRouteService(c client.Client, r *RouteEndpoint, t Transfer) error {
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
			Type:     v1.ServiceTypeClusterIP,
		},
	}

	return c.Create(context.TODO(), &service, &client.CreateOptions{})
}

func createRoute(c client.Client, r *RouteEndpoint, t Transfer) error {
	route := routev1.Route{
		ObjectMeta: metav1.ObjectMeta{
			Name:      t.GetPVC().Name,
			Namespace: t.GetPVC().Namespace,
			Labels:    labels,
		},
		Spec: routev1.RouteSpec{
			To: routev1.RouteTargetReference{
				Kind: "Service",
				Name: t.GetPVC().Name,
			},
			Port: &routev1.RoutePort{
				TargetPort: intstr.IntOrString{Type: intstr.Int, IntVal: t.GetTransport().GetTransportPort()},
			},
			TLS: &routev1.TLSConfig{
				Termination: routev1.TLSTerminationPassthrough,
			},
		},
	}

	err := c.Create(context.TODO(), &route, &client.CreateOptions{})
	if err != nil {
		return err
	}

	err = c.Get(context.TODO(), types.NamespacedName{Name: t.GetPVC().Name, Namespace: t.GetPVC().Namespace}, &route)
	if err != nil {
		return err
	}

	r.SetHostname(route.Spec.Host)
	r.SetPort(int32(443))

	return nil
}

func (r *RouteEndpoint) SetHostname(hostname string) {
	r.Hostname = hostname
}

func (r *RouteEndpoint) GetHostname() string {
	return r.Hostname
}

func (r *RouteEndpoint) SetPort(port int32) {
	r.Port = port
}

func (r *RouteEndpoint) GetPort() int32 {
	return r.Port
}
