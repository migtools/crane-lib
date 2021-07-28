package rsync

import (
	"bytes"
	"context"
	"fmt"
	"github.com/konveyor/crane-lib/state_transfer/transfer"
	"strconv"
	"text/template"
	"time"

	"github.com/konveyor/crane-lib/state_transfer/endpoint"
	"github.com/konveyor/crane-lib/state_transfer/transport"

	random "math/rand"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	rsyncServerConfTemplate = `syslog facility = local7
read only = no
list = yes
log file = /dev/stdout
max verbosity = 4
[mnt]
    comment = mnt
    path = /mnt
    use chroot = no
    munge symlinks = no
    list = yes
    read only = false
    auth users = {{ . }} 
    secrets file = /etc/rsync-secret/rsyncd.secrets
`
)

func (r *RsyncTransfer) CreateServer(c client.Client) error {
	destNs := r.pvcList.GetDestinationNamespaces()[0]
	err := createRsyncServerResources(c, r, destNs)
	if err != nil {
		return err
	}

	_, err = transport.CreateServer(r.Transport(), c, r.Endpoint())
	if err != nil {
		return err
	}

	err = createRsyncServer(c, r, destNs)
	if err != nil {
		return err
	}

	_, err = endpoint.Create(r.Endpoint(), c)
	if err != nil {
		return err
	}

	return nil
}

func (r *RsyncTransfer) IsServerHealthy(c client.Client) (bool, error) {
	return transfer.IsPodHealthy(c, client.ObjectKey{Namespace: r.pvcList.GetDestinationNamespaces()[0], Name: "rsync-server"})
}

func createRsyncServerResources(c client.Client, r *RsyncTransfer, ns string) error {
	r.username = rsyncUser
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
	rsyncConfTemplate, err := template.New("config").Parse(rsyncServerConfTemplate)
	if err != nil {
		return err
	}

	err = rsyncConfTemplate.Execute(&rsyncConf, rsyncUser)
	if err != nil {
		return err
	}

	rsyncConfigMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: ns,
			Name:      rsyncConfigPrefix,
			Labels:    r.transferOptions().DestinationPodMeta.Labels,
		},
		Data: map[string]string{
			"rsyncd.conf": rsyncConf.String(),
		},
	}

	return c.Create(context.TODO(), rsyncConfigMap, &client.CreateOptions{})
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
			Name:      rsyncSecretPrefix,
			Labels:    r.transferOptions().DestinationPodMeta.Labels,
		},
		Data: map[string][]byte{
			"credentials": []byte(r.Username() + ":" + r.Password()),
		},
	}
	return c.Create(context.TODO(), rsyncSecret, &client.CreateOptions{})
}

func createRsyncServer(c client.Client, r *RsyncTransfer, ns string) error {
	transferOptions := r.transferOptions()
	podLabels := transferOptions.DestinationPodMeta.Labels

	volumeMounts := []corev1.VolumeMount{}
	configVolumeMounts := []corev1.VolumeMount{
		{
			Name:      rsyncConfigPrefix,
			MountPath: "/etc/rsyncd.conf",
			SubPath:   "rsyncd.conf",
		},
		{
			Name:      rsyncSecretPrefix,
			MountPath: "/etc/rsync-secret",
		},
	}
	pvcVolumeMounts := []corev1.VolumeMount{}
	for _, pvc := range r.pvcList.InDestinationNamespace(ns) {
		pvcVolumeMounts = append(
			pvcVolumeMounts,
			corev1.VolumeMount{
				Name:      pvc.Destination().LabelSafeName(),
				MountPath: fmt.Sprintf("/mnt/%s", pvc.Destination().LabelSafeName()),
			})
	}
	volumeMounts = append(volumeMounts, configVolumeMounts...)
	volumeMounts = append(volumeMounts, pvcVolumeMounts...)
	containers := []corev1.Container{
		{
			Name:  "rsync",
			Image: rsyncImage,
			Command: []string{
				"/usr/bin/rsync",
				"--daemon",
				"--no-detach",
				"--port=" + strconv.Itoa(int(rsyncPort)),
				"-vvv",
			},
			Ports: []corev1.ContainerPort{
				{
					Name:          "rsyncd",
					Protocol:      corev1.ProtocolTCP,
					ContainerPort: rsyncPort,
				},
			},
			VolumeMounts: volumeMounts,
		},
	}

	containers = append(containers, r.Transport().ServerContainers()...)

	mode := int32(0600)

	configVolumes := []corev1.Volume{
		{
			Name: rsyncConfigPrefix,
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: rsyncConfigPrefix,
					},
				},
			},
		},
		{
			Name: rsyncSecretPrefix,
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName:  rsyncSecretPrefix,
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

	server := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "rsync-server",
			Namespace: ns,
			Labels:    podLabels,
		},
		Spec: corev1.PodSpec{
			Containers: containers,
			Volumes:    volumes,
		},
	}

	return c.Create(context.TODO(), server, &client.CreateOptions{})
}
