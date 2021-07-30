package rsync

import (
	"fmt"

	"github.com/konveyor/crane-lib/state_transfer/endpoint"
	"github.com/konveyor/crane-lib/state_transfer/meta"
	"github.com/konveyor/crane-lib/state_transfer/transfer"
	"github.com/konveyor/crane-lib/state_transfer/transport"
	v1 "k8s.io/api/core/v1"
	"k8s.io/client-go/rest"
)

const (
	RsyncContainer = "rsync"
)

const (
	rsyncUser         = "crane2"
	rsyncImage        = "quay.io/konveyor/rsync-transfer:latest"
	rsyncPort         = int32(1873)
	rsyncConfig       = "crane2-rsync-config"
	rsyncSecretPrefix = "crane2-rsync-secret"
)

type RsyncTransfer struct {
	username    string
	password    string
	source      *rest.Config
	destination *rest.Config
	pvcList     transfer.PVCPairList
	transport   transport.Transport
	endpoint    endpoint.Endpoint
	port        int32
	options     TransferOptions
}

func NewTransfer(t transport.Transport, e endpoint.Endpoint, src *rest.Config, dest *rest.Config,
	pvcList transfer.PVCPairList, opts ...TransferOption) (transfer.Transfer, error) {
	err := validatePVCList(pvcList)
	if err != nil {
		return nil, err
	}
	options := TransferOptions{}
	err = options.Apply(opts...)
	if err != nil {
		return nil, err
	}
	return &RsyncTransfer{
		transport:   t,
		endpoint:    e,
		source:      src,
		destination: dest,
		pvcList:     pvcList,
		options:     options,
	}, nil
}

func (r *RsyncTransfer) PVCs() transfer.PVCPairList {
	return r.pvcList
}

func (r *RsyncTransfer) Endpoint() endpoint.Endpoint {
	return r.endpoint
}

func (r *RsyncTransfer) Transport() transport.Transport {
	return r.transport
}

func (r *RsyncTransfer) Source() *rest.Config {
	return r.source
}

func (r *RsyncTransfer) Destination() *rest.Config {
	return r.destination
}

func (r *RsyncTransfer) Username() string {
	return r.username
}

func (r *RsyncTransfer) Password() string {
	return r.password
}

// transferOptions returns options used for the transfer
func (r *RsyncTransfer) transferOptions() TransferOptions {
	return r.options
}

// getMountPathForPVC given a PVC, returns a path where PVC can be mounted within a transfer Pod
func getMountPathForPVC(p transfer.PVC) string {
	return fmt.Sprintf("/mnt/%s/%s", p.Claim().Namespace, p.LabelSafeName())
}

// applyPodMutations given a pod spec and a list of podSpecMutation, applies
// each mutation to the given podSpec, only merge type mutations are allowed here
// Following fields will be mutated:
// - spec.NodeSelector
// - spec.SecurityContext
// - spec.NodeName
// - spec.Containers[i].SecurityContext
func applyPodMutations(podSpec *v1.PodSpec, ms []meta.PodSpecMutation) {
	for _, m := range ms {
		switch m.Type() {
		case meta.MutationTypeReplace:
			podSpec.NodeSelector = m.NodeSelector()
			if m.PodSecurityContext() != nil {
				podSpec.SecurityContext = m.PodSecurityContext()
			}
			if m.NodeName() != nil {
				podSpec.NodeName = *m.NodeName()
			}
		}
	}
}

func applyContainerMutations(container *v1.Container, ms []meta.ContainerMutation) {
	for _, m := range ms {
		switch m.Type() {
		case meta.MutationTypeReplace:
			if m.SecurityContext() != nil {
				container.SecurityContext = m.SecurityContext()
			}
			if m.Resources() != nil {
				container.Resources = *m.Resources()
			}
		}
	}
}
