package rclone

import (
	"bytes"
	"context"
	"strconv"
	"text/template"

	"github.com/konveyor/crane-lib/state_transfer/transfer"
	"github.com/konveyor/crane-lib/state_transfer/transport"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	rcloneClientConfTemplate = `[remote]
type = http
url = http://{{ .username }}:{{ .password }}@{{ .hostname }}:{{ .port }}
`
)

func (r *RcloneTransfer) CreateClient(c client.Client) error {
	pvc := r.pvcList[0]

	err := createRcloneClientResources(c, r, pvc)
	if err != nil {
		return err
	}

	_, err = transport.CreateClient(r.Transport(), c, r.Endpoint())
	if err != nil {
		return err
	}

	err = createRcloneClient(c, r, pvc)
	if err != nil {
		return err
	}

	return nil
}

func createRcloneClientResources(c client.Client, r *RcloneTransfer, pvc transfer.PVC) error {
	err := createRcloneClientConfig(c, r, pvc)
	if err != nil {
		return err
	}

	return nil
}

func createRcloneClientConfig(c client.Client, r *RcloneTransfer, pvc transfer.PVC) error {
	var rcloneConf bytes.Buffer
	rcloneConfTemplate, err := template.New("config").Parse(rcloneClientConfTemplate)
	if err != nil {
		return err
	}

	coordinates := map[string]string{
		"username": r.Username(),
		"password": r.Password(),
		"hostname": transfer.ConnectionHostname(r),
		"port":     strconv.Itoa(int(transfer.ConnectionPort(r))),
	}

	err = rcloneConfTemplate.Execute(&rcloneConf, coordinates)
	if err != nil {
		return err
	}

	rcloneConfigMap := &v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: pvc.Source().Claim().Namespace,
			Name:      rcloneConfigPrefix + pvc.Source().ValidatedName(),
			Labels:    r.Endpoint().Labels(),
		},
		Data: map[string]string{
			"rclone.conf": string(rcloneConf.Bytes()),
		},
	}

	return c.Create(context.TODO(), rcloneConfigMap, &client.CreateOptions{})
}

func createRcloneClient(c client.Client, r *RcloneTransfer, pvc transfer.PVC) error {
	podLabels := r.Endpoint().Labels()
	podLabels["pvc"] = pvc.Source().ValidatedName()

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
				"--http-headers",
				"Host," + r.Endpoint().Hostname(),
			},
			VolumeMounts: []v1.VolumeMount{
				{
					Name:      "mnt",
					MountPath: "/mnt",
				},
				{
					Name:      rcloneConfigPrefix + pvc.Source().ValidatedName(),
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
					ClaimName: pvc.Source().Claim().Name,
				},
			},
		},
		{
			Name: rcloneConfigPrefix + pvc.Source().ValidatedName(),
			VolumeSource: v1.VolumeSource{
				ConfigMap: &v1.ConfigMapVolumeSource{
					LocalObjectReference: v1.LocalObjectReference{
						Name: rcloneConfigPrefix + pvc.Source().ValidatedName(),
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
			Name:      pvc.Source().Claim().Name,
			Namespace: pvc.Source().Claim().Namespace,
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
