package rclone

import (
	"context"
	random "math/rand"
	"time"

	v1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/konveyor/crane-lib/state_transfer/endpoint"
	"github.com/konveyor/crane-lib/state_transfer/transfer"
)

const (
	rcloneServerConf = `[mnt]
type = local
`
)

func (r *RcloneTransfer) CreateServer(c client.Client) error {
	pvc := r.pvcList[0]

	err := createRcloneServerResources(c, r, pvc)
	if err != nil {
		return err
	}

	err = createRcloneServer(c, r, pvc)
	if err != nil {
		return err
	}

	_, err = endpoint.Create(r.Endpoint(), c)

	return err
}

func (r *RcloneTransfer) IsServerHealthy(c client.Client) (bool, error) {
	deploymentLabels := r.Endpoint().Labels()
	deploymentLabels["pvc"] = r.pvcList[0].Destination().LabelSafeName()
	return transfer.AreFilteredPodsHealthy(c, r.pvcList.GetDestinationNamespaces()[0], deploymentLabels)
}

func createRcloneServerResources(c client.Client, r *RcloneTransfer, pvc transfer.PVCPair) error {
	var letters = []byte("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ")
	random.Seed(time.Now().UnixNano())
	password := make([]byte, 24)
	for i := range password {
		password[i] = letters[random.Intn(len(letters))]
	}

	r.password = string(password)
	r.port = rclonePort
	r.username = rcloneUser

	err := createRcloneServerConfig(c, r, pvc)
	if err != nil {
		return err
	}

	return nil
}

func createRcloneServerConfig(c client.Client, r *RcloneTransfer, pvc transfer.PVCPair) error {
	rcloneConfigMap := &v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: pvc.Destination().Claim().Namespace,
			Name:      rcloneConfigPrefix + pvc.Destination().LabelSafeName(),
			Labels:    r.Endpoint().Labels(),
		},
		Data: map[string]string{
			"rclone.conf": rcloneServerConf,
		},
	}

	return c.Create(context.TODO(), rcloneConfigMap, &client.CreateOptions{})
}

func createRcloneServer(c client.Client, r *RcloneTransfer, pvc transfer.PVCPair) error {
	deploymentLabels := r.Endpoint().Labels()
	deploymentLabels["pvc"] = pvc.Destination().LabelSafeName()
	containers := []v1.Container{
		{
			Name:  "rclone",
			Image: rcloneImage,
			Command: []string{
				"/usr/bin/rclone",
				"serve",
				"http",
				"mnt",
				"--user",
				rcloneUser,
				"--pass",
				r.password,
				"--config",
				"/etc/rclone.conf",
				"--addr",
				":8080",
			},
			Ports: []v1.ContainerPort{
				{
					Name:          "rclone",
					Protocol:      v1.ProtocolTCP,
					ContainerPort: rclonePort,
				},
			},
			VolumeMounts: []v1.VolumeMount{
				{
					Name:      "mnt",
					MountPath: "/mnt",
				},
				{
					Name:      rcloneConfigPrefix + pvc.Destination().LabelSafeName(),
					MountPath: "/etc/rclone.conf",
					SubPath:   "rclone.conf",
				},
			},
		},
	}

	containers = append(containers, r.Transport().ServerContainers()...)

	volumes := []v1.Volume{
		{
			Name: "mnt",
			VolumeSource: v1.VolumeSource{
				PersistentVolumeClaim: &v1.PersistentVolumeClaimVolumeSource{
					ClaimName: pvc.Destination().Claim().Name,
				},
			},
		},
		{
			Name: rcloneConfigPrefix + pvc.Destination().LabelSafeName(),
			VolumeSource: v1.VolumeSource{
				ConfigMap: &v1.ConfigMapVolumeSource{
					LocalObjectReference: v1.LocalObjectReference{
						Name: rcloneConfigPrefix + pvc.Destination().LabelSafeName(),
					},
				},
			},
		},
	}

	volumes = append(volumes, r.Transport().ServerVolumes()...)

	server := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      pvc.Destination().Claim().Name,
			Namespace: pvc.Destination().Claim().Namespace,
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
