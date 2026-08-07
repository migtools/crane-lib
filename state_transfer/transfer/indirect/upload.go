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
	command := buildRcloneCommand("sync", dataMountPath, remotePath)

	pod := t.buildPod(
		fmt.Sprintf("rclone-upload-%s", pvc.Name),
		pvc.Namespace,
		pvc.Name,
		command,
		t.options.UploadSecurityContext,
	)

	if err := t.sourceClient.Create(ctx, pod); err != nil {
		return nil, fmt.Errorf("failed to create upload pod: %w", err)
	}
	return pod, nil
}

func (t *IndirectTransfer) IsUploadComplete(ctx context.Context, podName, namespace string) (bool, error) {
	return isPodComplete(ctx, t.sourceClient, podName, namespace)
}
