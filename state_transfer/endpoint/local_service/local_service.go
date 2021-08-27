package local_service

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

const clusterSubDomain = "svc.cluster.local"

type LocalServiceEndpoint struct {
	namespacedName types.NamespacedName
	labels         map[string]string
	port           int32
}

func NewEndpoint(namespacedName types.NamespacedName, labels map[string]string) endpoint.Endpoint {
	return &LocalServiceEndpoint{
		namespacedName: namespacedName,
		port:           2222,
		labels:         labels,
	}
}

func (l *LocalServiceEndpoint) Create(c client.Client) error {
	err := l.createLoadBalancerService(c)
	if err != nil {
		return err
	}

	return nil
}

func (l *LocalServiceEndpoint) Hostname() string {
	return fmt.Sprintf("%v.%v.%v", l.namespacedName.Name, l.namespacedName.Namespace, clusterSubDomain)
}

func (l *LocalServiceEndpoint) Port() int32 {
	return l.port
}

func (l *LocalServiceEndpoint) NamespacedName() types.NamespacedName {
	return l.namespacedName
}

func (l *LocalServiceEndpoint) Labels() map[string]string {
	return l.labels
}

func (l *LocalServiceEndpoint) ExposedPort() int32 {
	return l.port
}

func (l *LocalServiceEndpoint) IsHealthy(c client.Client) (bool, error) {
	service := corev1.Service{}
	err := c.Get(context.TODO(), types.NamespacedName{Name: l.NamespacedName().Name, Namespace: l.NamespacedName().Namespace}, &service)
	if err != nil {
		return false, err
	}
	if len(service.Spec.ClusterIPs) == 0 {
		return false, fmt.Errorf("no cluster IPs allocated for service")
	}
	return true, nil
}

func (l *LocalServiceEndpoint) createLoadBalancerService(c client.Client) error {
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
			Type:     corev1.ServiceTypeClusterIP,
		},
	}

	err := c.Create(context.TODO(), &service, &client.CreateOptions{})
	if err != nil {
		return err
	}
	return nil
}
