package kubernetes

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	jsonpatch "github.com/evanphx/json-patch"
	transform "github.com/konveyor/crane-lib/transform"
	"github.com/konveyor/crane-lib/transform/types"
	"github.com/konveyor/crane-lib/transform/util"
	"github.com/konveyor/crane-lib/version"
	"github.com/sirupsen/logrus"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	v1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/util/validation"
)

var logger logrus.FieldLogger

const (
	AddAnnotationsFlag           = "add-annotations"
	RemoveAnnotationsFlag        = "remove-annotations"
	RegistryReplacementFlag      = "registry-replacement"
	ExtraWhiteoutsFlag           = "extra-whiteouts"
	IncludeOnlyFlag              = "include-only"
	DisableWhiteoutOwnedFlag     = "disable-whiteout-owned"
	StripDefaultRBACFlag         = "strip-default-rbac"
	StripDefaultCABundleFlag     = "strip-default-cabundle"
	PVCRenameMap                 = "pvc-rename-map"
	PVCStorageClassMap           = "pvc-storage-class-map"
	WhiteoutPVCFlag              = "whiteout-pvc"
	DownscaleWorkloadsFlag       = "downscale-workloads"
	CraneJobIdempotentAnnotation = "crane.konveyor.io/job-idempotent"
	OriginalReplicasAnnotation   = "crane.konveyor.io/original-replicas"
)

const (
	containerImageUpdate        = "/spec/template/spec/containers/%v/image"
	initContainerImageUpdate    = "/spec/template/spec/initContainers/%v/image"
	podContainerImageUpdate     = "/spec/containers/%v/image"
	podInitContainerImageUpdate = "/spec/initContainers/%v/image"
	annotationInitial           = `%v
{"op": "add", "path": "/metadata/annotations/%v", "value": "%v"}`
	annotationNext = `%v,
{"op": "add", "path": "/metadata/annotations/%v", "value": "%v"}`
	removeAnnotationInitial = `%v
{"op": "remove", "path": "/metadata/annotations/%v"}`
	removeAnnotationNext = `%v,
{"op": "remove", "path": "/metadata/annotations/%v"}`

	opRemove = `[
{"op": "remove", "path": "%v"}
]`
	opReplace = `[
{"op": "replace", "path": "%v", "value": "%v"}
]`
	metadata             = "metadata"
	podNodeName          = "/spec/nodeName"
	podNodeSelector      = "/spec/nodeSelector"
	podPriority          = "/spec/priority"
	pvcVolumeName        = "/spec/volumeName"
	pvcFinalizers        = "/metadata/finalizers"
	pvcStorageClassName  = "/spec/storageClassName"
	roleBindingSubject   = "/subjects/%d/namespace"
	updateClusterIP      = "/spec/clusterIP"
	updateClusterIPs     = "/spec/clusterIPs"
	updateExternalIPs    = "/spec/externalIPs"
	updateNodePortString = "/spec/ports/%v/nodePort"
)

var fieldsToStrip = [...][]string{
	{metadata, "uid"},
	{metadata, "selfLink"},
	{metadata, "resourceVersion"},
	{metadata, "creationTimestamp"},
	{metadata, "generation"},
	{metadata, "managedFields"},
	{metadata, "annotations", "kubectl.kubernetes.io/last-applied-configuration"},
	{metadata, "annotations", "batch.kubernetes.io/controller-uid"},
	{metadata, "annotations", "controller-uid"},
	{metadata, "labels", "batch.kubernetes.io/controller-uid"},
	{metadata, "labels", "controller-uid"},
	{"status"},
}

// GroupKinds we are likely to interact with
var (
	configMapGK             = schema.GroupKind{Group: "", Kind: "ConfigMap"}
	cronJobGK               = schema.GroupKind{Group: "batch", Kind: "CronJob"}
	daemonSetGK             = schema.GroupKind{Group: "apps", Kind: "DaemonSet"}
	deploymentGK            = schema.GroupKind{Group: "apps", Kind: "Deployment"}
	endpointGK              = schema.GroupKind{Group: "", Kind: "Endpoints"}
	endpointSliceGK         = schema.GroupKind{Group: "discovery.k8s.io", Kind: "EndpointSlice"}
	eventGK                 = schema.GroupKind{Group: "", Kind: "Event"}
	eventsK8sGK             = schema.GroupKind{Group: "events.k8s.io", Kind: "Event"}
	extensionsGroup         = "extensions"
	jobGK                   = schema.GroupKind{Group: "batch", Kind: "Job"}
	pvcGK                   = schema.GroupKind{Group: "", Kind: "PersistentVolumeClaim"}
	podGK                   = schema.GroupKind{Group: "", Kind: "Pod"}
	replicaSetGK            = schema.GroupKind{Group: "apps", Kind: "ReplicaSet"}
	replicationControllerGK = schema.GroupKind{Group: "", Kind: "ReplicationController"}
	roleBindingGK           = schema.GroupKind{Group: "rbac.authorization.k8s.io", Kind: "RoleBinding"}
	serviceGK               = schema.GroupKind{Group: "", Kind: "Service"}
	secretGK                = schema.GroupKind{Group: "", Kind: "Secret"}
	serviceAccountGK        = schema.GroupKind{Group: "", Kind: "ServiceAccount"}
	statefulSetGK           = schema.GroupKind{Group: "apps", Kind: "StatefulSet"}

	// OLM / Operator Lifecycle Manager (operators.coreos.com) — lifecycle artifacts;
	subscriptionGK          = schema.GroupKind{Group: "operators.coreos.com", Kind: "Subscription"}
	installPlanGK           = schema.GroupKind{Group: "operators.coreos.com", Kind: "InstallPlan"}
	clusterServiceVersionGK = schema.GroupKind{Group: "operators.coreos.com", Kind: "ClusterServiceVersion"}
	catalogSourceGK         = schema.GroupKind{Group: "operators.coreos.com", Kind: "CatalogSource"}
	operatorGroupGK         = schema.GroupKind{Group: "operators.coreos.com", Kind: "OperatorGroup"}
	operatorConditionGK     = schema.GroupKind{Group: "operators.coreos.com", Kind: "OperatorCondition"}
)

