package blockrsync

import (
	"github.com/go-logr/logr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/konveyor/crane-lib/state_transfer/endpoint"
	"github.com/konveyor/crane-lib/state_transfer/transfer"
	"github.com/konveyor/crane-lib/state_transfer/transport"
)

const (
	blockrsyncImage     = "quay.io/awels/blockrsync:latest"
	volumeName          = "volume"
	BlockRsyncContainer = "blockrsync"
	Proxy               = "proxy"
)

type BlockrsyncTransfer struct {
	log             logr.Logger
	username        string
	password        string
	source          client.Client
	destination     client.Client
	pvcList         transfer.PVCPairList
	transport       transport.Transport
	endpoint        endpoint.Endpoint
	transferOptions *TransferOptions
}

func NewTransfer(t transport.Transport, e endpoint.Endpoint, src client.Client,
	dest client.Client, pvcList transfer.PVCPairList, log logr.Logger, options *TransferOptions) (transfer.Transfer, error) {
	err := validatePVCList(pvcList)
	if err != nil {
		return nil, err
	}
	return &BlockrsyncTransfer{
		log:             log,
		transport:       t,
		endpoint:        e,
		source:          src,
		destination:     dest,
		pvcList:         pvcList,
		transferOptions: options,
	}, nil
}

func (r *BlockrsyncTransfer) PVCs() transfer.PVCPairList {
	return r.pvcList
}

func (r *BlockrsyncTransfer) Endpoint() endpoint.Endpoint {
	return r.endpoint
}

func (r *BlockrsyncTransfer) Transport() transport.Transport {
	return r.transport
}

func (r *BlockrsyncTransfer) Source() client.Client {
	return r.source
}

func (r *BlockrsyncTransfer) Destination() client.Client {
	return r.destination
}

func (r *BlockrsyncTransfer) Username() string {
	return r.username
}

func (r *BlockrsyncTransfer) Password() string {
	return r.password
}
