package stunnel

import (
	"bytes"
	"context"
	"strconv"
	"text/template"

	"k8s.io/apimachinery/pkg/types"

	"github.com/konveyor/crane-lib/state_transfer/endpoint"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	errorsutil "k8s.io/apimachinery/pkg/util/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	stunnelClientConfTemplate = `
 pid =
 sslVersion = TLSv1.2
 client = yes
 syslog = no
 output = /dev/stdout
 [rsync]
 debug = 7
 accept = {{ .stunnelPort }}
 cert = /etc/stunnel/certs/tls.crt
 key = /etc/stunnel/certs/tls.key
{{- if not (eq .proxyHost "") }}
 protocol = connect
 connect = {{ .proxyHost }}
 protocolHost = {{ .hostname }}:{{ .port }}
{{- if not (eq .proxyUsername "") }}
 protocolUsername = {{ .proxyUsername }}
{{- end }}
{{- if not (eq .proxyPassword "") }}
 protocolPassword = {{ .proxyPassword }}
{{- end }}
{{- else }}
 connect = {{ .hostname }}:{{ .port }}
{{- end }}
{{- if not (eq .noVerifyCA "false") }}
 verify = {{ .caVerifyLevel }}
{{- end }}
`
)

func (s *StunnelTransport) CreateClient(c client.Client, e endpoint.Endpoint) error {
	err := createClientResources(c, s, e)
	return err
}

func createClientResources(c client.Client, s *StunnelTransport, e endpoint.Endpoint) error {
	errs := []error{}

	// assuming the name of the endpoint is the same as the name of the PVC
	err := createClientConfig(c, s, e)
	errs = append(errs, err)

	err = createClientSecret(c, s, e)
	errs = append(errs, err)

	setClientContainers(s, e)

	createClientVolumes(s)

	return errorsutil.NewAggregate(errs)
}

func getClientConfig(c client.Client, obj types.NamespacedName) (*corev1.ConfigMap, error) {
	cm := &corev1.ConfigMap{}
	err := c.Get(context.Background(), types.NamespacedName{
		Namespace: obj.Namespace,
		Name:      stunnelConfig,
	}, cm)
	return cm, err
}

func createClientConfig(c client.Client, s *StunnelTransport, e endpoint.Endpoint) error {
	var caVerifyLevel string

	if s.Options().CAVerifyLevel == "" {
		caVerifyLevel = "2"
	} else {
		caVerifyLevel = s.Options().CAVerifyLevel
	}

	connections := map[string]string{
		"stunnelPort":   strconv.Itoa(int(e.Port())),
		"hostname":      e.Hostname(),
		"port":          strconv.Itoa(int(e.ExposedPort())),
		"proxyHost":     s.Options().ProxyURL,
		"proxyUsername": s.Options().ProxyUsername,
		"proxyPassword": s.Options().ProxyPassword,
		"caVerifyLevel": caVerifyLevel,
		"noVerifyCA":    strconv.FormatBool(s.Options().NoVerifyCA),
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
			Namespace: s.nsNamePair.Source().Namespace,
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

func getClientSecret(c client.Client, obj types.NamespacedName) (*corev1.Secret, error) {
	secret := &corev1.Secret{}
	err := c.Get(context.Background(), types.NamespacedName{
		Namespace: obj.Namespace,
		Name:      stunnelSecret,
	}, secret)
	return secret, err
}

func createClientSecret(c client.Client, s *StunnelTransport, e endpoint.Endpoint) error {
	stunnelSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: s.nsNamePair.Source().Namespace,
			Name:      stunnelSecret,
			Labels:    e.Labels(),
		},
		Data: map[string][]byte{
			"tls.crt": s.Crt().Bytes(),
			"tls.key": s.Key().Bytes(),
		},
	}

	err := c.Create(context.TODO(), stunnelSecret, &client.CreateOptions{})
	if err != nil && !k8serrors.IsAlreadyExists(err) {
		return err
	}
	return nil
}

func setClientContainers(s *StunnelTransport, e endpoint.Endpoint) {
	s.clientContainers = []corev1.Container{
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

func createClientVolumes(s *StunnelTransport) {
	s.clientVolumes = []corev1.Volume{
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
