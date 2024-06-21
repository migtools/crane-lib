package blockrsync

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/konveyor/crane-lib/state_transfer/transfer"
	"github.com/konveyor/crane-lib/state_transfer/transport"
	"github.com/konveyor/crane-lib/state_transfer/transport/stunnel"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	stunnelCommunicationVolumeName = "stunnel-communication"
	stunnelCommunicationVolumePath = "/usr/share/stunnel-communication"
	rsyncDoneFile                  = "blockrsync-done"
	proxyListenPort                = "9002"
)

func (r *BlockrsyncTransfer) CreateClient(c client.Client) error {
	pvc := r.pvcList[0]

	_, err := transport.CreateClient(r.Transport(), c, "block", r.Endpoint())
	if err != nil {
		return err
	}

	err = createBlockrsyncClient(c, r, pvc)
	if err != nil {
		return err
	}

	return nil
}

func createBlockrsyncClient(c client.Client, r *BlockrsyncTransfer, pvc transfer.PVCPair) error {
	podLabels := r.transferOptions.SourcePodMeta.Labels
	podLabels["pvc"] = pvc.Source().LabelSafeName()

	containers := []v1.Container{
		{
			Name:            Proxy,
			ImagePullPolicy: v1.PullAlways,
			Image:           r.transferOptions.GetBlockrsyncClientImage(),
			Command:         getProxyCommand(r.Transport().Port(), pvc.Source().LabelSafeName()),
			VolumeMounts: []v1.VolumeMount{
				{
					Name:      stunnelCommunicationVolumeName,
					MountPath: stunnelCommunicationVolumePath,
				},
			},
		},
		{
			Name:            BlockRsyncContainer,
			ImagePullPolicy: v1.PullAlways,
			Image:           r.transferOptions.GetBlockrsyncClientImage(),
		},
	}
	addVolumeToContainer(pvc.Source().Claim(), pvc.Source().LabelSafeName(), pvc.Source().LabelSafeName(), &containers[1])
	containers[1].Command = getBlockrsyncCommand(proxyListenPort, containers[1].Env[0].Value)

	customizeTransportContainers(r.Transport().Type(), r.transport.ClientContainers())
	containers = append(containers, r.Transport().ClientContainers()...)

	volumes := []v1.Volume{
		{
			Name: pvc.Source().LabelSafeName(),
			VolumeSource: v1.VolumeSource{
				PersistentVolumeClaim: &v1.PersistentVolumeClaimVolumeSource{
					ClaimName: pvc.Source().Claim().Name,
				},
			},
		},
		{
			Name: stunnelCommunicationVolumeName,
			VolumeSource: v1.VolumeSource{
				EmptyDir: &v1.EmptyDirVolumeSource{Medium: v1.StorageMediumDefault},
			},
		},
	}

	volumes = append(volumes, r.Transport().ClientVolumes()...)

	pod := v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: "blockrsync-",
			Namespace:    pvc.Source().Claim().Namespace,
			Labels:       podLabels,
		},
		Spec: v1.PodSpec{
			Containers:    containers,
			Volumes:       volumes,
			RestartPolicy: v1.RestartPolicyOnFailure,
		},
	}

	return c.Create(context.TODO(), &pod, &client.CreateOptions{})
}

func getProxyCommand(port int32, identifier string) []string {
	proxyCommand := []string{"/proxy",
		"--source",
		"--target-address",
		"localhost",
		"--identifier",
		identifier,
		"--listen-port",
		proxyListenPort,
		"--target-port",
		strconv.Itoa(int(port)),
		"--control-file",
		fmt.Sprintf("%s/%s", stunnelCommunicationVolumePath, rsyncDoneFile),
	}
	return []string{
		"/bin/bash",
		"-c",
		strings.Join(proxyCommand, " "),
	}
}

func getBlockrsyncCommand(port, file string) []string {
	proxyCommand := []string{"/blockrsync",
		file,
		"--source",
		"--target-address",
		"localhost",
		"--port",
		port,
		"--zap-log-level",
		"3",
		"--block-size",
		"131072",
	}
	return []string{
		"/bin/bash",
		"-c",
		strings.Join(proxyCommand, " "),
	}
}

func addVolumeToContainer(pvc *v1.PersistentVolumeClaim, header, identifier string, container *v1.Container) {
	sourceVolumeMode := v1.PersistentVolumeFilesystem
	if pvc.Spec.VolumeMode != nil && *pvc.Spec.VolumeMode == v1.PersistentVolumeBlock {
		sourceVolumeMode = v1.PersistentVolumeBlock
	}
	if sourceVolumeMode == v1.PersistentVolumeFilesystem {
		container.Env = append(container.Env, v1.EnvVar{
			Name:  fmt.Sprintf("id-%s", header),
			Value: fmt.Sprintf("/mnt/%s/disk.img", identifier),
		})
		container.VolumeMounts = append(container.VolumeMounts, v1.VolumeMount{
			Name:      identifier,
			MountPath: fmt.Sprintf("/mnt/%s", identifier),
		})
	} else {
		container.Env = append(container.Env, v1.EnvVar{
			Name:  fmt.Sprintf("id-%s", header),
			Value: fmt.Sprintf("/dev/%s", identifier),
		})
		container.VolumeDevices = append(container.VolumeDevices, v1.VolumeDevice{
			Name:       identifier,
			DevicePath: fmt.Sprintf("/dev/%s", identifier),
		})
	}
}

func customizeTransportContainers(t transport.TransportType, containers []v1.Container) {
	switch t {
	case stunnel.TransportTypeStunnel:
		var stunnelContainer *v1.Container
		for i := range containers {
			c := &containers[i]
			if c.Name == stunnel.StunnelContainer {
				stunnelContainer = c
			}
		}
		stunnelContainer.Command = []string{
			"/bin/bash",
			"-c",
			`/bin/stunnel /etc/stunnel/stunnel.conf
while true
do test -f /usr/share/stunnel-communication/blockrsync-done
if [ $? -eq 0 ]
then
	break
else
	sleep 1
fi
done
exit 0`,
		}
		stunnelContainer.VolumeMounts = append(
			stunnelContainer.VolumeMounts,
			v1.VolumeMount{
				Name:      stunnelCommunicationVolumeName,
				MountPath: stunnelCommunicationVolumePath,
			})
	}
}
