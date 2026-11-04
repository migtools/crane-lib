package indirect

import (
	"context"
	"fmt"
	"strings"

	"github.com/konveyor/crane-lib/state_transfer/transport"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	configMountPath    = "/etc/rclone"
	dataMountPath      = "/data"
	configVolumeName   = "rclone-config"
	dataVolumeName     = "data"
	maxPodNameLen      = 63
	CryptRemoteName    = "encrypted"
	CryptRemoteSection = "[encrypted]"
)

type Options struct {
	Image        string
	CloudStorage string
	ConfigSecret string
	Encrypt      bool
	// TODO: KeepCloudData skips cloud storage cleanup after transfer.
	// When false, a cleanup pod should run "rclone delete remote:bucket/ns/pvc/"
	// after successful download to remove intermediate data from S3.
	KeepCloudData           bool
	Labels                  map[string]string
	UploadSecurityContext   corev1.PodSecurityContext
	DownloadSecurityContext corev1.PodSecurityContext
}

// Validate checks required fields. When Encrypt is true, the caller is
// responsible for appending a valid [encrypted] crypt section to the
// rclone.conf before creating the ConfigSecret.
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
		opts.Image = transport.DefaultRsyncTransferImage
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

func (t *IndirectTransfer) buildPod(name, namespace, pvcName string, command []string, secCtx corev1.PodSecurityContext, dataReadOnly bool) *corev1.Pod {
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
						{Name: dataVolumeName, MountPath: dataMountPath, ReadOnly: dataReadOnly},
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

func buildRcloneCommand(subcommand, src, dst string, extraArgs ...string) []string {
	cmd := []string{
		"rclone", subcommand,
		src, dst,
		"--config", configMountPath + "/rclone.conf",
		"--progress",
		"--links",
		"--metadata",
		// Replicate empty source dirs (issue migtools/crane#1082). Not
		// enough alone for object storage; the caller appends the matching
		// --<backend>-directory-markers flag via directoryMarkerArgs.
		"--create-empty-src-dirs",
		"-v",
	}
	return append(cmd, extraArgs...)
}

// directoryMarkerFlag maps an rclone backend type to the per-backend flag that
// writes a 0-byte marker per empty dir, so empty dirs survive object storage.
// Returns "" for backends with no such flag: an unknown
// --<backend>-directory-markers flag makes rclone abort, so never guess.
func directoryMarkerFlag(backendType string) string {
	switch strings.ToLower(strings.TrimSpace(backendType)) {
	case "s3":
		return "--s3-directory-markers"
	case "gcs", "google cloud storage":
		return "--gcs-directory-markers"
	case "azureblob", "azure blob", "azure blob storage":
		return "--azureblob-directory-markers"
	default:
		return ""
	}
}

// remoteBackendType returns the "type" value of the [remoteName] section in
// rclone.conf content, or "" if the section or key is absent.
func remoteBackendType(config []byte, remoteName string) string {
	wantHeader := "[" + remoteName + "]"
	inSection := false
	for _, line := range strings.Split(string(config), "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]") {
			inSection = trimmed == wantHeader
			continue
		}
		if !inSection {
			continue
		}
		key, value, ok := strings.Cut(trimmed, "=")
		if ok && strings.TrimSpace(key) == "type" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

// directoryMarkerArgs returns the directory-markers flag for the remote named
// in Options.CloudStorage, reading its backend type from the rclone config
// Secret. Returns nil when the backend is unknown, so a transfer never fails
// just because markers could not be enabled. Always targets the underlying
// CloudStorage backend, even when Encrypt wraps it in a crypt remote.
func (t *IndirectTransfer) directoryMarkerArgs(ctx context.Context, c client.Client, namespace string) []string {
	remoteName, _, _ := strings.Cut(t.options.CloudStorage, ":")
	if remoteName == "" {
		return nil
	}
	secret := &corev1.Secret{}
	if err := c.Get(ctx, client.ObjectKey{Namespace: namespace, Name: t.options.ConfigSecret}, secret); err != nil {
		return nil
	}
	flag := directoryMarkerFlag(remoteBackendType(secret.Data["rclone.conf"], remoteName))
	if flag == "" {
		return nil
	}
	return []string{flag}
}

// metadataSetArgs returns --metadata-set flags that override uid/gid to the
// pod's own UID so chown becomes a no-op. Without this, rclone treats chown
// failure as fatal and deletes the file.
func metadataSetArgs(secCtx corev1.PodSecurityContext) []string {
	uid := int64(65534) // default: nobody
	if secCtx.RunAsUser != nil {
		uid = *secCtx.RunAsUser
	}
	gid := uid
	if secCtx.RunAsGroup != nil {
		gid = *secCtx.RunAsGroup
	} else if secCtx.FSGroup != nil {
		gid = *secCtx.FSGroup
	}
	return []string{
		"--metadata-set", fmt.Sprintf("uid=%d", uid),
		"--metadata-set", fmt.Sprintf("gid=%d", gid),
	}
}

// BuildCryptSection returns the rclone config section that enables client-side
// encryption. The caller appends this to the existing rclone.conf before
// creating the K8s Secret. obscuredPassword must be in rclone's obscured format
// (produced by "rclone obscure <plaintext>").
func BuildCryptSection(cloudStoragePath, obscuredPassword string) (string, error) {
	if cloudStoragePath == "" {
		return "", fmt.Errorf("cloud storage path is required for encryption config")
	}
	if obscuredPassword == "" {
		return "", fmt.Errorf("obscured password is required for encryption config")
	}
	return fmt.Sprintf("\n%s\ntype = crypt\nremote = %s\npassword = %s\n",
		CryptRemoteSection, cloudStoragePath, obscuredPassword), nil
}
