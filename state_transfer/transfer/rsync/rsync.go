package rsync

import (
	"github.com/konveyor/crane-lib/state_transfer/endpoint"
	"github.com/konveyor/crane-lib/state_transfer/transfer"
	"github.com/konveyor/crane-lib/state_transfer/transport"
	"k8s.io/client-go/rest"
)

const (
	rsyncUser         = "crane2"
	rsyncImage        = "quay.io/konveyor/rsync-transfer:latest"
	rsyncPort         = int32(1873)
	rsyncConfigPrefix = "crane2-rsync-config-"
	rsyncSecretPrefix = "crane2-rsync-secret-"
)

type RsyncTransfer struct {
	username    string
	password    string
	source      *rest.Config
	destination *rest.Config
	pvcList     transfer.PersistentVolumeClaimList
	transport   transport.Transport
	endpoint    endpoint.Endpoint
	port        int32
	options     TransferOptions
}

func NewTransfer(t transport.Transport, e endpoint.Endpoint, src *rest.Config, dest *rest.Config,
	pvcList transfer.PersistentVolumeClaimList, opts ...TransferOption) (transfer.Transfer, error) {
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

func (r *RsyncTransfer) PVCs() transfer.PersistentVolumeClaimList {
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

func (r *RsyncTransfer) transferOptions() TransferOptions {
	return r.options
}
