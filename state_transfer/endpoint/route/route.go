package route

import (
	"context"
	"fmt"

	"github.com/konveyor/crane-lib/state_transfer/endpoint"
	routev1 "github.com/openshift/api/route/v1"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	errorsutil "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	EndpointTypePassthrough  = "EndpointTypePassthrough"
	EndpointTypeInsecureEdge = "EndpointTypeInsecureEdge"
)

type RouteEndpointType string

type RouteEndpoint struct {
	hostname string

	labels         map[string]string
	port           int32
	endpointType   RouteEndpointType
	namespacedName types.NamespacedName
}

func NewEndpoint(namespacedName types.NamespacedName, eType RouteEndpointType, labels map[string]string) endpoint.Endpoint {
	if eType != EndpointTypePassthrough && eType != EndpointTypeInsecureEdge {
		panic("unsupported endpoint type for routes")
	}
	return &RouteEndpoint{
		namespacedName: namespacedName,
		labels:         labels,
		endpointType:   eType,
	}
}

func (r *RouteEndpoint) Create(c client.Client) error {
	errs := []error{}

	err := r.createRoute(c)
	errs = append(errs, err)

	err = r.createRouteService(c)
	errs = append(errs, err)

	return errorsutil.NewAggregate(errs)
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

func (r *RouteEndpoint) NamespacedName() types.NamespacedName {
	return r.namespacedName
}

func (r *RouteEndpoint) Labels() map[string]string {
	return r.labels
}

func (r *RouteEndpoint) ExposedPort() int32 {
	return 443
}

func (r *RouteEndpoint) IsHealthy(c client.Client) (bool, error) {
	route := &routev1.Route{}
	err := c.Get(context.TODO(), r.NamespacedName(), route)
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

func (r *RouteEndpoint) createRouteService(c client.Client) error {
	serviceSelector := r.Labels()

	service := corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      r.NamespacedName().Name,
			Namespace: r.NamespacedName().Namespace,
			Labels:    r.Labels(),
		},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{
				{
					Name:     r.NamespacedName().Name,
					Protocol: corev1.ProtocolTCP,
					Port:     r.Port(),
					TargetPort: intstr.IntOrString{
						Type:   intstr.Int,
						IntVal: r.Port()},
				},
			},
			Selector: serviceSelector,
			Type:     corev1.ServiceTypeClusterIP,
		},
	}
	err := c.Create(context.TODO(), &service, &client.CreateOptions{})
	if err != nil && !k8serrors.IsAlreadyExists(err) {
		return err
	}

	return nil
}

func (r *RouteEndpoint) createRoute(c client.Client) error {
	termination := &routev1.TLSConfig{}
	switch r.endpointType {
	case EndpointTypeInsecureEdge:
		termination = &routev1.TLSConfig{
			Termination:                   routev1.TLSTerminationEdge,
			InsecureEdgeTerminationPolicy: "Allow",
		}
		r.port = int32(8080)
	case EndpointTypePassthrough:
		termination = &routev1.TLSConfig{
			Termination: routev1.TLSTerminationPassthrough,
		}
		r.port = int32(6443)
	}

	route := routev1.Route{
		ObjectMeta: metav1.ObjectMeta{
			Name:      r.NamespacedName().Name,
			Namespace: r.NamespacedName().Namespace,
			Labels:    r.Labels(),
		},
		Spec: routev1.RouteSpec{
			Port: &routev1.RoutePort{
				TargetPort: intstr.FromInt(int(r.Port())),
			},
			To: routev1.RouteTargetReference{
				Kind: "Service",
				Name: r.NamespacedName().Name,
			},
			TLS: termination,
		},
	}

	err := c.Create(context.TODO(), &route, &client.CreateOptions{})
	if err != nil && !k8serrors.IsAlreadyExists(err) {
		return err
	}

	err = c.Get(context.TODO(), types.NamespacedName{Name: r.NamespacedName().Name, Namespace: r.NamespacedName().Namespace}, &route)
	if err != nil {
		return err
	}

	r.setHostname(route.Spec.Host)

	return nil
}

func (r *RouteEndpoint) getRoute(c client.Client) (*routev1.Route, error) {
	route := &routev1.Route{}
	err := c.Get(context.TODO(), types.NamespacedName{Name: r.NamespacedName().Name, Namespace: r.NamespacedName().Namespace}, route)
	if err != nil {
		return nil, err
	}
	return route, err
}

func (r *RouteEndpoint) setFields(c client.Client) error {
	route, err := r.getRoute(c)
	if err != nil {
		return err
	}

	if route.Spec.Host == "" {
		return fmt.Errorf("route %s has empty spec.host field", r.NamespacedName())
	}
	if route.Spec.Port == nil {
		return fmt.Errorf("route %s has empty spec.port field", r.NamespacedName())
	}

	r.hostname = route.Spec.Host

	r.port = route.Spec.Port.TargetPort.IntVal

	switch route.Spec.TLS.Termination {
	case routev1.TLSTerminationEdge:
		r.endpointType = EndpointTypeInsecureEdge
	case routev1.TLSTerminationPassthrough:
		r.endpointType = EndpointTypePassthrough
	default:
		return fmt.Errorf("route %s has unsupported spec.spec.tls.termination value", r.NamespacedName())
	}

	return nil
}

// GetEndpointFromKubeObjects check if the required Route is created and healthy. It populates the fields
// for the Endpoint needed for transfer and transport objects.
func GetEndpointFromKubeObjects(c client.Client, obj types.NamespacedName) (endpoint.Endpoint, error) {
	r := &RouteEndpoint{namespacedName: obj}

	healthy, err := r.IsHealthy(c)
	if err != nil {
		return nil, err
	}
	if !healthy {
		return nil, fmt.Errorf("route %s not healthy", obj)
	}

	err = r.setFields(c)
	if err != nil {
		return nil, err
	}

	return r, nil
}
