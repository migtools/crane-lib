package rclone

import (
	"github.com/konveyor/crane-lib/state_transfer/endpoint"
	"github.com/konveyor/crane-lib/state_transfer/transport"
	v1 "k8s.io/api/core/v1"
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
	pvc         v1.PersistentVolumeClaim
	transport   transport.Transport
	endpoint    endpoint.Endpoint
	port        int32
}

func CreateRcloneTransfer() *RcloneTransfer {
	return &RcloneTransfer{}
}

func (r *RcloneTransfer) PVC() v1.PersistentVolumeClaim {
	return r.pvc
}

func (r *RcloneTransfer) SetPVC(pvc v1.PersistentVolumeClaim) {
	r.pvc = pvc
}

func (r *RcloneTransfer) Endpoint() endpoint.Endpoint {
	return r.endpoint
}

func (r *RcloneTransfer) SetEndpoint(endpoint endpoint.Endpoint) {
	r.endpoint = endpoint
}

func (r *RcloneTransfer) Transport() transport.Transport {
	return r.transport
}

func (r *RcloneTransfer) SetTransport(transport transport.Transport) {
	r.transport = transport
}

func (r *RcloneTransfer) Source() *rest.Config {
	return r.source
}

func (r *RcloneTransfer) SetSource(source *rest.Config) {
	r.source = source
}

func (r *RcloneTransfer) Destination() *rest.Config {
	return r.destination
}

func (r *RcloneTransfer) SetDestination(destination *rest.Config) {
	r.destination = destination
}

func (r *RcloneTransfer) Username() string {
	return r.username
}

func (r *RcloneTransfer) SetUsername(username string) {
	r.username = username
}

func (r *RcloneTransfer) Password() string {
	return r.password
}

func (r *RcloneTransfer) SetPassword(password string) {
	r.password = password
}

func (r *RcloneTransfer) Port() int32 {
	return r.port
}

func (r *RcloneTransfer) SetPort(transferPort int32) {
	r.port = transferPort
}
