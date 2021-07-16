package rclone

import (
	"github.com/konveyor/crane-lib/state_transfer/endpoint"
	"github.com/konveyor/crane-lib/state_transfer/transfer"
	"github.com/konveyor/crane-lib/state_transfer/transport"
	"k8s.io/client-go/rest"
)

const (
	rcloneUser         = "crane2"
	rcloneImage        = "quay.io/jmontleon/rclone-transfer:latest"
	rclonePort         = int32(8080)
	rcloneConfigPrefix = "crane2-rclone-config-"
)

type RcloneTransfer struct {
	username    string
	password    string
	source      *rest.Config
	destination *rest.Config
	pvcList     transfer.PVCPairList
	transport   transport.Transport
	endpoint    endpoint.Endpoint
	port        int32
}

func NewTransfer(t transport.Transport, e endpoint.Endpoint, src *rest.Config, dest *rest.Config, pvcList transfer.PVCPairList) (transfer.Transfer, error) {
	err := validatePVCList(pvcList)
	if err != nil {
		return nil, err
	}
	return &RcloneTransfer{
		transport:   t,
		endpoint:    e,
		source:      src,
		destination: dest,
		pvcList:     pvcList,
	}, nil
}

func (r *RcloneTransfer) PVCs() transfer.PVCPairList {
	return r.pvcList
}

func (r *RcloneTransfer) Endpoint() endpoint.Endpoint {
	return r.endpoint
}

func (r *RcloneTransfer) Transport() transport.Transport {
	return r.transport
}

func (r *RcloneTransfer) Source() *rest.Config {
	return r.source
}

func (r *RcloneTransfer) Destination() *rest.Config {
	return r.destination
}

func (r *RcloneTransfer) Username() string {
	return r.username
}

func (r *RcloneTransfer) Password() string {
	return r.password
}
