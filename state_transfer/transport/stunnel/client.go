package stunnel

import (
	"bytes"
	"context"
	"strconv"
	"text/template"

	"k8s.io/apimachinery/pkg/types"

	"github.com/konveyor/crane-lib/state_transfer/endpoint"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	stunnelClientConfTemplate = `foreground = yes
 pid =
 sslVersion = TLSv1.2
 client = yes
 syslog = no
 [rsync]
 accept = {{ .stunnelPort }}
 cert = /etc/stunnel/certs/tls.crt
 connect = {{ .hostname }}:{{ .port }}
 key = /etc/stunnel/certs/tls.key
`
)

func (s *StunnelTransport) CreateClient(c client.Client, e endpoint.Endpoint) error {
	err := createClientResources(c, s, e)
	return err
}

func createClientResources(c client.Client, s *StunnelTransport, e endpoint.Endpoint) error {
	s.port = stunnelPort

	// assuming the name of the endpoint is the same as the name of the PVC

	err := createClientConfig(c, e)
	if err != nil {
		return err
	}

	err = createClientSecret(c, s, e)
	if err != nil {
		return err
	}

	setClientContainers(s, e.NamespacedName())

	createClientVolumes(s, e.NamespacedName())

	return nil
}

func getClientConfig(c client.Client, obj types.NamespacedName) (*corev1.ConfigMap, error) {
	cm := &corev1.ConfigMap{}
	err := c.Get(context.Background(), types.NamespacedName{
		Namespace: obj.Namespace,
		Name:      stunnelConfigPrefix + obj.Name,
	}, cm)
	return cm, err
}

func createClientConfig(c client.Client, e endpoint.Endpoint) error {
	connections := map[string]string{
		"stunnelPort": strconv.Itoa(int(stunnelPort)),
		"hostname":    e.Hostname(),
		"port":        strconv.Itoa(int(e.Port())),
	}

	var stunnelConf bytes.Buffer
	stunnelConfTemplate, err := template.New("config").Parse(stunnelClientConfTemplate)
	if err != nil {
		return err
	}

	err = stunnelConfTemplate.Execute(&stunnelConf, connections)
	if err != nil {
		return err
	}

	stunnelConfigMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: e.NamespacedName().Namespace,
			Name:      stunnelConfigPrefix + e.NamespacedName().Name,
			Labels:    e.Labels(),
		},
		Data: map[string]string{
			"stunnel.conf": string(stunnelConf.Bytes()),
		},
	}

	return c.Create(context.TODO(), stunnelConfigMap, &client.CreateOptions{})

}

func getClientSecret(c client.Client, obj types.NamespacedName) (*corev1.Secret, error) {
	secret := &corev1.Secret{}
	err := c.Get(context.Background(), types.NamespacedName{
		Namespace: obj.Namespace,
		Name:      stunnelSecretPrefix + obj.Name,
	}, secret)
	return secret, err
}

func createClientSecret(c client.Client, s *StunnelTransport, e endpoint.Endpoint) error {
	stunnelSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: e.NamespacedName().Namespace,
			Name:      stunnelSecretPrefix + e.NamespacedName().Name,
			Labels:    e.Labels(),
		},
		Data: map[string][]byte{
			"tls.crt": s.Crt().Bytes(),
			"tls.key": s.Key().Bytes(),
		},
	}

	return c.Create(context.TODO(), stunnelSecret, &client.CreateOptions{})
}

func setClientContainers(s *StunnelTransport, obj types.NamespacedName) {
	s.clientContainers = []corev1.Container{
		{
			Name:  "stunnel",
			Image: stunnelImage,
			Command: []string{
				"/bin/stunnel",
				"/etc/stunnel/stunnel.conf",
			},
			Ports: []corev1.ContainerPort{
				{
					Name:          "stunnel",
					Protocol:      corev1.ProtocolTCP,
					ContainerPort: stunnelPort,
				},
			},
			VolumeMounts: []corev1.VolumeMount{
				{
					Name:      stunnelConfigPrefix + obj.Name,
					MountPath: "/etc/stunnel/stunnel.conf",
					SubPath:   "stunnel.conf",
				},
				{
					Name:      stunnelSecretPrefix + obj.Name,
					MountPath: "/etc/stunnel/certs",
				},
			},
		},
	}
}

func createClientVolumes(s *StunnelTransport, obj types.NamespacedName) {
	s.clientVolumes = []corev1.Volume{
		{
			Name: stunnelConfigPrefix + obj.Name,
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: stunnelConfigPrefix + obj.Name,
					},
				},
			},
		},
		{
			Name: stunnelSecretPrefix + obj.Name,
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName: stunnelSecretPrefix + obj.Name,
					Items: []corev1.KeyToPath{
						{
							Key:  "tls.crt",
							Path: "tls.crt",
						},
						{
							Key:  "tls.key",
							Path: "tls.key",
						},
					},
				},
			},
		},
	}
}