var gksToWhiteout = []schema.GroupKind{
	endpointGK,
	endpointSliceGK,
	eventGK,
	eventsK8sGK,
	subscriptionGK,
	installPlanGK,
	clusterServiceVersionGK,
	catalogSourceGK,
	operatorGroupGK,
	operatorConditionGK,
}

type KubernetesTransformPlugin struct {
	AddAnnotations       map[string]string
	RemoveAnnotations    []string
	RegistryReplacement  map[string]string
	DisableWhiteoutOwned bool
	ExtraWhiteouts       []schema.GroupKind
	IncludeOnly          []schema.GroupKind
	StripDefaultRBAC     bool
	StripDefaultCABundle bool
	PVCRenameMap         map[string]string
	PVCStorageClassMap   map[string]string
	WhiteoutPVC          bool
	DownscaleWorkloads   bool
}

func (k *KubernetesTransformPlugin) Run(request transform.PluginRequest) (transform.PluginResponse, error) {
	logger = logrus.New()
	resp := transform.PluginResponse{}
	err := k.setOptionalFields(request.Extras)
	if err != nil {
		return resp, err
	}
	// Set version in the future
	resp.Version = string(transform.V1)
	resp.IsWhiteOut = k.getWhiteOuts(request.Unstructured)
	if resp.IsWhiteOut {
		return resp, nil
	}
	resp.Patches, err = k.getKubernetesTransforms(request.Unstructured)
	return resp, err

}

func (k *KubernetesTransformPlugin) Metadata() transform.PluginMetadata {
	return transform.PluginMetadata{
		Name:            "KubernetesPlugin",
		Version:         version.Version,
		RequestVersion:  []transform.Version{transform.V1},
		ResponseVersion: []transform.Version{transform.V1},
		OptionalFields: []transform.OptionalFields{
			{
				FlagName: AddAnnotationsFlag,
				Help:     "Annotations to add to each resource",
				Example:  "annotation1=value1,annotation2=value2",
			},
			{
				FlagName: RegistryReplacementFlag,
				Help:     "Map of image registry paths to swap on transform, in the format original-registry1=target-registry1,original-registry2=target-registry2...",
				Example:  "docker-registry.default.svc:5000=image-registry.openshift-image-registry.svc:5000,docker.io/foo=quay.io/bar",
			},
			{
				FlagName: RemoveAnnotationsFlag,
				Help:     "Annotations to remove",
				Example:  "annotation1,annotation2",
			},
			{
				FlagName: DisableWhiteoutOwnedFlag,
				Help:     "Disable whiting out owned pods and pod template resources",
				Example:  "true",
			},
			{
				FlagName: ExtraWhiteoutsFlag,
				Help:     "Additional resources to whiteout specified as a comma-separated list of GroupKind strings.",
				Example:  "Deployment.apps,Service,Route.route.openshift.io",
			},
			{
				FlagName: IncludeOnlyFlag,
				Help:     "If specified, every resource not listed here will be a whiteout. extra-whiteouts is ignored when include-only is specified. Specified as a comma-separated list of GroupKind strings.",
				Example:  "Deployment.apps,Service,Route.route.openshift.io",
			},
			{
				FlagName: StripDefaultRBACFlag,
				Help:     "Whether to strip default RBAC including default serviceAccount (default: true)",
				Example:  "true",
			},
			{
				FlagName: StripDefaultCABundleFlag,
				Help:     "Whether to strip default CA Bundle (default: true)",
				Example:  "true",
			},
			{
				FlagName: PVCRenameMap,
				Help:     "A comma-separated list of colon separated pvc renames.",
				Example:  "old-pvc1-name:new-pvc1-name,old-pvc2-name:new-pvc2-name",
			},
			{
				FlagName: PVCStorageClassMap,
				Help:     "A comma-separated list of colon separated StorageClass replacements for PVCs and StatefulSet volumeClaimTemplates.",
				Example:  "old-storage-class:new-storage-class,standard:fast",
			},
			{
				FlagName: WhiteoutPVCFlag,
				Help:     "Whiteout PersistentVolumeClaims without owner references instead of including them in transformed output (PVCs with owner references are always whiteouted).",
				Example:  "true",
			},
			{
				FlagName: DownscaleWorkloadsFlag,
				Help:     "Scale PVC-consuming workloads to zero and store their original replica count in annotations.",
				Example:  "true",
			},
		},
	}
}

func (k *KubernetesTransformPlugin) setOptionalFields(extras map[string]string) error {
	// first set defaults as necessary
	k.StripDefaultRBAC = true
	k.StripDefaultCABundle = true

	if len(extras[AddAnnotationsFlag]) > 0 {
		k.AddAnnotations = transform.ParseOptionalFieldMapVal(extras[AddAnnotationsFlag])
	}
	if len(extras[RemoveAnnotationsFlag]) > 0 {
		k.RemoveAnnotations = transform.ParseOptionalFieldSliceVal(extras[RemoveAnnotationsFlag])
	}
	if len(extras[RegistryReplacementFlag]) > 0 {
		k.RegistryReplacement = transform.ParseOptionalFieldMapVal(extras[RegistryReplacementFlag])
	}
	if len(extras[ExtraWhiteoutsFlag]) > 0 {
		k.ExtraWhiteouts = parseGroupKindSlice(transform.ParseOptionalFieldSliceVal(extras[ExtraWhiteoutsFlag]))
	}
	if len(extras[IncludeOnlyFlag]) > 0 {
		k.IncludeOnly = parseGroupKindSlice(transform.ParseOptionalFieldSliceVal(extras[IncludeOnlyFlag]))
	}
	if len(extras[DisableWhiteoutOwnedFlag]) > 0 {
		var err error
		k.DisableWhiteoutOwned, err = strconv.ParseBool(extras[DisableWhiteoutOwnedFlag])
		if err != nil {
			k.DisableWhiteoutOwned = false
		}
	}
	if len(extras[StripDefaultRBACFlag]) > 0 {
		k.StripDefaultRBAC, _ = strconv.ParseBool(extras[StripDefaultRBACFlag])
	}
	if len(extras[StripDefaultCABundleFlag]) > 0 {
		k.StripDefaultCABundle, _ = strconv.ParseBool(extras[StripDefaultCABundleFlag])
	}
	if len(extras[PVCRenameMap]) > 0 {
		pvcMap, err := util.ProcessPVCMap(extras[PVCRenameMap])
		if err != nil {
			return err
		}
		k.PVCRenameMap = pvcMap
	}
	if len(extras[PVCStorageClassMap]) > 0 {
		storageClassMap, err := parsePVCStorageClassMap(extras[PVCStorageClassMap])
		if err != nil {
			return err
		}
		k.PVCStorageClassMap = storageClassMap
	}
	if err := setBoolFlag(extras, WhiteoutPVCFlag, &k.WhiteoutPVC); err != nil {
		return err
	}
	if err := setBoolFlag(extras, DownscaleWorkloadsFlag, &k.DownscaleWorkloads); err != nil {
		return err
	}
	if k.DownscaleWorkloads {
		if _, found := k.AddAnnotations[OriginalReplicasAnnotation]; found {
			return fmt.Errorf("annotation %q is reserved by %s and cannot be set with %s", OriginalReplicasAnnotation, DownscaleWorkloadsFlag, AddAnnotationsFlag)
		}
		for _, annotation := range k.RemoveAnnotations {
			if annotation == OriginalReplicasAnnotation {
				return fmt.Errorf("annotation %q is reserved by %s and cannot be removed with %s", OriginalReplicasAnnotation, DownscaleWorkloadsFlag, RemoveAnnotationsFlag)
			}
		}
	}
	return nil
}

