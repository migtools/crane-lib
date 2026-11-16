package indirect

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
)

func (t *IndirectTransfer) Upload(ctx context.Context, pvc *corev1.PersistentVolumeClaim) (*corev1.Pod, error) {
	if err := t.options.Validate(); err != nil {
		return nil, fmt.Errorf("invalid options for upload: %w", err)
	}
	remotePath := fmt.Sprintf("%s/%s/%s", t.options.CloudStorage, pvc.Namespace, pvc.Name)
	if t.options.Encrypt {
		remotePath = CryptRemoteName + ":"
	}
	markerArgs, err := t.directoryMarkerArgs(ctx, t.sourceClient, pvc.Namespace)
	if err != nil {
		return nil, err
	}
	// Sync the data, then mkdir the remote root so it always exists. For a
	// populated PVC the mkdir is a no-op; for an empty one it creates the root
	// marker (persisted by the directory-markers flag) that the download's sync
	// needs, so an empty PVC transfers cleanly (migtools/crane#1082) without the
	// download having to tolerate (and mask) a genuinely missing prefix.
	syncCmd := buildRcloneCommand("sync", dataMountPath, remotePath, markerArgs...)
	mkdirCmd := buildRcloneMkdirCommand(remotePath, markerArgs...)
	command := ensureRemoteRootCommand(syncCmd, mkdirCmd)

	pod := t.buildPod(
		fmt.Sprintf("rclone-upload-%s", pvc.Name),
		pvc.Namespace,
		pvc.Name,
		command,
		t.options.UploadSecurityContext,
		true, // mount source PVC read-only; the mover only reads from it
	)

	if err := t.sourceClient.Create(ctx, pod); err != nil {
		return nil, fmt.Errorf("failed to create upload pod: %w", err)
	}
	return pod, nil
}

func (t *IndirectTransfer) IsUploadComplete(ctx context.Context, podName, namespace string) (bool, error) {
	return isPodComplete(ctx, t.sourceClient, podName, namespace)
}
