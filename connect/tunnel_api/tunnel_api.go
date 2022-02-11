package tunnel_api

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"strconv"
	"strings"
	"text/template"
	"time"

	dhparam "github.com/Luzifer/go-dhparam"
	securityv1 "github.com/openshift/api/security/v1"
	appsv1 "k8s.io/api/apps/v1"
	appsv1beta1 "k8s.io/api/apps/v1beta1"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	dapi "k8s.io/client-go/discovery"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	openvpnClientConfTemplate = `client
cipher AES-256-GCM
dev tun
proto tcp4
remote {{ $.Endpoint }} {{ $.Port }} tcp-client
{{ $.Proxy }}
<ca>
{{ .CA }}
</ca>
<key>
{{ .Key }}
</key>
<cert>
{{ .Crt }}
</cert>
verify-x509-name "C=US, ST=NC, L=RDU, O=Engineering, OU=Crane, CN=Server"
`
	openvpnServerConfTemplate = `dh /certs/dh.pem
cipher AES-256-GCM
ca /certs/ca.crt
server 192.168.123.0 255.255.255.0
dev tun0
proto tcp4
port {{ $.Port }}
keepalive 10 120
tmp-dir /tmp/openvpn
cert /certs/server.crt
key /certs/server.key
`
	serviceName   = "openvpn"
	serviceConfig = "openvpn-conf"
	proxyConfig   = "openvpn-proxy-creds"
	keySize       = 2048
)

type Tunnel struct {
	DstClient       client.Client
	DstConfig       *rest.Config
	DstVersionMinor int
	SrcClient       client.Client
	SrcConfig       *rest.Config
	SrcVersionMinor int
	Options         Options
}

type Options struct {
	Namespace   string
	CACrt       *bytes.Buffer
	ClientCrt   *bytes.Buffer
	ClientKey   *bytes.Buffer
	ServerCrt   *bytes.Buffer
	ServerKey   *bytes.Buffer
	RSADHKey    *bytes.Buffer
	ClientImage string
	ServerImage string
	ServerPort  int32
	ProxyHost   string
	ProxyPort   string
	ProxyUser   string
	ProxyPass   string
}

type openvpnConfigData struct {
	Port     string
	CA       string
	Crt      string
	Key      string
	Endpoint string
	Proxy    string
}

func Openvpn(tunnel Tunnel) error {
	if tunnel.Options.Namespace == "" {
		tunnel.Options.Namespace = serviceName
	}

	if tunnel.Options.ClientImage == "" {
		tunnel.Options.ClientImage = "quay.io/konveyor/openvpn:latest"
	}

	if tunnel.Options.ServerImage == "" {
		tunnel.Options.ServerImage = "quay.io/konveyor/openvpn:latest"
	}
	if tunnel.Options.ServerPort == 0 {
		tunnel.Options.ServerPort = int32(443)
	}
	if tunnel.Options.CACrt == nil {
		ca, serverCrt, serverKey, clientCrt, clientKey, dh, err := GenOpenvpnSSLCrts()
		if err != nil {
			return err
		}
		tunnel.Options.CACrt = ca
		tunnel.Options.ServerCrt = serverCrt
		tunnel.Options.ServerKey = serverKey
		tunnel.Options.ClientCrt = clientCrt
		tunnel.Options.ClientKey = clientKey
		tunnel.Options.RSADHKey = dh
	}

	scheme := runtime.NewScheme()
	if err := securityv1.AddToScheme(scheme); err != nil {
		return err
	}
	if err := appsv1.AddToScheme(scheme); err != nil {
		return err
	}
	if err := appsv1beta1.AddToScheme(scheme); err != nil {
		return err
	}
	if err := corev1.AddToScheme(scheme); err != nil {
		return err
	}
	if err := rbacv1.AddToScheme(scheme); err != nil {
		return err
	}
	dstClient, err := client.New(tunnel.DstConfig, client.Options{Scheme: scheme})
	if err != nil {
		return err
	}
	srcClient, err := client.New(tunnel.SrcConfig, client.Options{Scheme: scheme})
	if err != nil {
		return err
	}

	tunnel.DstClient = dstClient
	tunnel.SrcClient = srcClient

	srcDiscoveryClient, err := dapi.NewDiscoveryClientForConfig(tunnel.SrcConfig)
	if err != nil {
		return err
	}
	srcVersion, err := srcDiscoveryClient.ServerVersion()
	if err != nil {
		return err
	}
	srcMinor, err := strconv.Atoi(strings.Trim(srcVersion.Minor, "+"))
	if err != nil {
		return err
	}

	dstDiscoveryClient, err := dapi.NewDiscoveryClientForConfig(tunnel.DstConfig)
	if err != nil {
		return err
	}
	dstVersion, err := dstDiscoveryClient.ServerVersion()
	if err != nil {
		return err
	}
	dstMinor, err := strconv.Atoi(strings.Trim(dstVersion.Minor, "+"))
	if err != nil {
		return err
	}

	tunnel.DstVersionMinor = dstMinor
	tunnel.SrcVersionMinor = srcMinor

	err = createOpenVPNServer(&tunnel)
	if err != nil {
		return err
	}

	err = createOpenVPNClient(&tunnel)
	if err != nil {
		return err
	}

	return err
}

