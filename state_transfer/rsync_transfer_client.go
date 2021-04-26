package state_transfer

import (
	"context"
	"strconv"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const ()

func (r *RsyncTransfer) createTransferClientResources(c client.Client) error {
	// no resource are created for rsync client side
	return nil
}

func (r *RsyncTransfer) createTransferClient(c client.Client) error {
	podLabels := labels
	podLabels["pvc"] = r.GetPVC().Name
	containers := []v1.Container{
		{
			Name:  "rsync",
			Image: rsyncImage,
			Command: []string{
				"/usr/bin/rsync",
				"-vvvv",
				"--delete",
				"--recursive",
				"--compress",
				"rsync://" + r.GetUsername() + "@localhost:" + strconv.Itoa(int(r.GetTransport().GetTransportPort())) + "/mnt",
				"/mnt/",
			},
			Env: []v1.EnvVar{
				{
					Name:  "RSYNC_PASSWORD",
					Value: r.GetPassword(),
				},
			},

			VolumeMounts: []v1.VolumeMount{
				{
					Name:      "mnt",
					MountPath: "/mnt",
				},
			},
		},
	}

	for _, container := range r.GetTransport().GetClientContainers() {
		containers = append(containers, container)
	}

	volumes := []v1.Volume{
		{
			Name: "mnt",
			VolumeSource: v1.VolumeSource{
				PersistentVolumeClaim: &v1.PersistentVolumeClaimVolumeSource{
					ClaimName: r.PVC.Name,
				},
			},
		},
	}

	for _, volume := range r.GetTransport().GetClientVolumes() {
		volumes = append(volumes, volume)
	}

	pod := v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      r.GetPVC().Name,
			Namespace: r.GetPVC().Namespace,
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
