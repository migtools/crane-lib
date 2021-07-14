package rsync

import (
	"context"
	"fmt"

	"github.com/konveyor/crane-lib/state_transfer/transfer"
	"github.com/konveyor/crane-lib/state_transfer/transport"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	errorsutil "k8s.io/apimachinery/pkg/util/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func (r *RsyncTransfer) CreateClient(c client.Client) error {
	sourceNs := r.pvcList.GetSourceNamespaces()[0]
	err := createRsyncClientResources(c, r, sourceNs)
	if err != nil {
		return err
	}

	_, err = transport.CreateClient(r.Transport(), c, r.Endpoint())
	if err != nil {
		return err
	}

	err = createRsyncClient(c, r, sourceNs)
	if err != nil {
		return err
	}

	return nil
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
		rsyncCommand := []string{"/usr/bin/rsync"}
		rsyncCommand = append(rsyncCommand, rsyncOptions...)
		rsyncCommand = append(rsyncCommand, "/mnt/")
		rsyncCommand = append(rsyncCommand,
			fmt.Sprintf("rsync://%s@%s:%d/mnt/%s",
				r.Username(), transfer.ConnectionHostname(r),
				transfer.ConnectionPort(r), pvc.Destination().ValidatedName()))
		podLabels["created-for-pvc"] = pvc.Destination().ValidatedName()
		containers := []v1.Container{
			{
				Name:    "rsync",
				Image:   rsyncImage,
				Command: rsyncCommand,
				Env: []v1.EnvVar{
					{
						Name:  "RSYNC_PASSWORD",
						Value: r.Password(),
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
		containers = append(containers, r.Transport().ClientContainers()...)
		volumes := []v1.Volume{
			{
				Name: "mnt",
				VolumeSource: v1.VolumeSource{
					PersistentVolumeClaim: &v1.PersistentVolumeClaimVolumeSource{
						ClaimName: pvc.Source().Claim().Name,
					},
				},
			},
		}
		volumes = append(volumes, r.Transport().ClientVolumes()...)
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

		err := c.Create(context.TODO(), &pod, &client.CreateOptions{})
		errs = append(errs, err)
	}

	return errorsutil.NewAggregate(errs)
}
