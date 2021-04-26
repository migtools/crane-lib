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

func (s *StunnelTransport) createTransportServerResources(c client.Client, t Transfer) error {
	s.SetTransportPort(stunnelPort)
	pvc := t.GetPVC()

	err := createStunnelServerConfig(c, s, t)
	if err != nil {
		return err
	}

	err = createStunnelServerSecret(c, s, pvc)
	if err != nil {
		return err
	}

	createStunnelServerContainers(s, t)

	createStunnelServerVolumes(s, t)

	return nil
}

func createStunnelServerConfig(c client.Client, s *StunnelTransport, t Transfer) error {
	ports := map[string]string{
		"stunnelPort":  strconv.Itoa(int(stunnelPort)),
		"transferPort": strconv.Itoa(int(t.GetTransferPort())),
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
			Namespace: t.GetPVC().Namespace,
			Name:      "crane2-stunnel-conf-" + t.GetPVC().Name,
			Labels:    labels,
		},
		Data: map[string]string{
			"stunnel.conf": string(stunnelConf.Bytes()),
		},
	}

	return c.Create(context.TODO(), stunnelConfigMap, &client.CreateOptions{})

}

func createStunnelServerSecret(c client.Client, s *StunnelTransport, pvc v1.PersistentVolumeClaim) error {
	_, crt, key, err := generateSSLCert()
	s.SetKey(key)
	s.SetCrt(crt)
	if err != nil {
		return err
	}

	stunnelSecret := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: pvc.Namespace,
			Name:      "crane2-stunnel-secret-" + pvc.Name,
			Labels:    labels,
		},
		Data: map[string][]byte{
			"tls.crt": s.GetCrt().Bytes(),
			"tls.key": s.GetKey().Bytes(),
		},
	}

	return c.Create(context.TODO(), stunnelSecret, &client.CreateOptions{})
}

func createStunnelServerContainers(s *StunnelTransport, t Transfer) {
	s.SetServerContainers([]v1.Container{
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
					Name:      "crane2-stunnel-conf-" + t.GetPVC().Name,
					MountPath: "/etc/stunnel/stunnel.conf",
					SubPath:   "stunnel.conf",
				},
				{
					Name:      "crane2-stunnel-secret-" + t.GetPVC().Name,
					MountPath: "/etc/stunnel/certs",
				},
			},
		},
	})
}

func createStunnelServerVolumes(s *StunnelTransport, t Transfer) {
	s.SetServerVolumes([]v1.Volume{
		{
			Name: "crane2-stunnel-conf-" + t.GetPVC().Name,
			VolumeSource: v1.VolumeSource{
				ConfigMap: &v1.ConfigMapVolumeSource{
					LocalObjectReference: v1.LocalObjectReference{
						Name: "crane2-stunnel-conf-" + t.GetPVC().Name,
					},
				},
			},
		},
		{
			Name: "crane2-stunnel-secret-" + t.GetPVC().Name,
			VolumeSource: v1.VolumeSource{
				Secret: &v1.SecretVolumeSource{
					SecretName: "crane2-stunnel-secret-" + t.GetPVC().Name,
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