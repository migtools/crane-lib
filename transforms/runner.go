package transform

import (
	k8stransforms "github.com/konveyor/crane-lib/transforms/kubernetes"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

type Runner struct {
	AddedAnnotations    map[string]string
	RegistryReplacement map[string]string
	RemoveAnnotation    []string
	NewNamespace        string
}

func (r Runner) RunKubernetesTransforms(object unstructured.Unstructured) ([]byte, bool, error) {
	if k8stransforms.GetWhiteOuts(object.GetObjectKind().GroupVersionKind().GroupKind()) {
		return nil, true, nil
	}

	t, err := k8stransforms.GetKubernetesTransforms(object, r.AddedAnnotations, r.RegistryReplacement, r.NewNamespace, r.RemoveAnnotation)
	if err != nil {
		return nil, false, err
	}
	return t, false, nil
}
