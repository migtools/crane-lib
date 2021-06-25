package state_transfer

import (
	"context"
	"encoding/json"
	"strconv"
	"time"

	ocappsv1 "github.com/openshift/api/apps/v1"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	batchv1beta "k8s.io/api/batch/v1beta1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	NodeSelectorAnnotation = "migration.openshift.io/preQuiesceNodeSelector"
	QuiesceNodeSelector    = "migration.openshift.io/quiesceDaemonSet"
	ReplicasAnnotation     = "migration.openshift.io/preQuiesceReplicas"
	SuspendAnnotation      = "migration.openshift.io/preQuiesceSuspend"
)

// Quiesce applications on source cluster
func QuiesceApplications(cfg *rest.Config, ns string) error {
	scheme := runtime.NewScheme()
	if err := appsv1.AddToScheme(scheme); err != nil {
		return err
	}
	if err := ocappsv1.AddToScheme(scheme); err != nil {
		return err
	}
	if err := batchv1.AddToScheme(scheme); err != nil {
		return err
	}
	if err := batchv1beta.AddToScheme(scheme); err != nil {
		return err
	}

	c, err := client.New(cfg, client.Options{Scheme: scheme})
	if err != nil {
		return err
	}

	err = quiesceCronJobs(c, ns)
	if err != nil {
		return err
	}
	err = quiesceDeploymentConfigs(c, ns)
	if err != nil {
		return err
	}
	err = quiesceDeployments(c, ns)
	if err != nil {
		return err
	}
	err = quiesceStatefulSets(c, ns)
	if err != nil {
		return err
	}
	err = quiesceReplicaSets(c, ns)
	if err != nil {
		return err
	}
	err = quiesceDaemonSets(c, ns)
	if err != nil {
		return err
	}
	err = quiesceJobs(c, ns)
	if err != nil {
		return err
	}

	for {
		quiesced, err := ensureQuiescedPodsTerminated(c, ns)
		if err != nil {
			return err
		}
		if quiesced {
			break
		}
		time.Sleep(5 * time.Second)
	}

	return nil
}

func UnQuiesceApplications(c client.Client, ns string) error {
	err := unQuiesceApplications(c, ns)
	if err != nil {
		return err
	}
	return nil
}

// Unquiesce applications using client and namespace list given
func unQuiesceApplications(c client.Client, ns string) error {
	err := unQuiesceCronJobs(c, ns)
	if err != nil {
		return err
	}
	err = unQuiesceDeploymentConfigs(c, ns)
	if err != nil {
		return err
	}
	err = unQuiesceDeployments(c, ns)
	if err != nil {
		return err
	}
	err = unQuiesceStatefulSets(c, ns)
	if err != nil {
		return err
	}
	err = unQuiesceReplicaSets(c, ns)
	if err != nil {
		return err
	}
	err = unQuiesceDaemonSets(c, ns)
	if err != nil {
		return err
	}
	err = unQuiesceJobs(c, ns)
	if err != nil {
		return err
	}

	return nil
}

// Scales down DeploymentConfig on source cluster
func quiesceDeploymentConfigs(c client.Client, ns string) error {
	list := ocappsv1.DeploymentConfigList{}
	options := client.ListOptions{Namespace: ns}
	err := c.List(
		context.TODO(),
		&list,
		&options)
	if err != nil {
		return err
	}
	for _, dc := range list.Items {
		if dc.Annotations == nil {
			dc.Annotations = make(map[string]string)
		}
		if dc.Spec.Replicas == 0 {
			continue
		}
		dc.Annotations[ReplicasAnnotation] = strconv.FormatInt(int64(dc.Spec.Replicas), 10)
		dc.Spec.Replicas = 0
		err = c.Update(context.TODO(), &dc)
		if err != nil {
			return err
		}
	}

	return nil
}

