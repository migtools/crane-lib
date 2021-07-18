package meta

import "k8s.io/apimachinery/pkg/types"

type nnPair struct {
	src  types.NamespacedName
	dest types.NamespacedName
}

func (n nnPair) Source() types.NamespacedName {
	return n.src
}

func (n nnPair) Destination() types.NamespacedName {
	return n.dest
}

func NewNamespacedPair(src types.NamespacedName, dest types.NamespacedName) NamespacedNamePair {
	srcNN := src
	destNN := dest
	if len(dest.Name) == 0 {
		destNN.Name = src.Name
	}
	if len(dest.Namespace) == 0 {
		destNN.Namespace = src.Namespace
	}
	return nnPair{src: srcNN, dest: destNN}
}