func setBoolFlag(extras map[string]string, flagName string, destination *bool) error {
	value, found := extras[flagName]
	if !found || value == "" {
		return nil
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return fmt.Errorf("invalid %s value %q: %w", flagName, value, err)
	}
	*destination = parsed
	return nil
}

func parsePVCStorageClassMap(value string) (map[string]string, error) {
	storageClassMap := map[string]string{}
	for _, pair := range strings.Split(value, ",") {
		parts := strings.SplitN(pair, ":", 2)
		if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
			return nil, fmt.Errorf("invalid StorageClass mapping %q: expected source:destination", pair)
		}
		for _, storageClass := range parts {
			if errs := validation.IsDNS1123Subdomain(storageClass); len(errs) > 0 {
				return nil, fmt.Errorf("invalid StorageClass name %q: %s", storageClass, strings.Join(errs, ", "))
			}
		}
		storageClassMap[parts[0]] = parts[1]
	}
	return storageClassMap, nil
}

var _ transform.Plugin = &KubernetesTransformPlugin{}

func (k *KubernetesTransformPlugin) getWhiteOuts(obj unstructured.Unstructured) bool {
	groupKind := obj.GroupVersionKind().GroupKind()
	if k.WhiteoutPVC && groupKind == pvcGK {
		return true
	}
	if k.DownscaleWorkloads && groupKind == podGK && hasPVCVolume(obj, "spec", "volumes") {
		return true
	}
	if len(k.IncludeOnly) > 0 {
		if !groupKindInList(groupKind, k.IncludeOnly) {
			return true
		}
	} else {
		if groupKindInList(groupKind, gksToWhiteout) {
			return true
		}
		if groupKindInList(groupKind, k.ExtraWhiteouts) {
			return true
		}
	}
	if groupKind == secretGK {
		if secretType, found, _ := unstructured.NestedString(obj.Object, "type"); found {
			switch secretType {
			case "kubernetes.io/service-account-token", "kubernetes.io/dockercfg":
				if _, generatedByServiceAccount := obj.GetAnnotations()["kubernetes.io/service-account.name"]; generatedByServiceAccount {
					return true
				}
			}
		}
	}
	if k.DisableWhiteoutOwned {
		return false
	}
	if len(obj.GetOwnerReferences()) > 0 {
		return true
	}
	// drop the default serviceaccount
	if groupKind == serviceAccountGK && obj.GetName() == "default" && k.StripDefaultRBAC {
		return true
	}
	// drop any Secrets belonging to default serviceaccount
	if groupKind == secretGK && k.StripDefaultRBAC {
		if sa, ok := obj.GetAnnotations()["kubernetes.io/service-account.name"]; ok && sa == "default" {
			return true
		}
	}
	// drop kube-root-ca.crt configmap
	if groupKind == configMapGK && obj.GetName() == "kube-root-ca.crt" && k.StripDefaultCABundle {
		return true
	}

	if groupKind.Group == extensionsGroup {
		return true
	}

	return false
}

func parseGroupKindSlice(gkStrings []string) []schema.GroupKind {
	gks := []schema.GroupKind{}
	for _, gk := range gkStrings {
		gks = append(gks, schema.ParseGroupKind(gk))
	}
	return gks
}

func groupKindInList(gk schema.GroupKind, list []schema.GroupKind) bool {
	for _, thisGK := range list {
		if gk == thisGK {
			return true
		}
	}
	return false
}

func isScalableWorkload(groupKind schema.GroupKind) bool {
	return groupKind == deploymentGK || groupKind == statefulSetGK || groupKind == replicaSetGK || groupKind == replicationControllerGK
}

func hasPVCVolume(obj unstructured.Unstructured, fields ...string) bool {
	volumes, found, err := unstructured.NestedSlice(obj.Object, fields...)
	if err != nil || !found {
		return false
	}
	for _, volume := range volumes {
		volumeMap, ok := volume.(map[string]interface{})
		if !ok {
			continue
		}
		if _, found, _ := unstructured.NestedMap(volumeMap, "persistentVolumeClaim"); found {
			return true
		}
	}
	return false
}

func hasPVCWorkload(obj unstructured.Unstructured) bool {
	if hasPVCVolume(obj, "spec", "template", "spec", "volumes") {
		return true
	}
	if obj.GroupVersionKind().GroupKind() != statefulSetGK {
		return false
	}
	volumeClaimTemplates, found, err := unstructured.NestedSlice(obj.Object, "spec", "volumeClaimTemplates")
	return err == nil && found && len(volumeClaimTemplates) > 0
}

