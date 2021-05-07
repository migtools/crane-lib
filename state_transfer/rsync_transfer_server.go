package state_transfer

import (
	"bytes"
	"context"
	"strconv"
	"text/template"
	"time"

	random "math/rand"

	v1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	rsyncServerConfTemplate = `syslog facility = local7
read only = no
list = yes
log file = /dev/stdout
max verbosity = 4
hosts allow = ::1, 127.0.0.1, localhost
[mnt]
    comment = mnt
    path = /mnt
    use chroot = no
    munge symlinks = no
    list = yes
    hosts allow = ::1, 127.0.0.1, localhost
    read only = false
    auth users = {{ . }} 
    secrets file = /etc/rsync-secret/rsyncd.secrets
`
)

func (r *RsyncTransfer) createTransferServerResources(c client.Client) error {
	r.SetPort(rsyncPort)
	r.SetUsername(rsyncUser)

	err := createRsyncServerConfig(c, r)
	if err != nil {
		return err
	}

	err = createRsyncServerSecret(c, r)
	if err != nil {
		return err
	}

	return nil
}

func createRsyncServerConfig(c client.Client, r *RsyncTransfer) error {
	var rsyncConf bytes.Buffer
	rsyncConfTemplate, err := template.New("config").Parse(rsyncServerConfTemplate)
	if err != nil {
		return err
	}

	err = rsyncConfTemplate.Execute(&rsyncConf, rsyncUser)
	if err != nil {
		return err
	}

	rsyncConfigMap := &v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: r.PVC().Namespace,
			Name:      rsyncConfigPrefix + r.PVC().Name,
			Labels:    labels,
		},
		Data: map[string]string{
			"rsyncd.conf": string(rsyncConf.Bytes()),
		},
	}

	return c.Create(context.TODO(), rsyncConfigMap, &client.CreateOptions{})
}

func createRsyncServerSecret(c client.Client, r *RsyncTransfer) error {
	var letters = []byte("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ")
	random.Seed(time.Now().UnixNano())
	password := make([]byte, 24)
	for i := range password {
		password[i] = letters[random.Intn(len(letters))]
	}
	r.SetPassword(string(password))

	rsyncSecret := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: r.PVC().Namespace,
			Name:      rsyncSecretPrefix + r.PVC().Name,
			Labels:    labels,
		},
		Data: map[string][]byte{
			"credentials": []byte(r.Username() + ":" + r.Password()),
		},
	}
	return c.Create(context.TODO(), rsyncSecret, &client.CreateOptions{})
}

func (r *RsyncTransfer) createTransferServer(c client.Client) error {
	deploymentLabels := labels
	deploymentLabels["pvc"] = r.PVC().Name
	containers := []v1.Container{
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
			Ports: []v1.ContainerPort{
				{
					Name:          "rsyncd",
					Protocol:      v1.ProtocolTCP,
					ContainerPort: rsyncPort,
				},
			},
			VolumeMounts: []v1.VolumeMount{
				{
					Name:      "mnt",
					MountPath: "/mnt",
				},
				{
					Name:      rsyncConfigPrefix + r.PVC().Name,
					MountPath: "/etc/rsyncd.conf",
					SubPath:   "rsyncd.conf",
				},
				{
					Name:      rsyncSecretPrefix + r.PVC().Name,
					MountPath: "/etc/rsync-secret",
				},
			},
		},
	}

	for _, container := range r.Transport().ServerContainers() {
		containers = append(containers, container)
	}

	mode := int32(0600)

	volumes := []v1.Volume{
		{
			Name: "mnt",
			VolumeSource: v1.VolumeSource{
				PersistentVolumeClaim: &v1.PersistentVolumeClaimVolumeSource{
					ClaimName: r.PVC().Name,
				},
			},
		},
		{
			Name: rsyncConfigPrefix + r.PVC().Name,
			VolumeSource: v1.VolumeSource{
				ConfigMap: &v1.ConfigMapVolumeSource{
					LocalObjectReference: v1.LocalObjectReference{
						Name: rsyncConfigPrefix + r.PVC().Name,
					},
				},
			},
		},
		{
			Name: rsyncSecretPrefix + r.PVC().Name,
			VolumeSource: v1.VolumeSource{
				Secret: &v1.SecretVolumeSource{
					SecretName:  rsyncSecretPrefix + r.PVC().Name,
					DefaultMode: &mode,
					Items: []v1.KeyToPath{
						{
							Key:  "credentials",
							Path: "rsyncd.secrets",
						},
					},
				},
			},
		},
	}

	for _, volume := range r.Transport().ServerVolumes() {
		volumes = append(volumes, volume)
	}

	server := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      r.PVC().Name,
			Namespace: r.PVC().Namespace,
			Labels:    deploymentLabels,
		},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: deploymentLabels,
			},
			Template: v1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: deploymentLabels,
				},
				Spec: v1.PodSpec{
					Containers: containers,
					Volumes:    volumes,
				},
			},
		},
	}

	return c.Create(context.TODO(), server, &client.CreateOptions{})
}
