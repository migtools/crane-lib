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
	// Image is the mover image; it must provide rclone (sync) and a POSIX sh (the
	// download leg wraps rclone in sh, see tolerateEmptySourceCommand). Defaults
	// to transport.DefaultRsyncTransferImage, which has both.
	Image        string
	CloudStorage string
	ConfigSecret string
	Encrypt      bool
	// TODO: when false, run a cleanup pod to delete intermediate cloud data
	// after a successful download.
	KeepCloudData           bool
	Labels                  map[string]string
	UploadSecurityContext   corev1.PodSecurityContext
	DownloadSecurityContext corev1.PodSecurityContext
}

// Validate checks required fields. When Encrypt is true, the caller must append
// a valid [encrypted] crypt section to rclone.conf before creating ConfigSecret.
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
		// Replicate empty source dirs (migtools/crane#1082); for object
		// storage the caller also appends --<backend>-directory-markers.
		"--create-empty-src-dirs",
		"-v",
	}
	return append(cmd, extraArgs...)
}

// emptySourceExitCode is rclone's exit code for "directory not found".
const emptySourceExitCode = 3

// tolerateEmptySourceCommand wraps an rclone download so rclone's "directory not
// found" exit (rc=3, raised for an empty source PVC under
// --<backend>-directory-markers, migtools/crane#1082) maps to success only when
// destPath is empty; other codes, and rc=3 with a non-empty destination,
// propagate so stale files are never masked. The emptiness check is pure shell
// (the minimal rclone image has no find) and ignores a root-owned lost+found,
// which fresh ext4/xfs PVCs create and which is not user data.
func tolerateEmptySourceCommand(destPath string, rcloneCmd []string) []string {
	script := fmt.Sprintf(
		`"$@"
rc=$?
if [ "$rc" -eq %[1]d ]; then
  for e in %[2]q/* %[2]q/.[!.]* %[2]q/..?*; do
    [ -e "$e" ] || [ -L "$e" ] || continue
    [ "$e" = %[2]q/lost+found ] && continue
    echo "indirect download: source root not found but destination is not empty (found $e); refusing to treat empty source as success" >&2
    exit "$rc"
  done
  echo "indirect download: source root not found (empty source PVC); destination empty, nothing to restore"
  exit 0
fi
exit "$rc"`,
		emptySourceExitCode, destPath,
	)
	// "sh" is the $0 placeholder; rcloneCmd becomes "$@".
	return append([]string{"sh", "-c", script, "sh"}, rcloneCmd...)
}

// directoryMarkerFlag maps an rclone backend type to the flag that writes a
// 0-byte marker per empty dir so empty dirs survive object storage. Returns ""
// for backends with no such flag; an unknown flag makes rclone abort, so never
// guess.
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

// remoteSection returns the key/value pairs of the [remoteName] section in
// rclone.conf content, or nil if the section is absent.
func remoteSection(config []byte, remoteName string) map[string]string {
	wantHeader := "[" + remoteName + "]"
	inSection := false
	var section map[string]string
	for _, line := range strings.Split(string(config), "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]") {
			inSection = trimmed == wantHeader
			if inSection && section == nil {
				section = map[string]string{}
			}
			continue
		}
		if !inSection {
			continue
		}
		if key, value, ok := strings.Cut(trimmed, "="); ok {
			section[strings.TrimSpace(key)] = strings.TrimSpace(value)
		}
	}
	return section
}

// remoteBackendType returns the "type" value of the [remoteName] section in
// rclone.conf content, or "" if the section or key is absent.
func remoteBackendType(config []byte, remoteName string) string {
	return remoteSection(config, remoteName)["type"]
}

// resolveBackendType returns the concrete backend type for remoteName, following
// rclone "alias" and "crypt" remotes (both wrap another remote via "remote =")
// to their target so directoryMarkerFlag matches the underlying backend rather
// than the wrapper (which has no flag). Returns "" if it cannot be resolved; the
// visited set guards against cycles.
func resolveBackendType(config []byte, remoteName string) string {
	visited := map[string]bool{}
	for remoteName != "" && !visited[remoteName] {
		visited[remoteName] = true
		section := remoteSection(config, remoteName)
		backendType := section["type"]
		if !strings.EqualFold(backendType, "alias") && !strings.EqualFold(backendType, "crypt") {
			return backendType
		}
		// "remote = target:path" or "target,opt=val:path" -> "target".
		target, _, _ := strings.Cut(section["remote"], ":")
		target, _, _ = strings.Cut(target, ",")
		remoteName = strings.TrimSpace(target)
	}
	return ""
}

// directoryMarkerArgs returns the directory-markers flag for Options.CloudStorage,
// reading its backend type from the rclone config Secret. Returns nil (no error)
// when the backend is unknown, so an unsupported backend never fails a transfer.
// Returns an error if the config Secret cannot be read, so the caller does not
// start a transfer that would silently drop empty directories. Targets the
// underlying backend even when Encrypt wraps it in a crypt remote.
func (t *IndirectTransfer) directoryMarkerArgs(ctx context.Context, c client.Client, namespace string) ([]string, error) {
	remoteName, _, _ := strings.Cut(t.options.CloudStorage, ":")
	// Strip any inline option suffix: "remote,env_auth=true" -> "remote".
	remoteName, _, _ = strings.Cut(remoteName, ",")
	if remoteName == "" {
		return nil, nil
	}
	secret := &corev1.Secret{}
	if err := c.Get(ctx, client.ObjectKey{Namespace: namespace, Name: t.options.ConfigSecret}, secret); err != nil {
		return nil, fmt.Errorf("failed to read rclone config secret %s/%s: %w", namespace, t.options.ConfigSecret, err)
	}
	flag := directoryMarkerFlag(resolveBackendType(secret.Data["rclone.conf"], remoteName))
	if flag == "" {
		return nil, nil
	}
	return []string{flag}, nil
}

// metadataSetArgs returns --metadata-set flags that set uid/gid to the pod's own
// UID so chown is a no-op; otherwise rclone treats chown failure as fatal and
// deletes the file.
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

// BuildCryptSection returns the rclone config section enabling client-side
// encryption, which the caller appends to rclone.conf before creating the Secret.
// obscuredPassword must be in rclone's obscured format ("rclone obscure <pw>").
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
