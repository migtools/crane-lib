package rsync

import (
	"context"
	"fmt"
	"strings"

	"github.com/konveyor/crane-lib/state_transfer/transfer"
	"github.com/konveyor/crane-lib/state_transfer/transport"
	"github.com/konveyor/crane-lib/state_transfer/transport/stunnel"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	errorsutil "k8s.io/apimachinery/pkg/util/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func (r *RsyncTransfer) CreateClient(c client.Client) error {
	sourceNs := r.pvcList.GetSourceNamespaces()[0]

	errs := []error{}
	err := createRsyncClientResources(c, r, sourceNs)
	errs = append(errs, err)

	// _, err = transport.CreateClient(r.Transport(), c, r.Endpoint())
	// errs = append(errs, err)

	err = createRsyncClient(c, r, sourceNs)
	errs = append(errs, err)

	return errorsutil.NewAggregate(errs)
}

func createRsyncClientResources(c client.Client, r *RsyncTransfer, ns string) error {
	// no resource are created for rsync client side
	return nil
}

func createRsyncClient(c client.Client, r *RsyncTransfer, ns string) error {
	var errs []error
	transferOptions := r.transferOptions()
	rsyncOptions, err := transferOptions.AsRsyncCommandOptions()
	if err != nil {
		return err
	}
	podLabels := transferOptions.SourcePodMeta.Labels
	for _, pvc := range r.pvcList.InSourceNamespace(ns) {
		// create Rsync command for PVC
		rsyncCommand := []string{"/usr/bin/rsync"}
		rsyncCommand = append(rsyncCommand, rsyncOptions...)
		rsyncCommand = append(rsyncCommand, fmt.Sprintf("%s/", getMountPathForPVC(pvc.Source())))
		rsyncCommand = append(rsyncCommand,
			fmt.Sprintf("rsync://%s@%s/%s --port %d",
				transferOptions.username, transfer.ConnectionHostname(r),
				pvc.Destination().LabelSafeName(), r.Transport().Port()))
		rsyncCommandBashScript := fmt.Sprintf(
			"trap \"touch /usr/share/rsync/rsync-client-container-done\" EXIT SIGINT SIGTERM; timeout=120; SECONDS=0; while [ $SECONDS -lt $timeout ]; do nc -z localhost %d; rc=$?; if [ $rc -eq 0 ]; then %s; rc=$?; break; fi; done; exit $rc;",
			r.Transport().Port(),
			strings.Join(rsyncCommand, " "))
		rsyncContainerCommand := []string{
			"/bin/bash",
			"-c",
			rsyncCommandBashScript,
		}
		// create rsync container
		containers := []v1.Container{
			{
				Name:    RsyncContainer,
				Image:   r.getRsyncClientImage(),
				Command: rsyncContainerCommand,
				Env: []v1.EnvVar{
					{
						Name:  "RSYNC_PASSWORD",
						Value: transferOptions.password,
					},
				},

				VolumeMounts: []v1.VolumeMount{
					{
						Name:      "mnt",
						MountPath: getMountPathForPVC(pvc.Source()),
					},
					{
						Name:      "rsync-communication",
						MountPath: "/usr/share/rsync",
					},
				},
			},
		}
		// attach transport containers
		customizeTransportClientContainers(r.Transport())
		containers = append(containers, r.Transport().ClientContainers()...)
		// apply container mutations
		for i := range containers {
			c := &containers[i]
			applyContainerMutations(c, r.options.SourceContainerMutations)
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
				Name: "rsync-communication",
				VolumeSource: v1.VolumeSource{
					EmptyDir: &v1.EmptyDirVolumeSource{Medium: v1.StorageMediumDefault},
				},
			},
		}
		volumes = append(volumes, r.Transport().ClientVolumes()...)
		podSpec := v1.PodSpec{
			Containers:    containers,
			Volumes:       volumes,
			RestartPolicy: v1.RestartPolicyNever,
		}

		applyPodMutations(&podSpec, r.options.SourcePodMutations)

		pod := v1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "rsync-",
				Namespace:    pvc.Source().Claim().Namespace,
				Labels:       podLabels,
			},
			Spec: podSpec,
		}

		err := c.Create(context.TODO(), &pod, &client.CreateOptions{})
		errs = append(errs, err)
	}

	return errorsutil.NewAggregate(errs)
}

// customizeTransportClientContainers customizes transport's client containers for specific rsync communication
func customizeTransportClientContainers(t transport.Transport) {
	switch t.Type() {
	case stunnel.TransportTypeStunnel:
		var stunnelContainer *v1.Container
		for i := range t.ClientContainers() {
			c := &t.ClientContainers()[i]
			if c.Name == stunnel.StunnelContainer {
				stunnelContainer = c
			}
		}
		stunnelContainer.Command = []string{
			"/bin/bash",
			"-c",
			`/bin/stunnel /etc/stunnel/stunnel.conf
while true
do test -f /usr/share/rsync/rsync-client-container-done
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
				Name:      "rsync-communication",
				MountPath: "/usr/share/rsync",
			})
	}
}
