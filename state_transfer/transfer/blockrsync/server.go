package blockrsync

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/konveyor/crane-lib/state_transfer/endpoint"
	"github.com/konveyor/crane-lib/state_transfer/transfer"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	v1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	blockrsyncServerPodName = "blockrsync-server"
)

func (r *BlockrsyncTransfer) CreateServer(c client.Client) error {
	err := r.createBlockrysncServer(c)
	if err != nil {
		return err
	}

	_, err = endpoint.Create(r.Endpoint(), c)
	return err
}

func (r *BlockrsyncTransfer) IsServerHealthy(c client.Client) (bool, error) {
	deploymentLabels := r.Endpoint().Labels()
	deploymentLabels["pvc"] = r.pvcList[0].Destination().LabelSafeName()
	return transfer.AreFilteredPodsHealthy(c, r.pvcList.GetDestinationNamespaces()[0], deploymentLabels)
}

func (r *BlockrsyncTransfer) createBlockrysncServer(c client.Client) error {
	pvcs := r.PVCs()
	destNs := r.pvcList.GetDestinationNamespaces()[0]
	containers := make([]v1.Container, 0)
	volumes := make([]v1.Volume, 0)
	blockRsyncCommand := []string{"/proxy",
		"--target",
		"--listen-port",
		strconv.Itoa(int(r.Transport().ExposedPort())),
		"--blockrsync-path",
		"/blockrsync",
		"--control-file",
		fmt.Sprintf("%s/%s", stunnelCommunicationVolumePath, rsyncDoneFile),
		"--block-size",
		"131072",
	}
	container := v1.Container{
		Name:            BlockRsyncContainer,
		ImagePullPolicy: v1.PullAlways,
		Image:           r.transferOptions.GetBlockrsyncServerImage(),
		Ports: []v1.ContainerPort{
			{
				Name:          "blockrsync",
				Protocol:      v1.ProtocolTCP,
				ContainerPort: r.Transport().ExposedPort(),
			},
		},
		VolumeMounts: []v1.VolumeMount{
			{
				Name:      stunnelCommunicationVolumeName,
				MountPath: stunnelCommunicationVolumePath,
			},
		},
	}
	for _, pvc := range pvcs {
		blockRsyncCommand = append(blockRsyncCommand, "--identifier", pvc.Source().LabelSafeName())
		addVolumeToContainer(pvc.Destination().Claim(), pvc.Source().LabelSafeName(), pvc.Destination().LabelSafeName(), &container)
		volumes = append(volumes, v1.Volume{
			Name: pvc.Destination().LabelSafeName(),
			VolumeSource: v1.VolumeSource{
				PersistentVolumeClaim: &v1.PersistentVolumeClaimVolumeSource{
					ClaimName: pvc.Destination().Claim().Name,
				},
			},
		})
	}
	blockRsyncContainerCommand := []string{
		"/bin/bash",
		"-c",
		strings.Join(blockRsyncCommand, " "),
	}
	container.Command = blockRsyncContainerCommand
	containers = append(containers, container)

	containers = append(containers, r.Transport().ServerContainers()...)

	volumes = append(volumes, v1.Volume{
		Name: stunnelCommunicationVolumeName,
		VolumeSource: v1.VolumeSource{
			EmptyDir: &v1.EmptyDirVolumeSource{Medium: v1.StorageMediumDefault},
		},
	})

	volumes = append(volumes, r.Transport().ServerVolumes()...)

	server := v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      blockrsyncServerPodName,
			Namespace: destNs,
			Labels:    r.transferOptions.SourcePodMeta.Labels,
		},
		Spec: v1.PodSpec{
			Containers:    containers,
			Volumes:       volumes,
			RestartPolicy: v1.RestartPolicyOnFailure,
		},
	}

	return c.Create(context.TODO(), &server, &client.CreateOptions{})
}
