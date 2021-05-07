package state_transfer

import (
	"context"
	"reflect"

	routev1 "github.com/openshift/api/route/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type RouteEndpoint struct {
	hostname string
	port     int32
}

func (r *RouteEndpoint) Create(c client.Client, t Transfer) error {
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

func createRouteResources(c client.Client, r *RouteEndpoint, t Transfer) error {
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
			Type:     v1.ServiceTypeClusterIP,
		},
	}

	return c.Create(context.TODO(), &service, &client.CreateOptions{})
}

func createRoute(c client.Client, r *RouteEndpoint, t Transfer) error {
	termination := &routev1.TLSConfig{}
	switch reflect.TypeOf(t.Transport()) {
	case reflect.TypeOf(&NullTransport{}):
		termination = &routev1.TLSConfig{
			Termination:                   routev1.TLSTerminationEdge,
			InsecureEdgeTerminationPolicy: "Allow",
		}
		r.SetPort(int32(80))
	default:
		termination = &routev1.TLSConfig{
			Termination: routev1.TLSTerminationPassthrough,
		}
		r.SetPort(int32(443))
	}

	route := routev1.Route{
		ObjectMeta: metav1.ObjectMeta{
			Name:      t.PVC().Name,
			Namespace: t.PVC().Namespace,
			Labels:    labels,
		},
		Spec: routev1.RouteSpec{
			To: routev1.RouteTargetReference{
				Kind: "Service",
				Name: t.PVC().Name,
			},
			Port: &routev1.RoutePort{
				TargetPort: intstr.IntOrString{Type: intstr.Int, IntVal: t.Transport().Port()},
			},
			TLS: termination,
		},
	}

	err := c.Create(context.TODO(), &route, &client.CreateOptions{})
	if err != nil {
		return err
	}

	err = c.Get(context.TODO(), types.NamespacedName{Name: t.PVC().Name, Namespace: t.PVC().Namespace}, &route)
	if err != nil {
		return err
	}

	r.SetHostname(route.Spec.Host)

	return nil
}

func (r *RouteEndpoint) SetHostname(hostname string) {
	r.hostname = hostname
}

func (r *RouteEndpoint) Hostname() string {
	return r.hostname
}

func (r *RouteEndpoint) SetPort(port int32) {
	r.port = port
}

func (r *RouteEndpoint) Port() int32 {
	return r.port
}