func createOpenVPNServer(tunnel *Tunnel) error {

	deploymentLabels := map[string]string{}
	deploymentLabels["app"] = serviceName

	namespace := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: tunnel.Options.Namespace,
		},
	}

	var openvpnConf bytes.Buffer
	openvpnConfTemplate, err := template.New("config").Parse(openvpnServerConfTemplate)
	if err != nil {
		return err
	}

	configdata := openvpnConfigData{
		Port: strconv.Itoa(int(tunnel.Options.ServerPort)),
	}

	err = openvpnConfTemplate.Execute(&openvpnConf, configdata)
	if err != nil {
		return err
	}

	configmap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      serviceName,
			Namespace: *&tunnel.Options.Namespace,
		},
		Data: map[string]string{
			"openvpn.conf": openvpnConf.String(),
		},
	}

	openvpnService := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      serviceName,
			Namespace: *&tunnel.Options.Namespace,
		},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{
				{
					Name:       serviceName,
					Protocol:   corev1.ProtocolTCP,
					Port:       tunnel.Options.ServerPort,
					TargetPort: intstr.IntOrString{Type: intstr.Int, IntVal: tunnel.Options.ServerPort},
				},
			},
			Selector: deploymentLabels,
			Type:     corev1.ServiceTypeLoadBalancer,
		},
	}

	proxiedService := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "proxied-cluster",
			Namespace: *&tunnel.Options.Namespace,
		},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{
				{
					Name:       "proxy",
					Protocol:   corev1.ProtocolTCP,
					Port:       int32(8443),
					TargetPort: intstr.IntOrString{Type: intstr.Int, IntVal: 8443},
				},
			},
			Selector: deploymentLabels,
			Type:     corev1.ServiceTypeClusterIP,
		},
	}

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      serviceName,
			Namespace: *&tunnel.Options.Namespace,
		},
		Data: map[string][]byte{
			"ca.crt":     tunnel.Options.CACrt.Bytes(),
			"dh.pem":     tunnel.Options.RSADHKey.Bytes(),
			"server.crt": tunnel.Options.ServerCrt.Bytes(),
			"server.key": tunnel.Options.ServerKey.Bytes(),
		},
	}

	mode := int32(0400)
	volumes := []v1.Volume{
		{
			Name: serviceName,
			VolumeSource: v1.VolumeSource{
				Secret: &v1.SecretVolumeSource{
					DefaultMode: &mode,
					SecretName:  serviceName,
				},
			},
		},
		{
			Name: serviceConfig,
			VolumeSource: v1.VolumeSource{
				ConfigMap: &v1.ConfigMapVolumeSource{
					LocalObjectReference: v1.LocalObjectReference{
						Name: serviceName,
					},
				},
			},
		},
	}

	volumeMounts := []v1.VolumeMount{
		{
			Name:      serviceName,
			MountPath: "/certs",
		},
		{
			Name:      serviceConfig,
			MountPath: "/openvpn.conf",
			SubPath:   "openvpn.conf",
		},
	}

	trueBool := true
	containers := []corev1.Container{
		{
			Name:  serviceName,
			Image: *&tunnel.Options.ServerImage,
			Command: []string{
				"bash",
				"-c",
				"mkdir /tmp/openvpn && openvpn --config /openvpn.conf",
			},
			VolumeMounts:    volumeMounts,
			SecurityContext: &corev1.SecurityContext{Privileged: &trueBool},
		},
		{
			Name:  "socat",
			Image: *&tunnel.Options.ServerImage,
			Command: []string{
				"bash",
				"-c",
				"socat TCP-LISTEN:8443,fork,reuseaddr TCP:192.168.123.6:8443",
			},
		},
	}

	rootUser := int64(0)
	podSpec := corev1.PodSpec{
		ServiceAccountName: serviceName,
		SecurityContext:    &corev1.PodSecurityContext{RunAsUser: &rootUser},
		Containers:         containers,
		Volumes:            volumes,
	}

	deploymentMeta := metav1.ObjectMeta{
		Name:      serviceName,
		Namespace: *&tunnel.Options.Namespace,
	}

	deploymentSelector := &metav1.LabelSelector{
		MatchLabels: deploymentLabels,
	}

	podTemplateSpec := v1.PodTemplateSpec{
		ObjectMeta: metav1.ObjectMeta{
			Labels: deploymentLabels,
		},
		Spec: podSpec,
	}

	err = tunnel.DstClient.Create(context.TODO(), namespace, &client.CreateOptions{})
	if err != nil {
		return err
	}
	err = tunnel.DstClient.Create(context.TODO(), configmap, &client.CreateOptions{})
	if err != nil {
		return err
	}
	err = tunnel.DstClient.Create(context.TODO(), openvpnService, &client.CreateOptions{})
	if err != nil {
		return err
	}
	err = tunnel.DstClient.Create(context.TODO(), proxiedService, &client.CreateOptions{})
	if err != nil {
		return err
	}
	err = createRBAC(tunnel, "dst")
	if err != nil {
		return err
	}
	err = tunnel.DstClient.Create(context.TODO(), secret, &client.CreateOptions{})
	if err != nil {
		return err
	}

	if tunnel.DstVersionMinor < 9 {
		deploymentBeta := &appsv1beta1.Deployment{
			ObjectMeta: deploymentMeta,
			Spec: appsv1beta1.DeploymentSpec{
				Selector: deploymentSelector,
				Template: podTemplateSpec,
			},
		}

		err = tunnel.DstClient.Create(context.TODO(), deploymentBeta, &client.CreateOptions{})
		if err != nil {
			return err
		}
	} else {
		deployment := &appsv1.Deployment{
			ObjectMeta: deploymentMeta,
			Spec: appsv1.DeploymentSpec{
				Selector: deploymentSelector,
				Template: podTemplateSpec,
			},
		}

		err = tunnel.DstClient.Create(context.TODO(), deployment, &client.CreateOptions{})
		if err != nil {
			return err
		}
	}
	return nil
}