func downscaleWorkload(obj unstructured.Unstructured, addedAnnotations map[string]string) (jsonpatch.Patch, error) {
	replicas, found, err := unstructured.NestedInt64(obj.Object, "spec", "replicas")
	if err != nil {
		return nil, err
	}
	if !found {
		replicas = 1
	}
	if replicas == 0 {
		return nil, nil
	}

	annotations, annotationsFound, err := unstructured.NestedStringMap(obj.Object, "metadata", "annotations")
	if err != nil {
		return nil, err
	}
	annotationPath := "/metadata/annotations/" + escapeJSONPointer(OriginalReplicasAnnotation)
	annotationOperation := "add"
	annotationValue := interface{}(map[string]string{OriginalReplicasAnnotation: strconv.FormatInt(replicas, 10)})
	if annotationsFound {
		annotationValue = strconv.FormatInt(replicas, 10)
		if _, found := annotations[OriginalReplicasAnnotation]; found {
			annotationOperation = "replace"
		}
	} else {
		annotationPath = "/metadata/annotations"
		annotations := annotationValue.(map[string]string)
		for key, value := range addedAnnotations {
			annotations[key] = value
		}
	}

	patches, err := valuePatch(annotationOperation, annotationPath, annotationValue)
	if err != nil {
		return nil, err
	}
	replicaOperation := "replace"
	if !found {
		replicaOperation = "add"
	}
	replicaPatch, err := valuePatch(replicaOperation, "/spec/replicas", 0)
	if err != nil {
		return nil, err
	}
	patches = append(patches, replicaPatch...)
	if annotationsFound && len(addedAnnotations) > 0 {
		annotationPatches, err := addAnnotations(addedAnnotations)
		if err != nil {
			return nil, err
		}
		patches = append(patches, annotationPatches...)
	}
	return patches, nil
}

func valuePatch(operation, path string, value interface{}) (jsonpatch.Patch, error) {
	patchJSON, err := json.Marshal([]map[string]interface{}{{"op": operation, "path": path, "value": value}})
	if err != nil {
		return nil, err
	}
	return jsonpatch.DecodePatch(patchJSON)
}

