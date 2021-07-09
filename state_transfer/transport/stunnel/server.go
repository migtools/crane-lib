package stunnel

import (
	"bytes"
	"context"
	"strconv"
	"text/template"

	"github.com/konveyor/crane-lib/state_transfer/endpoint"

	"github.com/konveyor/crane-lib/state_transfer/transport"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	stunnelServerConfTemplate = `foreground = yes
pid =
socket = l:TCP_NODELAY=1
socket = r:TCP_NODELAY=1
debug = 7
sslVersion = TLSv1.2
[rsync]
accept = {{ $.stunnelPort }}
connect = {{ $.transferPort }}
key = /etc/stunnel/certs/tls.key
cert = /etc/stunnel/certs/tls.crt
TIMEOUTclose = 0
`
)

func (s *StunnelTransport) CreateServer(c client.Client, e endpoint.Endpoint) error {
	err := createStunnelServerResources(c, s, e)
	return err
}

func createStunnelServerResources(c client.Client, s *StunnelTransport, e endpoint.Endpoint) error {
	s.port = stunnelPort

	err := createStunnelServerConfig(c, e)
	if err != nil {
		return err
	}

	err = createStunnelServerSecret(c, s, e)
	if err != nil {
		return err
	}

	createStunnelServerContainers(s, e)

	createStunnelServerVolumes(s, e)

	return nil
}

func createStunnelServerConfig(c client.Client, e endpoint.Endpoint) error {
	ports := map[string]string{
		"stunnelPort":  strconv.Itoa(int(stunnelPort)),
		"transferPort": strconv.Itoa(int(e.Port())),
	}

	var stunnelConf bytes.Buffer
	stunnelConfTemplate, err := template.New("config").Parse(stunnelServerConfTemplate)
	if err != nil {
		return err
	}

	err = stunnelConfTemplate.Execute(&stunnelConf, ports)
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

func createStunnelServerSecret(c client.Client, s *StunnelTransport, e endpoint.Endpoint) error {
	_, crt, key, err := transport.GenerateSSLCert()
	s.key = key
	s.crt = crt
	if err != nil {
		return err
	}

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

func createStunnelServerContainers(s *StunnelTransport, e endpoint.Endpoint) {
	s.serverContainers = []v1.Container{
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

func createStunnelServerVolumes(s *StunnelTransport, e endpoint.Endpoint) {
	s.serverVolumes = []v1.Volume{
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