func createOpenVPNClient(tunnel *Tunnel) error {
	deploymentLabels := map[string]string{}
	deploymentLabels["app"] = serviceName

	namespace := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: tunnel.Options.Namespace,
		},
	}

	var openvpnConf bytes.Buffer
	openvpnConfTemplate, err := template.New("config").Parse(openvpnClientConfTemplate)
	if err != nil {
		return err
	}

	dstService := &corev1.Service{}

	for i := 0; i < 40; i++ {
		err = tunnel.DstClient.Get(context.TODO(), client.ObjectKey{Name: serviceName, Namespace: tunnel.Options.Namespace}, dstService)
		if err != nil {
			return err
		}
		if len(dstService.Status.LoadBalancer.Ingress) > 0 {
			break
		}
		if i == 39 {
			return fmt.Errorf("Unable to determine ingress address on destination cluster.")
		}
		time.Sleep(time.Second * 3)

	}

	var endpoint string
	if dstService.Status.LoadBalancer.Ingress[0].Hostname != "" {
		endpoint = dstService.Status.LoadBalancer.Ingress[0].Hostname
	} else if dstService.Status.LoadBalancer.Ingress[0].IP != "" {
		endpoint = dstService.Status.LoadBalancer.Ingress[0].IP
	} else {
		return fmt.Errorf("The service endpoint has no hostname or IP address associated with it.")
	}

	var proxy string

	if tunnel.Options.ProxyPort == "" {
		tunnel.Options.ProxyPort = "3128"
	}

	if tunnel.Options.ProxyHost != "" && tunnel.Options.ProxyUser != "" && tunnel.Options.ProxyPass != "" {
		proxy = fmt.Sprintf("http-proxy %s %s /proxy.secret basic", tunnel.Options.ProxyHost, tunnel.Options.ProxyPort)
	} else if tunnel.Options.ProxyHost != "" {
		proxy = fmt.Sprintf("http-proxy %s %s", tunnel.Options.ProxyHost, tunnel.Options.ProxyPort)
	}

	configdata := openvpnConfigData{
		Port:     strconv.Itoa(int(tunnel.Options.ServerPort)),
		CA:       tunnel.Options.CACrt.String(),
		Key:      tunnel.Options.ClientKey.String(),
		Crt:      tunnel.Options.ClientCrt.String(),
		Endpoint: endpoint,
		Proxy:    proxy,
	}

	err = openvpnConfTemplate.Execute(&openvpnConf, configdata)
	if err != nil {
		return err
	}

	configSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      serviceName,
			Namespace: *&tunnel.Options.Namespace,
		},
		Data: map[string][]byte{
			"openvpn.conf": openvpnConf.Bytes(),
		},
	}

	proxySecretString := fmt.Sprintf("%s\n%s", tunnel.Options.ProxyUser, tunnel.Options.ProxyPass)
	proxySecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      proxyConfig,
			Namespace: *&tunnel.Options.Namespace,
		},
		Data: map[string][]byte{
			"proxy.secret": []byte(proxySecretString),
		},
	}

	mode := int32(0400)
	volumes := []v1.Volume{
		{
			Name: serviceConfig,
			VolumeSource: v1.VolumeSource{
				Secret: &v1.SecretVolumeSource{
					DefaultMode: &mode,
					SecretName:  serviceName,
				},
			},
		},
	}

	volumeMounts := []v1.VolumeMount{
		{
			Name:      serviceConfig,
			MountPath: "/openvpn.conf",
			SubPath:   "openvpn.conf",
		},
	}

	if tunnel.Options.ProxyUser != "" && tunnel.Options.ProxyPass != "" {
		proxyVolume := v1.Volume{
			Name: proxyConfig,
			VolumeSource: v1.VolumeSource{
				Secret: &v1.SecretVolumeSource{
					DefaultMode: &mode,
					SecretName:  proxyConfig,
				},
			},
		}

		proxyVolumeMount := v1.VolumeMount{
			Name:      proxyConfig,
			MountPath: "/proxy.secret",
			SubPath:   "proxy.secret",
		}

		volumes = append(volumes, proxyVolume)
		volumeMounts = append(volumeMounts, proxyVolumeMount)
	}

	trueBool := true
	containers := []corev1.Container{
		{
			Name:  serviceName,
			Image: *&tunnel.Options.ServerImage,
			Command: []string{
				"bash",
				"-c",
				"mkdir /tmp/openvpn && openvpn --config /openvpn.conf",
			},
			VolumeMounts:    volumeMounts,
			SecurityContext: &corev1.SecurityContext{Privileged: &trueBool},
		},
		{
			Name:  "socat",
			Image: *&tunnel.Options.ServerImage,
			Command: []string{
				"bash",
				"-c",
				"socat TCP-LISTEN:8443,fork,reuseaddr TCP:${KUBERNETES_SERVICE_HOST}:${KUBERNETES_SERVICE_PORT_HTTPS}",
			},
		},
	}

	rootUser := int64(0)
	podSpec := corev1.PodSpec{
		ServiceAccountName: serviceName,
		SecurityContext:    &corev1.PodSecurityContext{RunAsUser: &rootUser},
		Containers:         containers,
		Volumes:            volumes,
	}

	deploymentMeta := metav1.ObjectMeta{
		Name:      serviceName,
		Namespace: *&tunnel.Options.Namespace,
	}

	deploymentSelector := &metav1.LabelSelector{
		MatchLabels: deploymentLabels,
	}

	podTemplateSpec := v1.PodTemplateSpec{
		ObjectMeta: metav1.ObjectMeta{
			Labels: deploymentLabels,
		},
		Spec: podSpec,
	}

	err = tunnel.SrcClient.Create(context.TODO(), namespace, &client.CreateOptions{})
	if err != nil {
		return err
	}
	err = createRBAC(tunnel, "src")
	if err != nil {
		return err
	}
	err = tunnel.SrcClient.Create(context.TODO(), configSecret, &client.CreateOptions{})
	if err != nil {
		return err
	}
	if tunnel.Options.ProxyUser != "" && tunnel.Options.ProxyPass != "" {
		err = tunnel.SrcClient.Create(context.TODO(), proxySecret, &client.CreateOptions{})
		if err != nil {
			return err
		}
	}
	if tunnel.SrcVersionMinor < 9 {
		deploymentBeta := &appsv1beta1.Deployment{
			ObjectMeta: deploymentMeta,
			Spec: appsv1beta1.DeploymentSpec{
				Selector: deploymentSelector,
				Template: podTemplateSpec,
			},
		}

		err = tunnel.SrcClient.Create(context.TODO(), deploymentBeta, &client.CreateOptions{})
		if err != nil {
			return err
		}
	} else {
		deployment := &appsv1.Deployment{
			ObjectMeta: deploymentMeta,
			Spec: appsv1.DeploymentSpec{
				Selector: deploymentSelector,
				Template: podTemplateSpec,
			},
		}

		err = tunnel.SrcClient.Create(context.TODO(), deployment, &client.CreateOptions{})
		if err != nil {
			return err
		}
	}

	return nil
}

