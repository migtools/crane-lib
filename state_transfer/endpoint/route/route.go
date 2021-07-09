package route

import (
	"context"
	"fmt"

	"github.com/konveyor/crane-lib/state_transfer/endpoint"
	routev1 "github.com/openshift/api/route/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	EndpointTypePassthrough  = "EndpointTypePassthrough"
	EndpointTypeInsecureEdge = "EndpointTypeInsecureEdge"
)

type RouteEndpointType string

type RouteEndpoint struct {
	name      string
	namespace string
	hostname  string

	labels       map[string]string
	port         int32
	endpointType RouteEndpointType
}

func NewEndpoint(name, namespace string, eType RouteEndpointType, labels map[string]string) endpoint.Endpoint {
	if eType != EndpointTypePassthrough && eType != EndpointTypeInsecureEdge {
		panic("unsupported endpoint type for routes")
	}
	return &RouteEndpoint{
		name:         name,
		namespace:    namespace,
		labels:       labels,
		endpointType: eType,
	}
}

func (r *RouteEndpoint) Create(c client.Client) error {
	err := r.createRouteService(c)
	if err != nil {
		return err
	}

	err = r.createRoute(c)
	if err != nil {
		return err
	}

	return nil
}

func (r *RouteEndpoint) setHostname(hostname string) {
	r.hostname = hostname
}

func (r *RouteEndpoint) Hostname() string {
	return r.hostname
}

func (r *RouteEndpoint) Port() int32 {
	return r.port
}

func (r *RouteEndpoint) Name() string {
	return r.name
}

func (r *RouteEndpoint) Namespace() string {
	return r.namespace
}

func (r *RouteEndpoint) Labels() map[string]string {
	return r.labels
}

func (r *RouteEndpoint) IsHealthy(c client.Client) (bool, error) {
	route := &routev1.Route{}
	err := c.Get(context.TODO(), types.NamespacedName{Name: r.Name(), Namespace: r.Namespace()}, route)
	if err != nil {
		return false, err
	}
	if route.Spec.Host == "" {
		return false, fmt.Errorf("hostname not set for rsync route: %s", route)
	}

	if len(route.Status.Ingress) > 0 && len(route.Status.Ingress[0].Conditions) > 0 {
		for _, c := range route.Status.Ingress[0].Conditions {
			if c.Type == routev1.RouteAdmitted && c.Status == corev1.ConditionTrue {
				// TODO: remove setHostname and configure the hostname after this condition has been satisfied,
				//  this is the implementation detail that we dont need the users of the interface work with
				return true, nil
			}
		}
	}
	// TODO: probably using error.Wrap/Unwrap here makes much more sense
	return false, fmt.Errorf("route status is not in valid state: %s", route.Status)
}

func (r *RouteEndpoint) createRouteResources(c client.Client) error {
	err := r.createRouteService(c)
	if err != nil {
		return err
	}

	err = r.createRoute(c)
	if err != nil {
		return err
	}

	return nil
}

func (r *RouteEndpoint) createRouteService(c client.Client) error {
	serviceSelector := r.Labels()
	serviceSelector["pvc"] = r.Name()

	service := corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      r.Name(),
			Namespace: r.Namespace(),
			Labels:    r.Labels(),
		},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{
				{
					Name:       r.Name(),
					Protocol:   corev1.ProtocolTCP,
					Port:       r.Port(),
					TargetPort: intstr.IntOrString{Type: intstr.Int, IntVal: r.Port()},
				},
			},
			Selector: serviceSelector,
			Type:     corev1.ServiceTypeClusterIP,
		},
	}

	return c.Create(context.TODO(), &service, &client.CreateOptions{})
}

func (r *RouteEndpoint) createRoute(c client.Client) error {
	termination := &routev1.TLSConfig{}
	switch r.endpointType {
	case EndpointTypeInsecureEdge:
		termination = &routev1.TLSConfig{
			Termination:                   routev1.TLSTerminationEdge,
			InsecureEdgeTerminationPolicy: "Allow",
		}
		r.port = int32(80)
	case EndpointTypePassthrough:
		termination = &routev1.TLSConfig{
			Termination: routev1.TLSTerminationPassthrough,
		}
		r.port = int32(443)
	}

	route := routev1.Route{
		ObjectMeta: metav1.ObjectMeta{
			Name:      r.Name(),
			Namespace: r.Namespace(),
			Labels:    r.Labels(),
		},
		Spec: routev1.RouteSpec{
			To: routev1.RouteTargetReference{
				Kind: "Service",
				Name: r.Name(),
			},
			Port: &routev1.RoutePort{
				TargetPort: intstr.IntOrString{Type: intstr.Int, IntVal: r.Port()},
			},
			TLS: termination,
		},
	}

	err := c.Create(context.TODO(), &route, &client.CreateOptions{})
	if err != nil {
		return err
	}

	err = c.Get(context.TODO(), types.NamespacedName{Name: r.Name(), Namespace: r.Namespace()}, &route)
	if err != nil {
		return err
	}

	r.setHostname(route.Spec.Host)

	return nil
}

func (r *RouteEndpoint) setPort(port int32) {
	r.port = port
}
