package rsync

import (
	"bytes"
	"context"
	"fmt"
	random "math/rand"
	"text/template"
	"time"

	"github.com/konveyor/crane-lib/state_transfer/transfer"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	errorsutil "k8s.io/apimachinery/pkg/util/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	rsyncServerConfTemplate = `syslog facility = local7
read only = no
list = yes
log file = /dev/stdout
max verbosity = 4
auth users = {{ $.Username }}
hosts allow = ::1, 127.0.0.1, localhost
{{ if $.RunAsRoot }}
uid = root
gid = root
{{ end }}
{{ if $.EnableChroot }}
use chroot = yes
{{ else }}
use chroot = no
{{ end }}
{{ if .MungeSymlinks }}
munge symlinks = yes
{{ else }}
munge symlinks = no
{{ end }}
{{ range $i, $pvc := .PVCPairList }}
[{{ $pvc.Destination.LabelSafeName }}]
    comment = archive for {{ $pvc.Destination.Claim.Namespace }}/{{ $pvc.Destination.Claim.Name }}
    path = /mnt/{{ $pvc.Destination.Claim.Namespace }}/{{ $pvc.Destination.LabelSafeName }}
    list = yes
    read only = false
    auth users = {{ $.Username }}
    secrets file = /etc/rsync-secret/rsyncd.secrets
{{ end }}
`
)

type rsyncConfigData struct {
	Username      string
	PVCPairList   transfer.PVCPairList
	RunAsRoot     bool
	EnableChroot  bool
	MungeSymlinks bool
}

func (r *RsyncTransfer) CreateServer(c client.Client) error {
	destNs := r.pvcList.GetDestinationNamespaces()[0]
	errs := []error{}

	err := createRsyncServerResources(c, r, destNs)
	errs = append(errs, err)

	err = createRsyncServer(c, r, destNs)
	errs = append(errs, err)

	return errorsutil.NewAggregate(errs)
}

func (r *RsyncTransfer) IsServerHealthy(c client.Client) (bool, error) {
	return transfer.IsPodHealthy(c, client.ObjectKey{Namespace: r.pvcList.GetDestinationNamespaces()[0], Name: "rsync-server"})
}

func createRsyncServerResources(c client.Client, r *RsyncTransfer, ns string) error {
	r.username = defaultRsyncUser
	r.port = rsyncPort

	err := createRsyncServerConfig(c, r, ns)
	if err != nil {
		return err
	}

	err = createRsyncServerSecret(c, r, ns)
	if err != nil {
		return err
	}

	return nil
}

func createRsyncServerConfig(c client.Client, r *RsyncTransfer, ns string) error {
	var rsyncConf bytes.Buffer
	runRsyncAsRoot := false
	runRsyncAsPrivileged := false
	for _, ops := range r.options.DestContainerMutations {
		if ops.SecurityContext().RunAsUser != nil && *ops.SecurityContext().RunAsUser == int64(0) {
			// running rsync as root
			runRsyncAsRoot = true
		}
		if ops.SecurityContext().Privileged != nil && *ops.SecurityContext().Privileged {
			// running rsync as privileged
			runRsyncAsPrivileged = true
		}
	}
	rsyncConfTemplate, err := template.New("config").Parse(rsyncServerConfTemplate)
	if err != nil {
		return err
	}

	configdata := rsyncConfigData{
		Username:      r.options.username,
		PVCPairList:   r.pvcList.InDestinationNamespace(ns),
		RunAsRoot:     runRsyncAsRoot || runRsyncAsPrivileged,
		EnableChroot:  runRsyncAsPrivileged,
		MungeSymlinks: r.options.mungeSymlinks,
	}

	err = rsyncConfTemplate.Execute(&rsyncConf, configdata)
	if err != nil {
		return err
	}

	rsyncConfigMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: ns,
			Name:      defaultRsyncServerConfig,
			Labels:    r.transferOptions().DestinationPodMeta.Labels,
		},
		Data: map[string]string{
			"rsyncd.conf": rsyncConf.String(),
		},
	}
	err = c.Create(context.TODO(), rsyncConfigMap, &client.CreateOptions{})
	if err != nil && !k8serrors.IsAlreadyExists(err) {
		return err
	}
	return nil
}

func createRsyncServerSecret(c client.Client, r *RsyncTransfer, ns string) error {
	var letters = []byte("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ")
	random.Seed(time.Now().UnixNano())
	password := make([]byte, 24)
	for i := range password {
		password[i] = letters[random.Intn(len(letters))]
	}
	r.password = string(password)

	rsyncSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: ns,
			Name:      defaultRsyncServerSecret,
			Labels:    r.transferOptions().DestinationPodMeta.Labels,
		},
		Data: map[string][]byte{
			"credentials": []byte(r.transferOptions().username + ":" + r.transferOptions().password),
		},
	}
	err := c.Create(context.TODO(), rsyncSecret, &client.CreateOptions{})
	if err != nil && !k8serrors.IsAlreadyExists(err) {
		return err
	}

	return nil
}

func createRsyncServer(c client.Client, r *RsyncTransfer, ns string) error {
	transferOptions := r.transferOptions()
	podLabels := transferOptions.DestinationPodMeta.Labels
	volumeMounts := []corev1.VolumeMount{}
	configVolumeMounts := []corev1.VolumeMount{
		{
			Name:      defaultRsyncServerConfig,
			MountPath: "/etc/rsyncd.conf",
			SubPath:   "rsyncd.conf",
		},
		{
			Name:      defaultRsyncServerSecret,
			MountPath: "/etc/rsync-secret",
		},
	}
	pvcVolumeMounts := []corev1.VolumeMount{}
	for _, pvc := range r.pvcList.InDestinationNamespace(ns) {
		pvcVolumeMounts = append(
			pvcVolumeMounts,
			corev1.VolumeMount{
				Name:      pvc.Destination().LabelSafeName(),
				MountPath: fmt.Sprintf("/mnt/%s/%s", pvc.Destination().Claim().Namespace, pvc.Destination().LabelSafeName()),
			})
	}
	volumeMounts = append(volumeMounts, configVolumeMounts...)
	volumeMounts = append(volumeMounts, pvcVolumeMounts...)
	containers := []corev1.Container{
		{
			Name:  RsyncContainer,
			Image: r.getRsyncServerImage(),
			Command: []string{
				"/usr/bin/rsync",
				"--daemon",
				"--no-detach",
				fmt.Sprintf("--port=%d", r.Transport().ExposedPort()),
				"-vvv",
			},
			Ports: []corev1.ContainerPort{
				{
					Name:          "rsyncd",
					Protocol:      corev1.ProtocolTCP,
					ContainerPort: r.Transport().ExposedPort(),
				},
			},
			VolumeMounts: volumeMounts,
		},
	}

	containers = append(containers, r.transport.ServerContainers()...)
	// apply container mutations
	for i := range containers {
		c := &containers[i]
		applyContainerMutations(c, r.options.DestContainerMutations)
	}

	mode := int32(0600)

	configVolumes := []corev1.Volume{
		{
			Name: defaultRsyncServerConfig,
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: defaultRsyncServerConfig,
					},
				},
			},
		},
		{
			Name: defaultRsyncServerSecret,
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName:  defaultRsyncServerSecret,
					DefaultMode: &mode,
					Items: []corev1.KeyToPath{
						{
							Key:  "credentials",
							Path: "rsyncd.secrets",
						},
					},
				},
			},
		},
	}
	pvcVolumes := []corev1.Volume{}
	for _, pvc := range r.pvcList.InDestinationNamespace(ns) {
		pvcVolumes = append(
			pvcVolumes,
			corev1.Volume{
				Name: pvc.Destination().LabelSafeName(),
				VolumeSource: corev1.VolumeSource{
					PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
						ClaimName: pvc.Destination().Claim().Name,
					},
				},
			},
		)
	}
	volumes := append(pvcVolumes, configVolumes...)
	volumes = append(volumes, r.Transport().ServerVolumes()...)

	podSpec := corev1.PodSpec{
		Containers: containers,
		Volumes:    volumes,
	}

	applyPodMutations(&podSpec, r.options.DestinationPodMutations)

	server := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "rsync-server",
			Namespace: ns,
			Labels:    podLabels,
		},
		Spec: podSpec,
	}

	err := c.Create(context.TODO(), server, &client.CreateOptions{})
	if err != nil && !k8serrors.IsAlreadyExists(err) {
		return err
	}
	return err
}