func createRBAC(tunnel *Tunnel, cluster string) error {
	var c client.Client
	var minor int

	switch cluster {
	case "src":
		c = tunnel.SrcClient
		minor = tunnel.SrcVersionMinor
	case "dst":
		c = tunnel.DstClient
		minor = tunnel.DstVersionMinor
	default:
		return fmt.Errorf("Cannot create RBAC rules for unknown cluster %s", cluster)
	}

	serviceAccount := &v1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      serviceName,
			Namespace: *&tunnel.Options.Namespace,
		},
	}
	err := c.Create(context.TODO(), serviceAccount, &client.CreateOptions{})
	if err != nil {
		return err
	}

	if minor <= 11 {
		scc := &securityv1.SecurityContextConstraints{
			ObjectMeta: metav1.ObjectMeta{
				Name: tunnel.Options.Namespace,
			},
			AllowPrivilegedContainer: true,
			AllowedCapabilities:      []corev1.Capability{"*"},
			AllowHostDirVolumePlugin: true,
			Volumes:                  []securityv1.FSType{"*"},
			AllowHostNetwork:         true,
			AllowHostPorts:           true,
			AllowHostPID:             true,
			AllowHostIPC:             true,
			SELinuxContext: securityv1.SELinuxContextStrategyOptions{
				Type: "RunAsAny",
			},
			RunAsUser: securityv1.RunAsUserStrategyOptions{
				Type: "RunAsAny",
			},
			SupplementalGroups: securityv1.SupplementalGroupsStrategyOptions{
				Type: "RunAsAny",
			},
			FSGroup: securityv1.FSGroupStrategyOptions{
				Type: "RunAsAny",
			},
			ReadOnlyRootFilesystem: false,
			Users:                  []string{"system:serviceaccount:" + tunnel.Options.Namespace + ":openvpn"},
			SeccompProfiles:        []string{"*"},
		}

		err = c.Create(context.TODO(), scc, &client.CreateOptions{})
		if err != nil && !k8serrors.IsAlreadyExists(err) {
			return err
		}

	} else {
		role := &rbacv1.Role{
			ObjectMeta: metav1.ObjectMeta{
				Name:      serviceName,
				Namespace: *&tunnel.Options.Namespace,
			},
			Rules: []rbacv1.PolicyRule{
				{
					Verbs:           []string{"use"},
					APIGroups:       []string{"security.openshift.io"},
					Resources:       []string{"securitycontextconstraints"},
					ResourceNames:   []string{"privileged"},
					NonResourceURLs: []string{},
				},
			},
		}

		roleBinding := &rbacv1.RoleBinding{
			ObjectMeta: metav1.ObjectMeta{
				Name:      serviceName,
				Namespace: *&tunnel.Options.Namespace,
			},
			Subjects: []rbacv1.Subject{
				{
					Kind:      "ServiceAccount",
					Name:      serviceName,
					Namespace: *&tunnel.Options.Namespace,
				},
			},
			RoleRef: rbacv1.RoleRef{
				APIGroup: "rbac.authorization.k8s.io",
				Kind:     "Role",
				Name:     serviceName,
			},
		}

		err = c.Create(context.TODO(), role, &client.CreateOptions{})
		if err != nil {
			return err
		}
		err = c.Create(context.TODO(), roleBinding, &client.CreateOptions{})
		if err != nil {
			return err
		}
	}

	return nil
}

