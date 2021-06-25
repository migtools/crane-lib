package state_transfer

import (
	"bytes"
	"context"
	"strconv"
	"text/template"

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

func (s *StunnelTransport) CreateClient(c client.Client, t Transfer) error {
	err := createStunnelClientResources(c, s, t)
	return err
}

func createStunnelClientResources(c client.Client, s *StunnelTransport, t Transfer) error {
	s.SetPort(stunnelPort)
	pvc := t.PVC()

	err := createStunnelClientConfig(c, s, t)
	if err != nil {
		return err
	}

	err = createStunnelClientSecret(c, s, pvc)
	if err != nil {
		return err
	}

	createStunnelClientContainers(s, t)

	createStunnelClientVolumes(s, t)

	return nil
}

func createStunnelClientConfig(c client.Client, s *StunnelTransport, t Transfer) error {
	connections := map[string]string{
		"stunnelPort": strconv.Itoa(int(stunnelPort)),
		"hostname":    t.Endpoint().Hostname(),
		"port":        strconv.Itoa(int(t.Endpoint().Port())),
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
			Namespace: t.PVC().Namespace,
			Name:      stunnelConfigPrefix + t.PVC().Name,
			Labels:    labels,
		},
		Data: map[string]string{
			"stunnel.conf": string(stunnelConf.Bytes()),
		},
	}

	return c.Create(context.TODO(), stunnelConfigMap, &client.CreateOptions{})

}

func createStunnelClientSecret(c client.Client, s *StunnelTransport, pvc v1.PersistentVolumeClaim) error {
	stunnelSecret := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: pvc.Namespace,
			Name:      stunnelSecretPrefix + pvc.Name,
			Labels:    labels,
		},
		Data: map[string][]byte{
			"tls.crt": s.Crt().Bytes(),
			"tls.key": s.Key().Bytes(),
		},
	}

	return c.Create(context.TODO(), stunnelSecret, &client.CreateOptions{})
}

func createStunnelClientContainers(s *StunnelTransport, t Transfer) {
	s.SetClientContainers([]v1.Container{
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
					Name:      stunnelConfigPrefix + t.PVC().Name,
					MountPath: "/etc/stunnel/stunnel.conf",
					SubPath:   "stunnel.conf",
				},
				{
					Name:      stunnelSecretPrefix + t.PVC().Name,
					MountPath: "/etc/stunnel/certs",
				},
			},
		},
	})
}

func createStunnelClientVolumes(s *StunnelTransport, t Transfer) {
	s.SetClientVolumes([]v1.Volume{
		{
			Name: stunnelConfigPrefix + t.PVC().Name,
			VolumeSource: v1.VolumeSource{
				ConfigMap: &v1.ConfigMapVolumeSource{
					LocalObjectReference: v1.LocalObjectReference{
						Name: stunnelConfigPrefix + t.PVC().Name,
					},
				},
			},
		},
		{
			Name: stunnelSecretPrefix + t.PVC().Name,
			VolumeSource: v1.VolumeSource{
				Secret: &v1.SecretVolumeSource{
					SecretName: stunnelSecretPrefix + t.PVC().Name,
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
	})
}
