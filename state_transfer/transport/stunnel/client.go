package stunnel

import (
	"bytes"
	"context"
	"strconv"
	"text/template"

	"github.com/konveyor/crane-lib/state_transfer/endpoint"
	v1 "k8s.io/api/core/v1"
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
	err := createStunnelClientResources(c, s, e)
	return err
}

func createStunnelClientResources(c client.Client, s *StunnelTransport, e endpoint.Endpoint) error {
	s.port = stunnelPort

	// assuming the name of the endpoint is the same as the name of the PVC

	err := createStunnelClientConfig(c, e)
	if err != nil {
		return err
	}

	err = createStunnelClientSecret(c, s, e)
	if err != nil {
		return err
	}

	createStunnelClientContainers(s, e)

	createStunnelClientVolumes(s, e)

	return nil
}

func createStunnelClientConfig(c client.Client, e endpoint.Endpoint) error {
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

	stunnelConfigMap := &v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: e.Namespace(),
			Name:      stunnelConfigPrefix + e.Name(),
			Labels:    e.Labels(),
		},
		Data: map[string]string{
			"stunnel.conf": string(stunnelConf.Bytes()),
		},
	}

	return c.Create(context.TODO(), stunnelConfigMap, &client.CreateOptions{})

}

func createStunnelClientSecret(c client.Client, s *StunnelTransport, e endpoint.Endpoint) error {
	stunnelSecret := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: e.Namespace(),
			Name:      stunnelSecretPrefix + e.Name(),
			Labels:    e.Labels(),
		},
		Data: map[string][]byte{
			"tls.crt": s.Crt().Bytes(),
			"tls.key": s.Key().Bytes(),
		},
	}

	return c.Create(context.TODO(), stunnelSecret, &client.CreateOptions{})
}

func createStunnelClientContainers(s *StunnelTransport, e endpoint.Endpoint) {
	s.clientContainers = []v1.Container{
		{
			Name:  "stunnel",
			Image: stunnelImage,
			Command: []string{
				"/bin/stunnel",
				"/etc/stunnel/stunnel.conf",
			},
			Ports: []v1.ContainerPort{
				{
					Name:          "stunnel",
					Protocol:      v1.ProtocolTCP,
					ContainerPort: stunnelPort,
				},
			},
			VolumeMounts: []v1.VolumeMount{
				{
					Name:      stunnelConfigPrefix + e.Name(),
					MountPath: "/etc/stunnel/stunnel.conf",
					SubPath:   "stunnel.conf",
				},
				{
					Name:      stunnelSecretPrefix + e.Name(),
					MountPath: "/etc/stunnel/certs",
				},
			},
		},
	}
}

func createStunnelClientVolumes(s *StunnelTransport, e endpoint.Endpoint) {
	s.clientVolumes = []v1.Volume{
		{
			Name: stunnelConfigPrefix + e.Name(),
			VolumeSource: v1.VolumeSource{
				ConfigMap: &v1.ConfigMapVolumeSource{
					LocalObjectReference: v1.LocalObjectReference{
						Name: stunnelConfigPrefix + e.Name(),
					},
				},
			},
		},
		{
			Name: stunnelSecretPrefix + e.Name(),
			VolumeSource: v1.VolumeSource{
				Secret: &v1.SecretVolumeSource{
					SecretName: stunnelSecretPrefix + e.Name(),
					Items: []v1.KeyToPath{
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
