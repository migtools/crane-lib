package indirect

import (
	"context"
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	defaultImage     = "quay.io/konveyor/rsync-transfer:latest"
	configMountPath  = "/etc/rclone"
	dataMountPath    = "/data"
	configVolumeName = "rclone-config"
	dataVolumeName   = "data"
	maxPodNameLen    = 63
)

type Options struct {
	Image                  string
	CloudStorage           string
	ConfigSecret           string
	Encrypt                bool
	KeepCloudData          bool
	Labels                 map[string]string
	UploadSecurityContext  corev1.PodSecurityContext
	DownloadSecurityContext corev1.PodSecurityContext
}

func (o *Options) Validate() error {
	if o.CloudStorage == "" {
		return fmt.Errorf("cloud storage path is required")
	}
	if o.ConfigSecret == "" {
		return fmt.Errorf("rclone config secret name is required")
	}
	return nil
}

type IndirectTransfer struct {
	sourceClient client.Client
	destClient   client.Client
	options      Options
}

func New(srcClient, destClient client.Client, opts Options) *IndirectTransfer {
	if opts.Image == "" {
		opts.Image = defaultImage
	}
	if len(opts.Labels) == 0 {
		opts.Labels = map[string]string{
			"app.kubernetes.io/name":      "crane",
			"app.kubernetes.io/component": "indirect-transfer",
		}
	}
	return &IndirectTransfer{
		sourceClient: srcClient,
		destClient:   destClient,
		options:      opts,
	}
}

func isPodComplete(ctx context.Context, c client.Client, podName, namespace string) (bool, error) {
	pod := &corev1.Pod{}
	if err := c.Get(ctx, client.ObjectKey{Name: podName, Namespace: namespace}, pod); err != nil {
		return false, err
	}
	switch pod.Status.Phase {
	case corev1.PodSucceeded:
		return true, nil
	case corev1.PodFailed:
		return true, fmt.Errorf("pod %s/%s failed", namespace, podName)
	default:
		return false, nil
	}
}

func copyLabels(src map[string]string) map[string]string {
	dst := make(map[string]string, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

func truncatePodName(name string) string {
	if len(name) <= maxPodNameLen {
		return name
	}
	name = name[:maxPodNameLen]
	return strings.TrimRight(name, "-.")
}

func (t *IndirectTransfer) buildPod(name, namespace, pvcName string, command []string, secCtx corev1.PodSecurityContext) *corev1.Pod {
	podLabels := copyLabels(t.options.Labels)
	podLabels["app.konveyor.io/created-for-pvc"] = pvcName
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      truncatePodName(name),
			Namespace: namespace,
			Labels:    podLabels,
		},
		Spec: corev1.PodSpec{
			RestartPolicy:   corev1.RestartPolicyNever,
			SecurityContext: &secCtx,
			Containers: []corev1.Container{
				{
					Name:    "rclone",
					Image:   t.options.Image,
					Command: command,
					VolumeMounts: []corev1.VolumeMount{
						{Name: dataVolumeName, MountPath: dataMountPath},
						{Name: configVolumeName, MountPath: configMountPath, ReadOnly: true},
					},
				},
			},
			Volumes: []corev1.Volume{
				{
					Name: dataVolumeName,
					VolumeSource: corev1.VolumeSource{
						PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
							ClaimName: pvcName,
						},
					},
				},
				{
					Name: configVolumeName,
					VolumeSource: corev1.VolumeSource{
						Secret: &corev1.SecretVolumeSource{
							SecretName: t.options.ConfigSecret,
						},
					},
				},
			},
		},
	}
}

func buildRcloneCommand(subcommand, src, dst string) []string {
	return []string{
		"rclone", subcommand,
		src, dst,
		"--config", configMountPath + "/rclone.conf",
		"--progress",
		"--links",
		"-v",
	}
}
