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
	rcloneClientConfTemplate = `[remote]
type = http
url = http://{{ .username }}:{{ .password }}@localhost:{{ .port }}
`
)

func (r *RcloneTransfer) createTransferClientResources(c client.Client) error {
	err := createRcloneClientConfig(c, r)
	if err != nil {
		return err
	}

	return nil
}

func createRcloneClientConfig(c client.Client, r *RcloneTransfer) error {
	var rcloneConf bytes.Buffer
	rcloneConfTemplate, err := template.New("config").Parse(rcloneClientConfTemplate)
	if err != nil {
		return err
	}

	coordinates := map[string]string{
		"username": r.Username(),
		"password": r.Password(),
		"hostname": r.Endpoint().Hostname(),
		"port":     strconv.Itoa(int(r.Transport().Port())),
	}

	err = rcloneConfTemplate.Execute(&rcloneConf, coordinates)
	if err != nil {
		return err
	}

	rcloneConfigMap := &v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: r.PVC().Namespace,
			Name:      rcloneConfigPrefix + r.PVC().Name,
			Labels:    labels,
		},
		Data: map[string]string{
			"rclone.conf": string(rcloneConf.Bytes()),
		},
	}

	return c.Create(context.TODO(), rcloneConfigMap, &client.CreateOptions{})
}

func (r *RcloneTransfer) createTransferClient(c client.Client) error {
	podLabels := labels
	podLabels["pvc"] = r.PVC().Name
	containers := []v1.Container{
		{
			Name:  "rclone",
			Image: rcloneImage,
			Command: []string{
				"/usr/bin/rclone",
				"sync",
				"remote:/",
				"/mnt",
				"--config",
				"/etc/rclone.conf",
			},
			VolumeMounts: []v1.VolumeMount{
				{
					Name:      "mnt",
					MountPath: "/mnt",
				},
				{
					Name:      rcloneConfigPrefix + r.PVC().Name,
					MountPath: "/etc/rclone.conf",
					SubPath:   "rclone.conf",
				},
			},
		},
	}

	for _, container := range r.Transport().ClientContainers() {
		containers = append(containers, container)
	}

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
			Name: rcloneConfigPrefix + r.PVC().Name,
			VolumeSource: v1.VolumeSource{
				ConfigMap: &v1.ConfigMapVolumeSource{
					LocalObjectReference: v1.LocalObjectReference{
						Name: rcloneConfigPrefix + r.PVC().Name,
					},
				},
			},
		},
	}

	for _, volume := range r.Transport().ClientVolumes() {
		volumes = append(volumes, volume)
	}

	pod := v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      r.PVC().Name,
			Namespace: r.PVC().Namespace,
			Labels:    podLabels,
		},
		Spec: v1.PodSpec{
			Containers:    containers,
			Volumes:       volumes,
			RestartPolicy: v1.RestartPolicyOnFailure,
		},
	}

	return c.Create(context.TODO(), &pod, &client.CreateOptions{})
}
