package meta

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type MutationType string

const (
	MutationTypeMerge   = "merge"
	MutationTypeReplace = "replace"
)

type mutation struct {
	t       MutationType
	podSpec *corev1.PodSpec
	meta    *metav1.ObjectMeta
	c       *corev1.Container
}

func (m *mutation) Type() MutationType {
	return m.t
}

func (m *mutation) PodSecurityContext() *corev1.PodSecurityContext {
	if m.podSpec == nil {
		return nil
	}
	return m.podSpec.SecurityContext
}

func (m *mutation) SecurityContext() *corev1.SecurityContext {
	if m.c == nil {
		return nil
	}
	return m.c.SecurityContext
}

func (m *mutation) NodeSelector() map[string]string {
	if m.podSpec == nil {
		return nil
	}
	return m.podSpec.NodeSelector
}

func (m *mutation) NodeName() *string {
	if m.podSpec == nil {
		return nil
	}
	return &m.podSpec.NodeName
}

func (m *mutation) Labels() map[string]string {
	if m.meta == nil {
		return nil
	}
	return m.meta.Labels
}

func (m *mutation) Annotations() map[string]string {
	if m.meta == nil {
		return nil
	}
	return m.meta.Annotations
}

func (m *mutation) Name() *string {
	if m.meta == nil {
		return nil
	}
	return &m.meta.Name
}

func (m *mutation) OwnerReferences() []metav1.OwnerReference {
	if m.meta == nil {
		return nil
	}
	return m.meta.OwnerReferences
}

func NewPodSpecMutation(spec *corev1.PodSpec, typ MutationType) PodSpecMutation {
	return &mutation{
		t:       typ,
		podSpec: spec,
	}
}

func NewObjectMetaMutation(objectMeta *metav1.ObjectMeta, typ MutationType) ObjectMetaMutation {
	return &mutation{
		t:    typ,
		meta: objectMeta,
	}
}

func NewContainerMutation(spec *corev1.Container, typ MutationType) ContainerMutation {
	return &mutation{
		t: typ,
		c: spec,
	}
}