// Scales DeploymentConfig back up on source cluster
func unQuiesceDeploymentConfigs(c client.Client, ns string) error {
	list := ocappsv1.DeploymentConfigList{}
	options := client.ListOptions{Namespace: ns}
	err := c.List(
		context.TODO(),
		&list,
		&options)
	if err != nil {
		return err
	}
	for _, dc := range list.Items {
		if dc.Annotations == nil {
			continue
		}
		replicas, exist := dc.Annotations[ReplicasAnnotation]
		if !exist {
			continue
		}
		number, err := strconv.Atoi(replicas)
		if err != nil {
			return err
		}
		delete(dc.Annotations, ReplicasAnnotation)
		// Only set replica count if currently 0
		if dc.Spec.Replicas == 0 {
			dc.Spec.Replicas = int32(number)
		}
		err = c.Update(context.TODO(), &dc)
		if err != nil {
			return err
		}
	}

	return nil
}

// Scales down all Deployments
func quiesceDeployments(c client.Client, ns string) error {
	zero := int32(0)
	list := appsv1.DeploymentList{}
	options := client.ListOptions{Namespace: ns}
	err := c.List(
		context.TODO(),
		&list,
		&options)
	if err != nil {
		return err
	}
	for _, deployment := range list.Items {
		if deployment.Annotations == nil {
			deployment.Annotations = make(map[string]string)
		}
		if *deployment.Spec.Replicas == zero {
			continue
		}
		deployment.Annotations[ReplicasAnnotation] = strconv.FormatInt(int64(*deployment.Spec.Replicas), 10)
		deployment.Spec.Replicas = &zero
		err = c.Update(context.TODO(), &deployment)
		if err != nil {
			return err
		}
	}

	return nil
}

// Scales all Deployments back up
func unQuiesceDeployments(c client.Client, ns string) error {
	list := appsv1.DeploymentList{}
	options := client.ListOptions{Namespace: ns}
	err := c.List(
		context.TODO(),
		&list,
		&options)
	if err != nil {
		return err
	}
	for _, deployment := range list.Items {
		if deployment.Annotations == nil {
			deployment.Annotations = make(map[string]string)
		}
		replicas, exist := deployment.Annotations[ReplicasAnnotation]
		if !exist {
			continue
		}
		number, err := strconv.Atoi(replicas)
		if err != nil {
			return err
		}
		delete(deployment.Annotations, ReplicasAnnotation)
		restoredReplicas := int32(number)
		// Only change replica count if currently == 0
		if *deployment.Spec.Replicas == 0 {
			deployment.Spec.Replicas = &restoredReplicas
		}
		err = c.Update(context.TODO(), &deployment)
		if err != nil {
			return err
		}
	}

	return nil
}

// Scales down all StatefulSets.
func quiesceStatefulSets(c client.Client, ns string) error {
	zero := int32(0)
	list := appsv1.StatefulSetList{}
	options := client.ListOptions{Namespace: ns}
	err := c.List(
		context.TODO(),
		&list,
		&options)
	if err != nil {
		return err
	}
	for _, set := range list.Items {
		if set.Annotations == nil {
			set.Annotations = make(map[string]string)
		}
		if *set.Spec.Replicas == zero {
			continue
		}
		set.Annotations[ReplicasAnnotation] = strconv.FormatInt(int64(*set.Spec.Replicas), 10)
		set.Spec.Replicas = &zero
		err = c.Update(context.TODO(), &set)
		if err != nil {
			return err
		}
	}
	return nil
}

// Scales all StatefulSets back up
func unQuiesceStatefulSets(c client.Client, ns string) error {
	list := appsv1.StatefulSetList{}
	options := client.ListOptions{Namespace: ns}
	err := c.List(
		context.TODO(),
		&list,
		&options)
	if err != nil {
		return err
	}
	for _, set := range list.Items {
		if set.Annotations == nil {
			continue
		}
		replicas, exist := set.Annotations[ReplicasAnnotation]
		if !exist {
			continue
		}
		number, err := strconv.Atoi(replicas)
		if err != nil {
			return err
		}
		delete(set.Annotations, ReplicasAnnotation)
		restoredReplicas := int32(number)

		// Only change replica count if currently == 0
		if *set.Spec.Replicas == 0 {
			set.Spec.Replicas = &restoredReplicas
		}
		err = c.Update(context.TODO(), &set)
		if err != nil {
			return err
		}
	}
	return nil
}

