package indirect

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
)

func (t *IndirectTransfer) Download(ctx context.Context, pvc *corev1.PersistentVolumeClaim, sourceNamespace, remotePVCName string) (*corev1.Pod, error) {
	if err := t.options.Validate(); err != nil {
		return nil, fmt.Errorf("invalid options for download: %w", err)
	}
	remotePath := fmt.Sprintf("%s/%s/%s", t.options.CloudStorage, sourceNamespace, remotePVCName)
	command := buildRcloneCommand("sync", remotePath, dataMountPath)

	pod := t.buildPod(
		fmt.Sprintf("rclone-download-%s", pvc.Name),
		pvc.Namespace,
		pvc.Name,
		command,
		t.options.DownloadSecurityContext,
	)

	if err := t.destClient.Create(ctx, pod); err != nil {
		return nil, fmt.Errorf("failed to create download pod: %w", err)
	}
	return pod, nil
}

func (t *IndirectTransfer) IsDownloadComplete(ctx context.Context, podName, namespace string) (bool, error) {
	return isPodComplete(ctx, t.destClient, podName, namespace)
}
