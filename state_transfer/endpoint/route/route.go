package route

import (
	"context"

	"github.com/konveyor/crane-lib/state_transfer/endpoint"
	routev1 "github.com/openshift/api/route/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type RouteEndpoint struct {
	name      string
	namespace string
	hostname  string

	labels       map[string]string
	port         int32
	endpointType endpoint.EndpointType
}

func NewRouteEndpoint(name, namespace string, eType endpoint.EndpointType, labels map[string]string) endpoint.Endpoint {
	if eType != endpoint.EndpointTypeRoutePassthrough && eType != endpoint.EndpointTypeRouteInsecureEdge {
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
	err := createRouteService(c, r)
	if err != nil {
		return err
	}

	err = createRoute(c, r)
	if err != nil {
		return err
	}

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

func (r *RouteEndpoint) Name() string {
	return r.name
}

func (r *RouteEndpoint) Namespace() string {
	return r.namespace
}

func (r *RouteEndpoint) Labels() map[string]string {
	return r.labels
}

func (r *RouteEndpoint) Type() endpoint.EndpointType {
	return r.endpointType
}

func createRouteResources(c client.Client, e endpoint.Endpoint) error {
	err := createRouteService(c, e)
	if err != nil {
		return err
	}

	err = createRoute(c, e)
	if err != nil {
		return err
	}

	return nil
}

func createRouteService(c client.Client, e endpoint.Endpoint) error {
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
			Type:     v1.ServiceTypeClusterIP,
		},
	}

	return c.Create(context.TODO(), &service, &client.CreateOptions{})
}

func createRoute(c client.Client, e endpoint.Endpoint) error {
	termination := &routev1.TLSConfig{}
	switch e.Type() {
	case endpoint.EndpointTypeRouteInsecureEdge:
		termination = &routev1.TLSConfig{
			Termination:                   routev1.TLSTerminationEdge,
			InsecureEdgeTerminationPolicy: "Allow",
		}
		e.SetPort(int32(80))
	case endpoint.EndpointTypeRoutePassthrough:
		termination = &routev1.TLSConfig{
			Termination: routev1.TLSTerminationPassthrough,
		}
		e.SetPort(int32(443))
	}

	route := routev1.Route{
		ObjectMeta: metav1.ObjectMeta{
			Name:      e.Name(),
			Namespace: e.Namespace(),
			Labels:    e.Labels(),
		},
		Spec: routev1.RouteSpec{
			To: routev1.RouteTargetReference{
				Kind: "Service",
				Name: e.Name(),
			},
			Port: &routev1.RoutePort{
				TargetPort: intstr.IntOrString{Type: intstr.Int, IntVal: e.Port()},
			},
			TLS: termination,
		},
	}

	err := c.Create(context.TODO(), &route, &client.CreateOptions{})
	if err != nil {
		return err
	}

	err = c.Get(context.TODO(), types.NamespacedName{Name: e.Name(), Namespace: e.Namespace()}, &route)
	if err != nil {
		return err
	}

	e.SetHostname(route.Spec.Host)

	return nil
}