func GenOpenvpnSSLCrts() (*bytes.Buffer, *bytes.Buffer, *bytes.Buffer, *bytes.Buffer, *bytes.Buffer, *bytes.Buffer, error) {

	subj := pkix.Name{
		Country:            []string{"US"},
		Province:           []string{"NC"},
		Locality:           []string{"RDU"},
		Organization:       []string{"Engineering"},
		OrganizationalUnit: []string{"Crane"},
	}

	caSubj := subj
	caSubj.CommonName = "CA"
	caCrtTemp := x509.Certificate{
		SerialNumber:          big.NewInt(2021),
		Subject:               caSubj,
		NotBefore:             time.Now(),
		NotAfter:              time.Now().AddDate(10, 0, 0),
		IsCA:                  true,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth},
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		BasicConstraintsValid: true,
	}

	ca, caKeyBytes, err := createCACrtKeyPair(caCrtTemp)
	if err != nil {
		return nil, nil, nil, nil, nil, nil, err
	}

	serverSubj := subj
	serverSubj.CommonName = "Server"

	serverCrtTemp := x509.Certificate{
		SerialNumber: big.NewInt(2022),
		Subject:      serverSubj,
		IPAddresses:  []net.IP{net.IPv4(127, 0, 0, 1), net.IPv6loopback},
		NotBefore:    time.Now(),
		NotAfter:     time.Now().AddDate(10, 0, 0),
		SubjectKeyId: []byte{1, 2, 3, 4, 6},
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth},
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
	}

	serverCrt, serverKey, err := createCrtKeyPair(serverCrtTemp, caCrtTemp, caKeyBytes)
	if err != nil {
		return nil, nil, nil, nil, nil, nil, err
	}

	clientSubj := subj
	clientSubj.CommonName = "Client"

	clientCrtTemp := x509.Certificate{
		SerialNumber: big.NewInt(2023),
		Subject:      clientSubj,
		IPAddresses:  []net.IP{net.IPv4(127, 0, 0, 1), net.IPv6loopback},
		NotBefore:    time.Now(),
		NotAfter:     time.Now().AddDate(10, 0, 0),
		SubjectKeyId: []byte{1, 2, 3, 4, 6},
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth},
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
	}

	clientCrt, clientKey, err := createCrtKeyPair(clientCrtTemp, caCrtTemp, caKeyBytes)
	if err != nil {
		return nil, nil, nil, nil, nil, nil, err
	}

	var cb dhparam.GeneratorCallback

	dhCrtTemp, err := dhparam.Generate(keySize, 2, cb)
	if err != nil {
		return nil, nil, nil, nil, nil, nil, err
	}

	dhBytes, err := dhCrtTemp.ToPEM()
	if err != nil {
		return nil, nil, nil, nil, nil, nil, err
	}

	dhCrt := bytes.NewBuffer(dhBytes)

	return ca, serverCrt, serverKey, clientCrt, clientKey, dhCrt, nil
}

