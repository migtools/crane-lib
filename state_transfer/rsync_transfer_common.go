package state_transfer

import (
	v1 "k8s.io/api/core/v1"
	"k8s.io/client-go/rest"
)

const (
	rsyncUser  = "crane2"
	rsyncImage = "quay.io/konveyor/rsync-transfer:latest"
	rsyncPort  = int32(1873)
)

type RsyncTransfer struct {
	username     string
	password     string
	Source       *rest.Config
	Destination  *rest.Config
	PVC          v1.PersistentVolumeClaim
	Transport    Transport
	Endpoint     Endpoint
	TransferPort int32
}

func CreateRsyncTransfer() *RsyncTransfer {
	return &RsyncTransfer{}
}

func (r *RsyncTransfer) GetPVC() v1.PersistentVolumeClaim {
	return r.PVC
}

func (r *RsyncTransfer) SetPVC(pvc v1.PersistentVolumeClaim) {
	r.PVC = pvc
}

func (r *RsyncTransfer) GetEndpoint() Endpoint {
	return r.Endpoint
}

func (r *RsyncTransfer) SetEndpoint(endpoint Endpoint) {
	r.Endpoint = endpoint
}

func (r *RsyncTransfer) GetTransport() Transport {
	return r.Transport
}

func (r *RsyncTransfer) SetTransport(transport Transport) {
	r.Transport = transport
}

func (r *RsyncTransfer) GetSource() *rest.Config {
	return r.Source
}

func (r *RsyncTransfer) SetSource(source *rest.Config) {
	r.Source = source
}

func (r *RsyncTransfer) GetDestination() *rest.Config {
	return r.Destination
}

func (r *RsyncTransfer) SetDestination(destination *rest.Config) {
	r.Destination = destination
}

func (r *RsyncTransfer) GetUsername() string {
	return r.username
}

func (r *RsyncTransfer) SetUsername(username string) {
	r.username = username
}

func (r *RsyncTransfer) GetPassword() string {
	return r.password
}

func (r *RsyncTransfer) SetPassword(password string) {
	r.password = password
}

func (r *RsyncTransfer) GetTransferPort() int32 {
	return r.TransferPort
}

func (r *RsyncTransfer) SetTransferPort(transferPort int32) {
	r.TransferPort = transferPort
}