// Scales down all ReplicaSets.
func quiesceReplicaSets(c client.Client, ns string) error {
	zero := int32(0)
	list := appsv1.ReplicaSetList{}
	options := client.ListOptions{Namespace: ns}
	err := c.List(
		context.TODO(),
		&list,
		&options)
	if err != nil {
		return err
	}
	for _, set := range list.Items {
		if len(set.OwnerReferences) > 0 {
			continue
		}
		if set.Annotations == nil {
			set.Annotations = make(map[string]string)
		}
		if *set.Spec.Replicas == zero {
			continue
		}
		set.Annotations[ReplicasAnnotation] = strconv.FormatInt(int64(*set.Spec.Replicas), 10)
		set.Spec.Replicas = &zero
		err = c.Update(context.TODO(), &set)
		if err != nil {
			return err
		}
	}
	return nil
}

// Scales all ReplicaSets back up
func unQuiesceReplicaSets(c client.Client, ns string) error {
	list := appsv1.ReplicaSetList{}
	options := client.ListOptions{Namespace: ns}
	err := c.List(
		context.TODO(),
		&list,
		&options)
	if err != nil {
		return err
	}
	for _, set := range list.Items {
		if len(set.OwnerReferences) > 0 {
			continue
		}
		if set.Annotations == nil {
			continue
		}
		replicas, exist := set.Annotations[ReplicasAnnotation]
		if !exist {
			continue
		}
		number, err := strconv.Atoi(replicas)
		if err != nil {
			return err
		}
		delete(set.Annotations, ReplicasAnnotation)
		restoredReplicas := int32(number)
		// Only change replica count if currently == 0
		if *set.Spec.Replicas == 0 {
			set.Spec.Replicas = &restoredReplicas
		}
		err = c.Update(context.TODO(), &set)
		if err != nil {
			return err
		}
	}
	return nil
}

// Scales down all DaemonSets.
func quiesceDaemonSets(c client.Client, ns string) error {
	list := appsv1.DaemonSetList{}
	options := client.ListOptions{Namespace: ns}
	err := c.List(
		context.TODO(),
		&list,
		&options)
	if err != nil {
		return err
	}
	for _, set := range list.Items {
		if set.Annotations == nil {
			set.Annotations = make(map[string]string)
		}
		if set.Spec.Template.Spec.NodeSelector == nil {
			set.Spec.Template.Spec.NodeSelector = map[string]string{}
		} else if _, exist := set.Spec.Template.Spec.NodeSelector[QuiesceNodeSelector]; exist {
			continue
		}
		selector, err := json.Marshal(set.Spec.Template.Spec.NodeSelector)
		if err != nil {
			return err
		}
		set.Annotations[NodeSelectorAnnotation] = string(selector)
		set.Spec.Template.Spec.NodeSelector[QuiesceNodeSelector] = "true"
		err = c.Update(context.TODO(), &set)
		if err != nil {
			return err
		}
	}
	return nil
}

// Scales all DaemonSets back up
func unQuiesceDaemonSets(c client.Client, ns string) error {
	list := appsv1.DaemonSetList{}
	options := client.ListOptions{Namespace: ns}
	err := c.List(
		context.TODO(),
		&list,
		&options)
	if err != nil {
		return err
	}
	for _, set := range list.Items {
		if set.Annotations == nil {
			continue
		}
		selector, exist := set.Annotations[NodeSelectorAnnotation]
		if !exist {
			continue
		}
		nodeSelector := map[string]string{}
		err := json.Unmarshal([]byte(selector), &nodeSelector)
		if err != nil {
			return err
		}
		// Only change node selector if set to our quiesce nodeselector
		_, isQuiesced := set.Spec.Template.Spec.NodeSelector[QuiesceNodeSelector]
		if !isQuiesced {
			continue
		}
		delete(set.Annotations, NodeSelectorAnnotation)
		set.Spec.Template.Spec.NodeSelector = nodeSelector
		err = c.Update(context.TODO(), &set)
		if err != nil {
			return err
		}
	}
	return nil
}