func createCACrtKeyPair(crtTemp x509.Certificate) (*bytes.Buffer, *rsa.PrivateKey, error) {
	keyBytes, err := rsa.GenerateKey(rand.Reader, keySize)
	if err != nil {
		return nil, nil, err
	}

	crtBytes, err := x509.CreateCertificate(
		rand.Reader,
		&crtTemp,
		&crtTemp,
		&keyBytes.PublicKey,
		keyBytes,
	)
	if err != nil {
		return nil, nil, err
	}

	crt := new(bytes.Buffer)
	err = pem.Encode(crt, &pem.Block{
		Type:  "CERTIFICATE",
		Bytes: crtBytes,
	})
	if err != nil {
		return nil, nil, err
	}
	key := new(bytes.Buffer)
	err = pem.Encode(key, &pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(keyBytes),
	})
	if err != nil {
		return nil, nil, err
	}

	return crt, keyBytes, nil
}

func createCrtKeyPair(crtTemp x509.Certificate, caCrtTemp x509.Certificate, caKeyBytes *rsa.PrivateKey) (*bytes.Buffer, *bytes.Buffer, error) {
	keyBytes, err := rsa.GenerateKey(rand.Reader, keySize)
	if err != nil {
		return nil, nil, err
	}

	crtBytes, err := x509.CreateCertificate(
		rand.Reader,
		&crtTemp,
		&caCrtTemp,
		&keyBytes.PublicKey,
		caKeyBytes,
	)
	if err != nil {
		return nil, nil, err
	}

	crt := new(bytes.Buffer)
	err = pem.Encode(crt, &pem.Block{
		Type:  "CERTIFICATE",
		Bytes: crtBytes,
	})
	if err != nil {
		return nil, nil, err
	}
	key := new(bytes.Buffer)
	err = pem.Encode(key, &pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(keyBytes),
	})
	if err != nil {
		return nil, nil, err
	}

	return crt, key, nil
}
