package state_transfer

import (
	v1 "k8s.io/api/core/v1"
	"k8s.io/client-go/rest"
)

const (
	rcloneUser  = "crane2"
	rcloneImage = "quay.io/jmontleon/rclone-transfer:latest"
	rclonePort  = int32(8080)
)

type RcloneTransfer struct {
	username     string
	password     string
	Source       *rest.Config
	Destination  *rest.Config
	PVC          v1.PersistentVolumeClaim
	Transport    Transport
	Endpoint     Endpoint
	TransferPort int32
}

func CreateRcloneTransfer() *RcloneTransfer {
	return &RcloneTransfer{}
}

func (r *RcloneTransfer) GetPVC() v1.PersistentVolumeClaim {
	return r.PVC
}

func (r *RcloneTransfer) SetPVC(pvc v1.PersistentVolumeClaim) {
	r.PVC = pvc
}

func (r *RcloneTransfer) GetEndpoint() Endpoint {
	return r.Endpoint
}

func (r *RcloneTransfer) SetEndpoint(endpoint Endpoint) {
	r.Endpoint = endpoint
}

func (r *RcloneTransfer) GetTransport() Transport {
	return r.Transport
}

func (r *RcloneTransfer) SetTransport(transport Transport) {
	r.Transport = transport
}

func (r *RcloneTransfer) GetSource() *rest.Config {
	return r.Source
}

func (r *RcloneTransfer) SetSource(source *rest.Config) {
	r.Source = source
}

func (r *RcloneTransfer) GetDestination() *rest.Config {
	return r.Destination
}

func (r *RcloneTransfer) SetDestination(destination *rest.Config) {
	r.Destination = destination
}

func (r *RcloneTransfer) GetUsername() string {
	return r.username
}

func (r *RcloneTransfer) SetUsername(username string) {
	r.username = username
}

func (r *RcloneTransfer) GetPassword() string {
	return r.password
}

func (r *RcloneTransfer) SetPassword(password string) {
	r.password = password
}

func (r *RcloneTransfer) GetTransferPort() int32 {
	return r.TransferPort
}

func (r *RcloneTransfer) SetTransferPort(transferPort int32) {
	r.TransferPort = transferPort
}
