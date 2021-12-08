package ingress

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"fmt"

	"github.com/konveyor/crane-lib/state_transfer/endpoint"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	errorsutil "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type IngressEndpoint struct {
	hostname string

	labels         map[string]string
	port           int32
	namespacedName types.NamespacedName
}

func (i *IngressEndpoint) Create(c client.Client) error {
	errs := []error{}

	err := i.createIngressService(c)
	errs = append(errs, err)

	err = i.createIngress(c)
	errs = append(errs, err)

	return errorsutil.NewAggregate(errs)
}

func (i *IngressEndpoint) Hostname() string {
	return i.hostname
}

func (i *IngressEndpoint) Port() int32 {
	return i.port
}

func (i *IngressEndpoint) ExposedPort() int32 {
	return 443
}

func (i *IngressEndpoint) NamespacedName() types.NamespacedName {
	return i.namespacedName
}

func (i *IngressEndpoint) Labels() map[string]string {
	return i.labels
}

func (i *IngressEndpoint) IsHealthy(c client.Client) (bool, error) {
	ing := &networkingv1.Ingress{}
	err := c.Get(context.TODO(), i.NamespacedName(), ing)
	if err != nil {
		return false, err
	}
	if len(ing.Spec.Rules) > 0 && ing.Spec.Rules[0].Host == "" {
		return false, fmt.Errorf("hostname not set for ingress: %s", ing)
	}

	if len(ing.Status.LoadBalancer.Ingress) > 0 && ing.Status.LoadBalancer.Ingress[0].Hostname != "" {
		return true, nil
	}
	return false, nil
}

func (i *IngressEndpoint) createIngressService(c client.Client) error {
	serviceSelector := i.Labels()

	service := corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      i.NamespacedName().Name,
			Namespace: i.NamespacedName().Namespace,
			Labels:    i.Labels(),
		},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{
				{
					Name:     i.NamespacedName().Name,
					Protocol: corev1.ProtocolTCP,
					Port:     i.Port(),
					TargetPort: intstr.IntOrString{
						Type:   intstr.Int,
						IntVal: i.Port()},
				},
			},
			Selector: serviceSelector,
			Type:     corev1.ServiceTypeNodePort,
		},
	}
	// TODO: consider patching an existing object if it already exists
	err := c.Create(context.TODO(), &service, &client.CreateOptions{})
	if err != nil && !k8serrors.IsAlreadyExists(err) {
		return err
	}

	return nil
}

func (i *IngressEndpoint) createIngress(c client.Client) error {
	pathType := networkingv1.PathTypePrefix
	ing := networkingv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      i.NamespacedName().Name,
			Namespace: i.NamespacedName().Namespace,
			Labels:    i.Labels(),
			Annotations: map[string]string{
				"nginx.ingress.kubernetes.io/ssl-passthrough": "true",
			},
		},
		Spec: networkingv1.IngressSpec{
			Rules: []networkingv1.IngressRule{{
				Host: i.hostname,
				IngressRuleValue: networkingv1.IngressRuleValue{
					HTTP: &networkingv1.HTTPIngressRuleValue{
						Paths: []networkingv1.HTTPIngressPath{{
							Backend: networkingv1.IngressBackend{
								Service: &networkingv1.IngressServiceBackend{
									Name: i.namespacedName.Name,
									Port: networkingv1.ServiceBackendPort{
										Number: i.port,
									},
								},
							},
							Path:     "/",
							PathType: &pathType,
						}},
					},
				},
			}},
		},
	}

	err := c.Create(context.TODO(), &ing, &client.CreateOptions{})
	if err != nil && !k8serrors.IsAlreadyExists(err) {
		return err
	}
	return nil
}

func getMD5Hash(s string) string {
	hash := md5.Sum([]byte(s))
	return hex.EncodeToString(hash[:])
}

func NewEndpoint(namespacedName types.NamespacedName, labels map[string]string, subdomain string) endpoint.Endpoint {
	ingressPrefix := fmt.Sprintf("%s-%s", namespacedName.Name, namespacedName.Namespace)
	if len(ingressPrefix) > 62 {
		ingressPrefix = fmt.Sprintf("%s-%s", namespacedName.Name, getMD5Hash(namespacedName.Namespace))
	}

	i := &IngressEndpoint{
		namespacedName: namespacedName,
		labels:         labels,
		port:           6443,
		hostname:       ingressPrefix + "." + subdomain,
	}
	return i
}

func (i *IngressEndpoint) setFields(c client.Client) error {
	i.port = 6443

	ing := &networkingv1.Ingress{}
	err := c.Get(context.TODO(), i.NamespacedName(), ing)
	if err != nil {
		return err
	}

	i.labels = ing.Labels
	if len(ing.Spec.Rules) > 0 {
		i.hostname = ing.Spec.Rules[0].Host
		return nil
	}
	return fmt.Errorf("ingress %s does not have the right spec", i.namespacedName)
}

// GetEndpointFromKubeObjects check if the required Ingress is created and healthy. It populates the fields
// for the Endpoint needed for transfer and transport objects.
func GetEndpointFromKubeObjects(c client.Client, obj types.NamespacedName) (endpoint.Endpoint, error) {
	i := &IngressEndpoint{namespacedName: obj}

	healthy, err := i.IsHealthy(c)
	if err != nil {
		return nil, err
	}
	if !healthy {
		return nil, fmt.Errorf("ingress %s not healthy", obj)
	}

	err = i.setFields(c)

	return i, nil
}