func (k *KubernetesTransformPlugin) getKubernetesTransforms(obj unstructured.Unstructured) (jsonpatch.Patch, error) {
	// Always attempt to add annotations for each thing.
	jsonPatch := jsonpatch.Patch{}
	patches, err := stripFields(obj)
	if err != nil {
		return nil, err
	}
	jsonPatch = append(jsonPatch, patches...)
	shouldDownscaleWorkload := k.DownscaleWorkloads && isScalableWorkload(obj.GroupVersionKind().GroupKind()) && hasPVCWorkload(obj)
	if !shouldDownscaleWorkload && k.AddAnnotations != nil && len(k.AddAnnotations) > 0 {
		patches, err := addAnnotations(k.AddAnnotations)
		if err != nil {
			return nil, err
		}
		jsonPatch = append(jsonPatch, patches...)
	}
	if len(k.RemoveAnnotations) > 0 {
		patches, err := removeAnnotations(k.RemoveAnnotations)
		if err != nil {
			return nil, err
		}
		jsonPatch = append(jsonPatch, patches...)
	}
	if shouldDownscaleWorkload {
		patches, err := downscaleWorkload(obj, k.AddAnnotations)
		if err != nil {
			return nil, err
		}
		jsonPatch = append(jsonPatch, patches...)
	}
	if cronJobGK == obj.GetObjectKind().GroupVersionKind().GroupKind() {
		js, err := obj.MarshalJSON()
		if err != nil {
			return nil, err
		}
		cronJob := &batchv1.CronJob{}
		err = json.Unmarshal(js, cronJob)
		if err != nil {
			return nil, err
		}

		patches, err := util.RenamePVCs(cronJob.Spec.JobTemplate.Spec.Template.Spec.Volumes, k.PVCRenameMap, util.PVCPathCronJobString)
		if err != nil {
			return nil, err
		}
		jsonPatch = append(jsonPatch, patches...)
	}

	if podGK == obj.GetObjectKind().GroupVersionKind().GroupKind() {
		js, err := obj.MarshalJSON()
		if err != nil {
			return nil, err
		}
		pod := &v1.Pod{}
		err = json.Unmarshal(js, pod)
		if err != nil {
			return nil, err
		}

		patches, err := removePodFields()
		if err != nil {
			return nil, err
		}
		jsonPatch = append(jsonPatch, patches...)

		patches, err = util.RenamePVCs(pod.Spec.Volumes, k.PVCRenameMap, util.PVCPathPodString)
		if err != nil {
			return nil, err
		}
		jsonPatch = append(jsonPatch, patches...)
	}
	if daemonSetGK == obj.GetObjectKind().GroupVersionKind().GroupKind() {
		js, err := obj.MarshalJSON()
		if err != nil {
			return nil, err
		}
		daemonSet := &appsv1.DaemonSet{}
		err = json.Unmarshal(js, daemonSet)
		if err != nil {
			return nil, err
		}

		patches, err := util.RenamePVCs(daemonSet.Spec.Template.Spec.Volumes, k.PVCRenameMap, util.PVCPathGenericString)
		if err != nil {
			return nil, err
		}
		jsonPatch = append(jsonPatch, patches...)
	}
	if deploymentGK == obj.GetObjectKind().GroupVersionKind().GroupKind() {
		js, err := obj.MarshalJSON()
		if err != nil {
			return nil, err
		}
		deployment := &appsv1.Deployment{}
		err = json.Unmarshal(js, deployment)
		if err != nil {
			return nil, err
		}

		patches, err := util.RenamePVCs(deployment.Spec.Template.Spec.Volumes, k.PVCRenameMap, util.PVCPathGenericString)
		if err != nil {
			return nil, err
		}
		jsonPatch = append(jsonPatch, patches...)
	}
	if jobGK == obj.GetObjectKind().GroupVersionKind().GroupKind() {
		js, err := obj.MarshalJSON()
		if err != nil {
			return nil, err
		}
		job := &batchv1.Job{}
		err = json.Unmarshal(js, job)
		if err != nil {
			return nil, err
		}

		patches, err := util.RenamePVCs(job.Spec.Template.Spec.Volumes, k.PVCRenameMap, util.PVCPathGenericString)
		if err != nil {
			return nil, err
		}
		jsonPatch = append(jsonPatch, patches...)

		patches, err = removeJobControllerUID(obj)
		if err != nil {
			return nil, err
		}
		jsonPatch = append(jsonPatch, patches...)

		patches, err = suspendStandaloneJob(obj)
		if err != nil {
			return nil, err
		}
		jsonPatch = append(jsonPatch, patches...)
	}
	if replicationControllerGK == obj.GetObjectKind().GroupVersionKind().GroupKind() {
		js, err := obj.MarshalJSON()
		if err != nil {
			return nil, err
		}
		replicationController := &v1.ReplicationController{}
		err = json.Unmarshal(js, replicationController)
		if err != nil {
			return nil, err
		}

		patches, err := util.RenamePVCs(replicationController.Spec.Template.Spec.Volumes, k.PVCRenameMap, util.PVCPathGenericString)
		if err != nil {
			return nil, err
		}
		jsonPatch = append(jsonPatch, patches...)
	}
	if replicaSetGK == obj.GetObjectKind().GroupVersionKind().GroupKind() {
		js, err := obj.MarshalJSON()
		if err != nil {
			return nil, err
		}
		replicaSet := &appsv1.ReplicaSet{}
		err = json.Unmarshal(js, replicaSet)
		if err != nil {
			return nil, err
		}

		patches, err := util.RenamePVCs(replicaSet.Spec.Template.Spec.Volumes, k.PVCRenameMap, util.PVCPathGenericString)
		if err != nil {
			return nil, err
		}
		jsonPatch = append(jsonPatch, patches...)
	}
	if roleBindingGK == obj.GetObjectKind().GroupVersionKind().GroupKind() {
		js, err := obj.MarshalJSON()
		if err != nil {
			return nil, err
		}
		rb := &rbacv1.RoleBinding{}

		err = json.Unmarshal(js, rb)
		if err != nil {
			return nil, err
		}
		for i, subj := range rb.Subjects {
			if subj.Kind == "ServiceAccount" && subj.Namespace == rb.Namespace {
				subjPath := fmt.Sprintf(roleBindingSubject, i)
				patch, err := jsonpatch.DecodePatch([]byte(fmt.Sprintf(opRemove, subjPath)))
				if err != nil {
					return nil, err
				}
				jsonPatch = append(jsonPatch, patch...)
			}
		}
	}
	if statefulSetGK == obj.GetObjectKind().GroupVersionKind().GroupKind() {
		js, err := obj.MarshalJSON()
		if err != nil {
			return nil, err
		}
		statefulSet := &appsv1.StatefulSet{}
		err = json.Unmarshal(js, statefulSet)
		if err != nil {
			return nil, err
		}

		patches, err := util.RenamePVCs(statefulSet.Spec.Template.Spec.Volumes, k.PVCRenameMap, util.PVCPathGenericString)
		if err != nil {
			return nil, err
		}
		jsonPatch = append(jsonPatch, patches...)

		patches, err = renamePVCTemplates(statefulSet.Spec.VolumeClaimTemplates, k.PVCRenameMap, util.PVCPathTemplateString)
		if err != nil {
			return nil, err
		}
		jsonPatch = append(jsonPatch, patches...)

		patches, err = replacePVCStorageClasses(statefulSet.Spec.VolumeClaimTemplates, k.PVCStorageClassMap, "/spec/volumeClaimTemplates/%d/spec/storageClassName")
		if err != nil {
			return nil, err
		}
		jsonPatch = append(jsonPatch, patches...)
	}
	if pvcGK == obj.GetObjectKind().GroupVersionKind().GroupKind() {
		volumeMode, found, err := unstructured.NestedString(obj.Object, "spec", "volumeMode")
		if err != nil {
			return nil, err
		}
		if found && volumeMode != string(v1.PersistentVolumeFilesystem) {
			logger.WithFields(logrus.Fields{
				"namespace":  obj.GetNamespace(),
				"name":       obj.GetName(),
				"volumeMode": volumeMode,
			}).Warn("PVC volumeMode other than Filesystem is not supported by transfer-pvc")
		}

		patches, err := removePVCFields(obj)
		if err != nil {
			return nil, err
		}
		jsonPatch = append(jsonPatch, patches...)

		patches, err = replacePVCStorageClass(obj, k.PVCStorageClassMap)
		if err != nil {
			return nil, err
		}
		jsonPatch = append(jsonPatch, patches...)

		patches, err = renamePVCManifest(obj, k.PVCRenameMap)
		if err != nil {
			return nil, err
		}
		jsonPatch = append(jsonPatch, patches...)
	}
	if serviceAccountGK == obj.GetObjectKind().GroupVersionKind().GroupKind() {
		if _, found, _ := unstructured.NestedSlice(obj.Object, "secrets"); found {
			patch, err := jsonpatch.DecodePatch([]byte(fmt.Sprintf(opRemove, "/secrets")))
			if err != nil {
				return nil, err
			}
			jsonPatch = append(jsonPatch, patch...)
		}
	}
	if k.RegistryReplacement != nil && len(k.RegistryReplacement) > 0 {
		if podGK == obj.GetObjectKind().GroupVersionKind().GroupKind() {
			js, err := obj.MarshalJSON()
			if err != nil {
				return nil, err
			}
			pod := &v1.Pod{}
			err = json.Unmarshal(js, pod)
			if err != nil {
				return nil, err
			}
			jps := jsonpatch.Patch{}
			for i, container := range pod.Spec.Containers {
				updatedImage, update := util.UpdateImageRegistry(k.RegistryReplacement, container.Image)
				if update {
					jp, err := util.UpdateImage(fmt.Sprintf(podContainerImageUpdate, i), updatedImage)
					if err != nil {
						return nil, err
					}
					jps = append(jps, jp...)
				}
			}
			for i, container := range pod.Spec.InitContainers {
				updatedImage, update := util.UpdateImageRegistry(k.RegistryReplacement, container.Image)
				if update {
					jp, err := util.UpdateImage(fmt.Sprintf(podInitContainerImageUpdate, i), updatedImage)
					if err != nil {
						return nil, err
					}
					jps = append(jps, jp...)
				}
			}
			jsonPatch = append(jsonPatch, jps...)
		} else if template, ok := types.IsPodSpecable(obj); ok {
			jps := jsonpatch.Patch{}
			for i, container := range template.Spec.Containers {
				updatedImage, update := util.UpdateImageRegistry(k.RegistryReplacement, container.Image)
				if update {
					jp, err := util.UpdateImage(fmt.Sprintf(containerImageUpdate, i), updatedImage)
					if err != nil {
						return nil, err
					}
					jps = append(jps, jp...)
				}
			}
			for i, container := range template.Spec.InitContainers {
				updatedImage, update := util.UpdateImageRegistry(k.RegistryReplacement, container.Image)
				if update {
					jp, err := util.UpdateImage(fmt.Sprintf(initContainerImageUpdate, i), updatedImage)
					if err != nil {
						return nil, err
					}
					jps = append(jps, jp...)
				}
			}
			jsonPatch = append(jsonPatch, jps...)
		}
	}
	if obj.GetObjectKind().GroupVersionKind().GroupKind() == serviceGK {
		patches, err := removeServiceFields(obj)
		if err != nil {
			return nil, err
		}
		jsonPatch = append(jsonPatch, patches...)
	}

	return jsonPatch, nil
}

