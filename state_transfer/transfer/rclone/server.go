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
	"github.com/konveyor/crane-lib/state_transfer/transport"
)

const (
	rcloneServerConf = `[mnt]
type = local
`
)

func (r *RcloneTransfer) CreateServer(c client.Client) error {
	err := createRcloneServerResources(c, r)
	if err != nil {
		return err
	}

	t, err := transport.CreateTransportServer(r.Transport(), c, r.Endpoint())
	if err != nil {
		return err
	}
	r.SetTransport(t)

	err = createRcloneServer(c, r)
	if err != nil {
		return err
	}

	e, err := endpoint.CreateEndpoint(r.Endpoint(), c)
	if err != nil {
		return err
	}
	r.SetEndpoint(e)

	return nil
}

func createRcloneServerResources(c client.Client, r *RcloneTransfer) error {
	var letters = []byte("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ")
	random.Seed(time.Now().UnixNano())
	password := make([]byte, 24)
	for i := range password {
		password[i] = letters[random.Intn(len(letters))]
	}

	r.SetPassword(string(password))
	r.SetPort(rclonePort)
	r.SetUsername(rcloneUser)

	err := createRcloneServerConfig(c, r)
	if err != nil {
		return err
	}

	return nil
}

func createRcloneServerConfig(c client.Client, r *RcloneTransfer) error {
	rcloneConfigMap := &v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: r.PVC().Namespace,
			Name:      rcloneConfigPrefix + r.PVC().Name,
			Labels:    r.Endpoint().Labels(),
		},
		Data: map[string]string{
			"rclone.conf": rcloneServerConf,
		},
	}

	return c.Create(context.TODO(), rcloneConfigMap, &client.CreateOptions{})
}

func createRcloneServer(c client.Client, r *RcloneTransfer) error {
	deploymentLabels := r.Endpoint().Labels()
	deploymentLabels["pvc"] = r.PVC().Name
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
				r.Password(),
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
					Name:      rcloneConfigPrefix + r.PVC().Name,
					MountPath: "/etc/rclone.conf",
					SubPath:   "rclone.conf",
				},
			},
		},
	}

	for _, container := range r.Transport().ServerContainers() {
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