// Suspends all CronJobs
func quiesceCronJobs(c client.Client, ns string) error {
	list := batchv1beta.CronJobList{}
	options := client.ListOptions{Namespace: ns}
	err := c.List(context.TODO(), &list, &options)
	if err != nil {
		return err
	}
	for _, r := range list.Items {
		if r.Annotations == nil {
			r.Annotations = make(map[string]string)
		}
		if r.Spec.Suspend == pointer.BoolPtr(true) {
			continue
		}
		r.Annotations[SuspendAnnotation] = "true"
		r.Spec.Suspend = pointer.BoolPtr(true)
		err = c.Update(context.TODO(), &r)
		if err != nil {
			return err
		}
	}

	return nil
}

// Undo quiescence on all CronJobs
func unQuiesceCronJobs(c client.Client, ns string) error {
	list := batchv1beta.CronJobList{}
	options := client.ListOptions{Namespace: ns}
	err := c.List(context.TODO(), &list, &options)
	if err != nil {
		return err
	}
	for _, r := range list.Items {
		if r.Annotations == nil {
			continue
		}
		// Only unsuspend if our suspend annotation is present
		if _, exist := r.Annotations[SuspendAnnotation]; !exist {
			continue
		}
		delete(r.Annotations, SuspendAnnotation)
		r.Spec.Suspend = pointer.BoolPtr(false)
		err = c.Update(context.TODO(), &r)
		if err != nil {
			return err
		}
	}

	return nil
}

// Scales down all Jobs
func quiesceJobs(c client.Client, ns string) error {
	zero := int32(0)
	list := batchv1.JobList{}
	options := client.ListOptions{Namespace: ns}
	err := c.List(
		context.TODO(),
		&list,
		&options)
	if err != nil {
		return err
	}
	for _, job := range list.Items {
		if job.Annotations == nil {
			job.Annotations = make(map[string]string)
		}
		if job.Spec.Parallelism == &zero {
			continue
		}
		job.Annotations[ReplicasAnnotation] = strconv.FormatInt(int64(*job.Spec.Parallelism), 10)
		job.Spec.Parallelism = &zero
		err = c.Update(context.TODO(), &job)
		if err != nil {
			return err
		}
	}

	return nil
}

// Scales all Jobs back up
func unQuiesceJobs(c client.Client, ns string) error {
	list := batchv1.JobList{}
	options := client.ListOptions{Namespace: ns}
	err := c.List(
		context.TODO(),
		&list,
		&options)
	if err != nil {
		return err
	}
	for _, job := range list.Items {
		if job.Annotations == nil {
			continue
		}
		replicas, exist := job.Annotations[ReplicasAnnotation]
		if !exist {
			continue
		}
		number, err := strconv.Atoi(replicas)
		if err != nil {
			return err
		}
		delete(job.Annotations, ReplicasAnnotation)
		parallelReplicas := int32(number)
		// Only change parallelism if currently == 0
		if *job.Spec.Parallelism == 0 {
			job.Spec.Parallelism = &parallelReplicas
		}
		err = c.Update(context.TODO(), &job)
		if err != nil {
			return err
		}
	}

	return nil
}

// Ensure scaled down pods have terminated.
// Returns: `true` when all pods terminated.
func ensureQuiescedPodsTerminated(c client.Client, ns string) (bool, error) {
	kinds := map[string]bool{
		"ReplicationController": true,
		"StatefulSet":           true,
		"ReplicaSet":            true,
		"DaemonSet":             true,
		"Job":                   true,
	}
	skippedPhases := map[v1.PodPhase]bool{
		v1.PodSucceeded: true,
		v1.PodFailed:    true,
		v1.PodUnknown:   true,
	}
	list := v1.PodList{}
	options := client.ListOptions{Namespace: ns}
	err := c.List(
		context.TODO(),
		&list,
		&options)
	if err != nil {
		return false, err
	}
	for _, pod := range list.Items {
		if pod.Annotations == nil {
			pod.Annotations = make(map[string]string)
		}
		if _, found := skippedPhases[pod.Status.Phase]; found {
			continue
		}
		for _, ref := range pod.OwnerReferences {
			if _, found := kinds[ref.Kind]; found {
				return false, nil
			}
		}
	}

	return true, nil
}
