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
	scratchMountPath   = "/scratch"
	configVolumeName   = "rclone-config"
	dataVolumeName     = "data"
	scratchVolumeName  = "scratch"
	maxPodNameLen      = 63
	CryptRemoteName    = "encrypted"
	CryptRemoteSection = "[encrypted]"
	// dirPermsManifest lists "<depth>\t<octal-mode>\t<relpath>" per source dir,
	// each record NUL-terminated so dir names containing newlines survive, so
	// download can restore modes that rclone's --metadata drops for directories
	// over object storage. See migtools/crane#1084.
	dirPermsManifest = ".crane-dirperms"
	// dirPermsMetaDir is the bucket subdir holding the manifest; it rides its own
	// sync because the mover's stripped rclone has only "sync".
	dirPermsMetaDir = ".cranemeta"
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
						// Writable scratch for the dir-perms manifest (source is
						// read-only on upload).
						{Name: scratchVolumeName, MountPath: scratchMountPath},
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
				{
					Name: scratchVolumeName,
					VolumeSource: corev1.VolumeSource{
						EmptyDir: &corev1.EmptyDirVolumeSource{},
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
		"-v",
	}
	return append(cmd, extraArgs...)
}

// shellQuote single-quotes s so it is safe to embed in an `sh -c` script,
// escaping any embedded single quotes.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// shellJoin quotes each element of argv and joins them with spaces for use
// inside an `sh -c` script.
func shellJoin(argv []string) string {
	quoted := make([]string, len(argv))
	for i, a := range argv {
		quoted[i] = shellQuote(a)
	}
	return strings.Join(quoted, " ")
}

// buildUploadCommand wraps the rclone data sync in a bash script that records
// every source directory's mode into the manifest, then syncs it into the bucket
// for download to replay (crane#1084).
//
// The image has no find(1), so dirs are enumerated with globstar (dotglob catches
// hidden dirs) and stat reads the mode. Entries are sorted deepest-first so
// download restores children before a parent's search bit is tightened; the PVC
// root is recorded as "." with depth -1 so it restores last.
//
// Data sync runs first so its mirror does not delete the meta subdir, and its
// exit code is preserved for partial-success handling. The manifest sync is
// best-effort with no "-v"/"--progress" so it never shadows the "Transferred:"
// line.
//
// If the source already owns a .cranemeta, the manifest is skipped with a warning
// so the mirroring meta sync cannot overwrite it.
func buildUploadCommand(src, dst, metaRemote string) []string {
	dataSync := shellJoin(buildRcloneCommand("sync", src, dst))
	metaSync := shellJoin([]string{
		"rclone", "sync",
		scratchMountPath, metaRemote,
		"--config", configMountPath + "/rclone.conf",
	})
	manifest := scratchMountPath + "/" + dirPermsManifest
	lines := []string{
		"base=" + shellQuote(src),
		"man=" + shellQuote(manifest),
		"shopt -s globstar dotglob nullglob",
		`: > "$man"`,
		`for d in "$base"/**/; do`,
		`  rel="${d#$base/}"; rel="${rel%/}"`,
		`  mode=$(stat -c '%a' "$d")`,
		`  if [ -z "$rel" ]; then rel="."; depth=-1; else slashes="${rel//[^\/]/}"; depth=${#slashes}; fi`,
		"  printf '%s\\t%s\\t%s\\0' \"$depth\" \"$mode\" \"$rel\" >> \"$man\"",
		"done",
		`sort -z -rn -k1,1 "$man" -o "$man"`,
		dataSync + "; rc=$?",
		// Skip the meta sync if the source owns a .cranemeta, so it is not overwritten.
		`if [ -e "$base/` + dirPermsMetaDir + `" ]; then`,
		`  echo "crane: source PVC already contains ` + dirPermsMetaDir + `; skipping directory-permission manifest so the existing entry is not overwritten (directory modes will not be preserved for this PVC)" >&2`,
		"else",
		"  " + metaSync + " || true",
		"fi",
		"exit $rc",
	}
	return []string{"bash", "-c", strings.Join(lines, "\n")}
}

// buildDownloadCommand wraps the rclone data sync in a bash script that replays
// the dir-perms manifest (carried into dst by the data sync) to restore each
// directory's mode, then removes only its own manifest file and the meta subdir
// if empty, so a source-owned .cranemeta is never clobbered. chmod failures
// (e.g. an untransferred empty dir) are ignored; the rclone exit code is
// preserved for partial-success handling.
func buildDownloadCommand(src, dst string, extraArgs ...string) []string {
	dataSync := shellJoin(buildRcloneCommand("sync", src, dst, extraArgs...))
	metaDir := dst + "/" + dirPermsMetaDir
	manifest := metaDir + "/" + dirPermsManifest
	lines := []string{
		"base=" + shellQuote(dst),
		"man=" + shellQuote(manifest),
		dataSync + "; rc=$?",
		`if [ -f "$man" ]; then`,
		`  while IFS=$'\t' read -r -d '' depth mode rel; do`,
		`    [ -n "$rel" ] && chmod "$mode" "$base/$rel" 2>/dev/null || true`,
		`  done < "$man"`,
		"fi",
		// Remove only our manifest, then the meta dir if empty (keeps a source .cranemeta).
		`rm -f "$man"`,
		"rmdir " + shellQuote(metaDir) + " 2>/dev/null || true",
		"exit $rc",
	}
	return []string{"bash", "-c", strings.Join(lines, "\n")}
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
