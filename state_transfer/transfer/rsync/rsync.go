package rsync

import (
	"fmt"

	"github.com/go-logr/logr"
	"github.com/konveyor/crane-lib/state_transfer/endpoint"
	"github.com/konveyor/crane-lib/state_transfer/meta"
	"github.com/konveyor/crane-lib/state_transfer/transfer"
	"github.com/konveyor/crane-lib/state_transfer/transport"
	v1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	RsyncContainer = "rsync"
)

const (
	defaultRsyncUser         = "crane2"
	defaultRsyncImage        = "quay.io/konveyor/rsync-transfer:latest"
	rsyncPort                = int32(1873)
	defaultRsyncClientSecret = "crane2-rsync-client-secret"
	defaultRsyncServerConfig = "crane2-rsync-server-config"
	defaultRsyncServerSecret = "crane2-rsync-server-secret"
)

type RsyncTransfer struct {
	Log         logr.Logger
	username    string
	password    string
	source      client.Client
	destination client.Client
	pvcList     transfer.PVCPairList
	transport   transport.Transport
	endpoint    endpoint.Endpoint
	port        int32
	options     TransferOptions
}

func NewTransfer(t transport.Transport, e endpoint.Endpoint, src client.Client, dest client.Client,
	pvcList transfer.PVCPairList, log logr.Logger, opts ...TransferOption) (transfer.Transfer, error) {
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
		Log:         log,
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

func (r *RsyncTransfer) Source() client.Client {
	return r.source
}

func (r *RsyncTransfer) Destination() client.Client {
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

func (r *RsyncTransfer) getRsyncServerImage() string {
	if r.transferOptions().rsyncServerImage == "" {
		return defaultRsyncImage
	} else {
		return r.transferOptions().rsyncServerImage
	}
}

func (r *RsyncTransfer) getRsyncClientImage() string {
	if r.transferOptions().rsyncClientImage == "" {
		return defaultRsyncImage
	} else {
		return r.transferOptions().rsyncClientImage
	}
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
