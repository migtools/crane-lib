package stunnel

import (
	"bytes"
	"context"
	"strconv"
	"text/template"

	"k8s.io/apimachinery/pkg/types"

	"github.com/konveyor/crane-lib/state_transfer/endpoint"

	"github.com/konveyor/crane-lib/state_transfer/transport"

	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	errorsutil "k8s.io/apimachinery/pkg/util/errors"
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
accept = {{ $.acceptPort }}
connect = {{ $.connectPort }}
key = /etc/stunnel/certs/tls.key
cert = /etc/stunnel/certs/tls.crt
TIMEOUTclose = 0
`
)

func (s *StunnelTransport) CreateServer(c client.Client, e endpoint.Endpoint) error {
	err := createStunnelServerResources(c, s, e)
	s.port = e.Port()
	return err
}

func createStunnelServerResources(c client.Client, s *StunnelTransport, e endpoint.Endpoint) error {
	errs := []error{}

	err := createStunnelServerConfig(c, s, e)
	errs = append(errs, err)

	err = createStunnelServerSecret(c, s, e)
	errs = append(errs, err)

	createStunnelServerContainers(s, e)

	createStunnelServerVolumes(s)

	return errorsutil.NewAggregate(errs)
}

func createStunnelServerConfig(c client.Client, s *StunnelTransport, e endpoint.Endpoint) error {
	ports := map[string]string{
		// port on which Stunnel service listens on, must connect with endpoint
		"acceptPort": strconv.Itoa(int(e.Port())),
		// port in the container on which Transfer is listening on
		"connectPort": strconv.Itoa(int(s.ExposedPort())),
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

	stunnelConfigMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: s.nsNamePair.Destination().Namespace,
			Name:      stunnelConfig,
			Labels:    e.Labels(),
		},
		Data: map[string]string{
			"stunnel.conf": stunnelConf.String(),
		},
	}

	err = c.Create(context.TODO(), stunnelConfigMap, &client.CreateOptions{})
	if err != nil && !k8serrors.IsAlreadyExists(err) {
		return err
	}
	return nil
}

func getServerConfig(c client.Client, obj types.NamespacedName) (*corev1.ConfigMap, error) {
	cm := &corev1.ConfigMap{}
	err := c.Get(context.Background(), types.NamespacedName{
		Namespace: obj.Namespace,
		Name:      stunnelConfig,
	}, cm)
	return cm, err
}

func createStunnelServerSecret(c client.Client, s *StunnelTransport, e endpoint.Endpoint) error {
	_, crt, key, err := transport.GenerateSSLCert()
	s.key = key
	s.crt = crt
	if err != nil {
		return err
	}

	stunnelSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: s.nsNamePair.Destination().Namespace,
			Name:      stunnelSecret,
			Labels:    e.Labels(),
		},
		Data: map[string][]byte{
			"tls.crt": s.Crt().Bytes(),
			"tls.key": s.Key().Bytes(),
		},
	}

	err = c.Create(context.TODO(), stunnelSecret, &client.CreateOptions{})
	if err != nil && !k8serrors.IsAlreadyExists(err) {
		return err
	}
	return nil
}

func getServerSecret(c client.Client, obj types.NamespacedName) (*corev1.Secret, error) {
	secret := &corev1.Secret{}
	err := c.Get(context.Background(), types.NamespacedName{
		Namespace: obj.Namespace,
		Name:      stunnelSecret,
	}, secret)
	return secret, err
}

func createStunnelServerContainers(s *StunnelTransport, e endpoint.Endpoint) {
	s.serverContainers = []corev1.Container{
		{
			Name:  StunnelContainer,
			Image: stunnelImage,
			Command: []string{
				"/bin/stunnel",
				"/etc/stunnel/stunnel.conf",
			},
			Ports: []corev1.ContainerPort{
				{
					Name:          "stunnel",
					Protocol:      corev1.ProtocolTCP,
					ContainerPort: e.Port(),
				},
			},
			VolumeMounts: []corev1.VolumeMount{
				{
					Name:      stunnelConfig,
					MountPath: "/etc/stunnel/stunnel.conf",
					SubPath:   "stunnel.conf",
				},
				{
					Name:      stunnelSecret,
					MountPath: "/etc/stunnel/certs",
				},
			},
		},
	}
}

func createStunnelServerVolumes(s *StunnelTransport) {
	s.serverVolumes = []corev1.Volume{
		{
			Name: stunnelConfig,
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: stunnelConfig,
					},
				},
			},
		},
		{
			Name: stunnelSecret,
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName: stunnelSecret,
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