// escapeJSONPointer escapes a string for use in a JSON Pointer path according to RFC 6901
// ~ must be escaped as ~0
// / must be escaped as ~1
func escapeJSONPointer(s string) string {
	s = strings.ReplaceAll(s, "~", "~0")
	s = strings.ReplaceAll(s, "/", "~1")
	return s
}

func stripFields(obj unstructured.Unstructured) (jsonpatch.Patch, error) {
	var patches jsonpatch.Patch
	for _, field := range fieldsToStrip {
		_, found, err := unstructured.NestedFieldNoCopy(obj.Object, field...)
		if err != nil {
			return patches, err
		}
		if found {
			// Build the JSON Pointer path with proper escaping
			var pathParts []string
			for _, f := range field {
				pathParts = append(pathParts, escapeJSONPointer(f))
			}
			path := "/" + strings.Join(pathParts, "/")
			patch, err := jsonpatch.DecodePatch([]byte(fmt.Sprintf(opRemove, path)))
			if err != nil {
				return nil, err
			}
			patches = append(patches, patch...)
		}
	}
	return patches, nil
}

func removePVCFields(obj unstructured.Unstructured) (jsonpatch.Patch, error) {
	var patches jsonpatch.Patch
	for _, path := range [][]string{{"spec", "volumeName"}, {"metadata", "finalizers"}} {
		if _, found, err := unstructured.NestedFieldNoCopy(obj.Object, path...); err != nil {
			return nil, err
		} else if found {
			patch, err := jsonpatch.DecodePatch([]byte(fmt.Sprintf(opRemove, "/"+strings.Join(path, "/"))))
			if err != nil {
				return nil, err
			}
			patches = append(patches, patch...)
		}
	}

	annotations, found, err := unstructured.NestedStringMap(obj.Object, "metadata", "annotations")
	if err != nil || !found {
		return patches, err
	}
	keys := make([]string, 0, len(annotations))
	for key := range annotations {
		if isServerManagedPVCAnnotation(key) {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	for _, key := range keys {
		patch, err := jsonpatch.DecodePatch([]byte(fmt.Sprintf(opRemove, "/metadata/annotations/"+escapeJSONPointer(key))))
		if err != nil {
			return nil, err
		}
		patches = append(patches, patch...)
	}
	return patches, nil
}

func isServerManagedPVCAnnotation(key string) bool {
	for _, prefix := range []string{
		"pv.kubernetes.io/",
		"volume.kubernetes.io/",
		"volume.beta.kubernetes.io/",
	} {
		if strings.HasPrefix(key, prefix) {
			return true
		}
	}
	return false
}

func replacePVCStorageClass(obj unstructured.Unstructured, storageClassMap map[string]string) (jsonpatch.Patch, error) {
	storageClass, found, err := unstructured.NestedString(obj.Object, "spec", "storageClassName")
	if err != nil || !found {
		return nil, err
	}
	replacement, ok := storageClassMap[storageClass]
	if !ok {
		return nil, nil
	}
	return jsonpatch.DecodePatch([]byte(fmt.Sprintf(opReplace, pvcStorageClassName, replacement)))
}

func renamePVCManifest(obj unstructured.Unstructured, pvcRenameMap map[string]string) (jsonpatch.Patch, error) {
	replacement, found := pvcRenameMap[obj.GetName()]
	if !found {
		return nil, nil
	}
	return valuePatch("replace", "/metadata/name", replacement)
}

func replacePVCStorageClasses(volumes []v1.PersistentVolumeClaim, storageClassMap map[string]string, path string) (jsonpatch.Patch, error) {
	var patches jsonpatch.Patch
	for i, volume := range volumes {
		if volume.Spec.StorageClassName == nil {
			continue
		}
		replacement, ok := storageClassMap[*volume.Spec.StorageClassName]
		if !ok {
			continue
		}
		patch, err := jsonpatch.DecodePatch([]byte(fmt.Sprintf(opReplace, fmt.Sprintf(path, i), replacement)))
		if err != nil {
			return nil, err
		}
		patches = append(patches, patch...)
	}
	return patches, nil
}

func addAnnotations(addAnnotations map[string]string) (jsonpatch.Patch, error) {
	patchJSON := `[`
	i := 0
	for key, value := range addAnnotations {
		if i == 0 {
			patchJSON = fmt.Sprintf(annotationInitial, patchJSON, key, value)
		} else {
			patchJSON = fmt.Sprintf(annotationNext, patchJSON, key, value)
		}
		i++
	}

	patchJSON = fmt.Sprintf("%v]", patchJSON)
	patch, err := jsonpatch.DecodePatch([]byte(patchJSON))
	if err != nil {
		fmt.Printf("%v", patchJSON)
		return nil, err
	}
	return patch, nil
}

func removeAnnotations(removeAnnotations []string) (jsonpatch.Patch, error) {
	patchJSON := `[`
	i := 0
	for _, annotation := range removeAnnotations {
		if i == 0 {
			patchJSON = fmt.Sprintf(removeAnnotationInitial, patchJSON, annotation)
		} else {
			patchJSON = fmt.Sprintf(removeAnnotationNext, patchJSON, annotation)
		}
		i++
	}

	patchJSON = fmt.Sprintf("%v]", patchJSON)
	patch, err := jsonpatch.DecodePatch([]byte(patchJSON))
	if err != nil {
		fmt.Printf("%v", patchJSON)
		return nil, err
	}
	return patch, nil
}

func removePodFields() (jsonpatch.Patch, error) {
	var patches jsonpatch.Patch
	patches, err := jsonpatch.DecodePatch([]byte(fmt.Sprintf(opRemove, podNodeName)))
	if err != nil {
		return nil, err
	}
	patch, err := jsonpatch.DecodePatch([]byte(fmt.Sprintf(opRemove, podNodeSelector)))
	if err != nil {
		return nil, err
	}
	patches = append(patches, patch...)
	patch, err = jsonpatch.DecodePatch([]byte(fmt.Sprintf(opRemove, podPriority)))
	if err != nil {
		return nil, err
	}
	patches = append(patches, patch...)
	return patches, nil
}

func renamePVCTemplates(volumes []v1.PersistentVolumeClaim, PVCRenameMap map[string]string, path string) (jsonpatch.Patch, error) {
	var patches jsonpatch.Patch
	if len(PVCRenameMap) > 0 && len(volumes) > 0 {
		for i, volume := range volumes {
			if pvcName, ok := PVCRenameMap[volume.Name]; ok {
				pvcTemplatePath := fmt.Sprintf(path, i)
				patch, err := jsonpatch.DecodePatch([]byte(fmt.Sprintf(opReplace, pvcTemplatePath, pvcName)))
				if err != nil {
					return nil, err
				}
				patches = append(patches, patch...)
			}
		}
	}
	return patches, nil
}

func removeServiceFields(obj unstructured.Unstructured) (jsonpatch.Patch, error) {
	var patches jsonpatch.Patch
	if isLoadBalancerService(obj) {
		patch, err := jsonpatch.DecodePatch([]byte(fmt.Sprintf(opRemove, updateExternalIPs)))
		if err != nil {
			return nil, err
		}
		patches = append(patches, patch...)
	}

	if shouldRemoveServiceClusterIP(obj) {
		patch, err := jsonpatch.DecodePatch([]byte(fmt.Sprintf(opRemove, updateClusterIP)))
		if err != nil {
			return nil, err
		}
		patches = append(patches, patch...)
	}
	if shouldRemoveServiceClusterIPs(obj) {
		patch, err := jsonpatch.DecodePatch([]byte(fmt.Sprintf(opRemove, updateClusterIPs)))
		if err != nil {
			return nil, err
		}
		patches = append(patches, patch...)
	}
	patch, err := getNodePortPatch(obj)
	if err != nil {
		return nil, err
	}
	patches = append(patches, patch...)
	return patches, nil
}

func isLoadBalancerService(u unstructured.Unstructured) bool {
	// Get Spec
	spec, ok := u.UnstructuredContent()["spec"]
	if !ok {
		return false
	}

	specMap, ok := spec.(map[string]interface{})
	if !ok {
		return false
	}
	// Get type
	serviceType, ok := specMap["type"]
	if !ok {
		return false
	}
	return serviceType == "LoadBalancer"
}

func shouldRemoveServiceClusterIP(u unstructured.Unstructured) bool {
	// Get Spec
	spec, ok := u.UnstructuredContent()["spec"]
	if !ok {
		return false
	}

	specMap, ok := spec.(map[string]interface{})
	if !ok {
		return false
	}
	clusterIP, ok := specMap["clusterIP"]
	if !ok {
		return false
	}
	return clusterIP != "None"
}

func shouldRemoveServiceClusterIPs(u unstructured.Unstructured) bool {
	// Get Spec
	spec, ok := u.UnstructuredContent()["spec"]
	if !ok {
		return false
	}

	specMap, ok := spec.(map[string]interface{})
	if !ok {
		return false
	}
	clusterIPs, ok := specMap["clusterIPs"]
	if !ok {
		return false
	}
	// At this point, we have clusterIPs. Remove unless there's a first None element

	clusterIPsSlice, ok := clusterIPs.([]interface{})
	if !ok {
		return true
	}
	if len(clusterIPsSlice) == 0 {
		return true
	}
	return (clusterIPsSlice[0] != "None")
}

func getNodePortPatch(u unstructured.Unstructured) (jsonpatch.Patch, error) {
	var patch jsonpatch.Patch
	// Get Spec
	spec, ok := u.UnstructuredContent()["spec"]
	if !ok {
		return patch, nil
	}
	specMap, ok := spec.(map[string]interface{})
	if !ok {
		return patch, nil
	}
	// Get type
	serviceType, ok := specMap["type"]
	if !ok {
		return patch, nil
	}
	if serviceType == "ExternalName" {
		return patch, nil
	}
	servicePorts, ok := specMap["ports"]
	if !ok {
		return patch, nil
	}
	portsSlice, ok := servicePorts.([]interface{})
	if !ok {
		return patch, nil
	}

	explicitNodePorts := sets.NewString()
	unnamedPortInts := sets.NewInt()
	lastAppliedConfig, ok := u.GetAnnotations()["kubectl.kubernetes.io/last-applied-configuration"]
	if ok {
		appliedServiceUnstructured := new(map[string]interface{})
		if err := json.Unmarshal([]byte(lastAppliedConfig), appliedServiceUnstructured); err != nil {
			return patch, err
		}

		ports, bool, err := unstructured.NestedSlice(*appliedServiceUnstructured, "spec", "ports")

		if err != nil {
			return patch, err
		}

		if bool {
			for _, port := range ports {
				p, ok := port.(map[string]interface{})
				if !ok {
					continue
				}
				nodePort, nodePortBool, err := unstructured.NestedFieldNoCopy(p, "nodePort")
				if err != nil {
					return patch, err
				}
				if nodePortBool {
					nodePortInt, err := getNodePortInt(nodePort)
					if err != nil {
						return patch, err
					}
					if nodePortInt > 0 {
						portName, ok := p["name"]
						if !ok {
							// unnamed port
							unnamedPortInts.Insert(nodePortInt)
						} else {
							explicitNodePorts.Insert(portName.(string))
						}

					}
				}
			}
		}
	}

	for i, portInterface := range portsSlice {
		removeNodePort := false
		var nameStr string
		port, ok := portInterface.(map[string]interface{})
		if !ok {
			continue
		}
		name, ok := port["name"]
		if ok {
			nameStr, _ = name.(string)
		}
		nodePort, ok := port["nodePort"]
		if !ok {
			continue
		}
		nodePortInt, err := getNodePortInt(nodePort)
		if err != nil {
			return patch, err
		}
		if nodePortInt == 0 {
			continue
		}
		if len(nameStr) > 0 {
			if !explicitNodePorts.Has(nameStr) {
				removeNodePort = true
			}
		} else {
			if !unnamedPortInts.Has(int(nodePortInt)) {
				removeNodePort = true
			}
		}
		if removeNodePort {
			patchJSON := fmt.Sprintf(opRemove, fmt.Sprintf(updateNodePortString, i))
			intPatch, err := jsonpatch.DecodePatch([]byte(patchJSON))
			if err != nil {
				return patch, err
			}
			patch = append(patch, intPatch...)
		}
	}

	return patch, nil
}

func getNodePortInt(nodePort interface{}) (int, error) {
	nodePortInt := 0
	switch nodePort.(type) {
	case int:
		nodePortInt = nodePort.(int)
	case int32:
		nodePortInt = int(nodePort.(int32))
	case int64:
		nodePortInt = int(nodePort.(int64))
	case float64:
		nodePortInt = int(nodePort.(float64))
	case string:
		nodePortInt, err := strconv.Atoi(nodePort.(string))
		if err != nil {
			return nodePortInt, err
		}
	}
	return nodePortInt, nil
}

func removeJobControllerUID(obj unstructured.Unstructured) (jsonpatch.Patch, error) {
	var patches jsonpatch.Patch

	// Controller UID keys to remove (both legacy and current)
	controllerUIDKeys := []string{
		"batch.kubernetes.io/controller-uid",
		"controller-uid",
	}

	// Check manualSelector value
	manualSelector, found, err := unstructured.NestedBool(obj.Object, "spec", "manualSelector")
	if err != nil {
		return patches, err
	}

	// If manualSelector is false or not set, remove the entire spec.selector
	// Kubernetes will auto-generate it
	if !found || !manualSelector {
		_, selectorFound, err := unstructured.NestedMap(obj.Object, "spec", "selector")
		if err != nil {
			return patches, err
		}
		if selectorFound {
			patch, err := jsonpatch.DecodePatch([]byte(fmt.Sprintf(opRemove, "/spec/selector")))
			if err != nil {
				return nil, err
			}
			patches = append(patches, patch...)
		}
	} else {
		// If manualSelector is true, remove both controller-uid keys from matchLabels
		for _, key := range controllerUIDKeys {
			_, found, err := unstructured.NestedString(obj.Object, "spec", "selector", "matchLabels", key)
			if err != nil {
				return patches, err
			}
			if found {
				path := "/spec/selector/matchLabels/" + escapeJSONPointer(key)
				patch, err := jsonpatch.DecodePatch([]byte(fmt.Sprintf(opRemove, path)))
				if err != nil {
					return nil, err
				}
				patches = append(patches, patch...)
			}
		}
	}

	// Remove both controller-uid keys from spec.template.metadata.labels
	for _, key := range controllerUIDKeys {
		_, found, err := unstructured.NestedString(obj.Object, "spec", "template", "metadata", "labels", key)
		if err != nil {
			return patches, err
		}
		if found {
			path := "/spec/template/metadata/labels/" + escapeJSONPointer(key)
			patch, err := jsonpatch.DecodePatch([]byte(fmt.Sprintf(opRemove, path)))
			if err != nil {
				return nil, err
			}
			patches = append(patches, patch...)
		}
	}

	return patches, nil
}

// suspendStandaloneJob suspends standalone Jobs by default to prevent unintended re-execution
// on the target cluster after migration.
//
// Behavior:
//   - Jobs with ownerReferences (e.g., created by CronJob) are NOT suspended - they are whited out
//   - Jobs with annotation "crane.konveyor.io/job-idempotent: true" are NOT suspended
//   - All other standalone Jobs are suspended (spec.suspend: true)
//
// Rationale:
// Kubernetes best practices expect Jobs to be idempotent, but in practice many Jobs are not
// (e.g., one-shot data imports, billing operations). Re-running such Jobs on migration can cause
// data corruption or duplicate charges. By suspending Jobs by default, we provide a safe migration
// path where users can explicitly opt-in idempotent Jobs via annotation, or manually unsuspend
// Jobs on the target cluster after reviewing their purpose.
//
// Related: https://github.com/migtools/crane/issues/482
func suspendStandaloneJob(obj unstructured.Unstructured) (jsonpatch.Patch, error) {
	var patches jsonpatch.Patch

	// Check if this is a standalone Job (no ownerReferences)
	if len(obj.GetOwnerReferences()) > 0 {
		// Job has owner (e.g., created by CronJob) - don't suspend
		return patches, nil
	}

	// Check if Job is explicitly marked as idempotent
	annotations := obj.GetAnnotations()
	if annotations[CraneJobIdempotentAnnotation] == "true" {
		// Job is idempotent, safe to run - don't suspend
		return patches, nil
	}

	// Check if Job is already suspended
	suspended, found, err := unstructured.NestedBool(obj.Object, "spec", "suspend")
	if err != nil {
		return patches, err
	}

	// If suspend field doesn't exist or is currently false, set it to true
	if !found || !suspended {
		patch, err := jsonpatch.DecodePatch([]byte(`[{"op": "add", "path": "/spec/suspend", "value": true}]`))
		if err != nil {
			return nil, err
		}
		patches = append(patches, patch...)
	}

	return patches, nil
}
