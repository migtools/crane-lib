package service

import (
	"context"
	"fmt"

	"github.com/konveyor/crane-lib/state_transfer/endpoint"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type ServiceEndpoint struct {
	namespacedName types.NamespacedName
	hostname       string
	svcType        corev1.ServiceType

	labels      map[string]string
	backendPort int32
	exposedPort int32
}

func NewEndpoint(namespacedName types.NamespacedName, labels map[string]string, hostname string, svcType corev1.ServiceType) endpoint.Endpoint {
	return &ServiceEndpoint{
		namespacedName: namespacedName,
		labels:         labels,
		svcType:        svcType,
		hostname:       hostname,
		backendPort:    int32(6443),
		exposedPort:    int32(6443),
	}
}

func (s *ServiceEndpoint) Create(c client.Client) error {
	err := s.createService(c)
	if err != nil {
		return err
	}

	return nil
}

func (s *ServiceEndpoint) Hostname() string {
	return s.hostname
}

func (s *ServiceEndpoint) Port() int32 {
	return s.backendPort
}

func (s *ServiceEndpoint) NamespacedName() types.NamespacedName {
	return s.namespacedName
}

func (s *ServiceEndpoint) Labels() map[string]string {
	return s.labels
}

func (s *ServiceEndpoint) ExposedPort() int32 {
	return s.exposedPort
}

func (s *ServiceEndpoint) IsHealthy(c client.Client) (bool, error) {
	svc := corev1.Service{}
	err := c.Get(context.TODO(), types.NamespacedName{
		Name:      s.NamespacedName().Name,
		Namespace: s.NamespacedName().Namespace}, &svc)
	if err != nil {
		return false, err
	}

	s.backendPort = svc.Spec.Ports[0].TargetPort.IntVal
	s.exposedPort = svc.Spec.Ports[0].Port
	s.labels = svc.Labels

	switch svc.Spec.Type {
	case corev1.ServiceTypeLoadBalancer:
		if len(svc.Status.LoadBalancer.Ingress) > 0 {
			if svc.Status.LoadBalancer.Ingress[0].Hostname != "" {
				s.hostname = svc.Status.LoadBalancer.Ingress[0].Hostname
			}
			if svc.Status.LoadBalancer.Ingress[0].IP != "" {
				s.hostname = svc.Status.LoadBalancer.Ingress[0].IP
			}
			return true, nil
		}
	case corev1.ServiceTypeClusterIP:
		if svc.Spec.ClusterIP != "" {
			s.hostname = svc.Spec.ClusterIP
		}
		if svc.Labels["hostname"] != "" {
			s.hostname = svc.Labels["hostname"]
		}
		return true, nil
	case corev1.ServiceTypeNodePort:
		if svc.Spec.ClusterIP != "" {
			s.hostname = svc.Spec.ClusterIP
			if len(svc.Spec.Ports) > 0 {
				port := svc.Spec.Ports[0]
				if port.NodePort != 0 {
					s.exposedPort = port.NodePort
				}
			}
		}
		if svc.Labels["hostname"] != "" {
			s.hostname = svc.Labels["hostname"]
		}
		return true, nil
	default:
		return false, fmt.Errorf("unsupported service type %s", s.svcType)
	}

	return false, fmt.Errorf("service status is not in valid state: %s", svc.Status.String())
}

func (s *ServiceEndpoint) createService(c client.Client) error {
	service := corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      s.NamespacedName().Name,
			Namespace: s.NamespacedName().Namespace,
			Labels:    s.getSvcLabels(),
		},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{
				{
					Name:       s.NamespacedName().Name,
					Protocol:   corev1.ProtocolTCP,
					Port:       s.ExposedPort(),
					TargetPort: intstr.IntOrString{Type: intstr.Int, IntVal: s.Port()},
				},
			},
			Selector: s.Labels(),
			Type:     s.svcType,
		},
	}

	err := c.Create(context.TODO(), &service, &client.CreateOptions{})
	if err != nil {
		return err
	}

	return nil

}

func (s *ServiceEndpoint) getSvcLabels() map[string]string {
	labels := make(map[string]string)
	for key, val := range s.labels {
		labels[key] = val
	}
	labels["hostname"] = s.hostname
	return labels
}

// GetEndpointFromKubeObjects check if the required svc is created and healthy. It populates the fields
// for the Endpoint needed for transfer and transport objects.
func GetEndpointFromKubeObjects(c client.Client, obj types.NamespacedName) (endpoint.Endpoint, error) {
	r := &ServiceEndpoint{namespacedName: obj}

	healthy, err := r.IsHealthy(c)
	if err != nil {
		return nil, err
	}
	if !healthy {
		return nil, fmt.Errorf("endpoint %s not healthy", obj)
	}

	return r, nil
}
