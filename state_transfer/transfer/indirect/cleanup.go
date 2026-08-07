package indirect

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func (t *IndirectTransfer) Cleanup(ctx context.Context, c client.Client, namespace, pvcName string) error {
	if len(t.options.Labels) == 0 {
		return fmt.Errorf("refusing to cleanup with empty labels: would match all pods in namespace %s", namespace)
	}
	cleanupLabels := copyLabels(t.options.Labels)
	cleanupLabels["app.konveyor.io/created-for-pvc"] = pvcName
	podList := &corev1.PodList{}
	if err := c.List(ctx, podList,
		client.InNamespace(namespace),
		client.MatchingLabels(cleanupLabels)); err != nil {
		return fmt.Errorf("failed to list indirect transfer pods: %w", err)
	}
	for i := range podList.Items {
		if err := c.Delete(ctx, &podList.Items[i]); err != nil && !errors.IsNotFound(err) {
			return fmt.Errorf("failed to delete pod %s: %w", podList.Items[i].Name, err)
		}
	}
	return nil
}
