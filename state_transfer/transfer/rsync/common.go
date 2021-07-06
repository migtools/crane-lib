package rsync

import (
	"github.com/konveyor/crane-lib/state_transfer/endpoint"
	"github.com/konveyor/crane-lib/state_transfer/transport"
	v1 "k8s.io/api/core/v1"
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
	pvc         v1.PersistentVolumeClaim
	transport   transport.Transport
	endpoint    endpoint.Endpoint
	port        int32
}

func CreateRsyncTransfer() *RsyncTransfer {
	return &RsyncTransfer{}
}

func (r *RsyncTransfer) PVC() v1.PersistentVolumeClaim {
	return r.pvc
}

func (r *RsyncTransfer) SetPVC(pvc v1.PersistentVolumeClaim) {
	r.pvc = pvc
}

func (r *RsyncTransfer) Endpoint() endpoint.Endpoint {
	return r.endpoint
}

func (r *RsyncTransfer) SetEndpoint(endpoint endpoint.Endpoint) {
	r.endpoint = endpoint
}

func (r *RsyncTransfer) Transport() transport.Transport {
	return r.transport
}

func (r *RsyncTransfer) SetTransport(transport transport.Transport) {
	r.transport = transport
}

func (r *RsyncTransfer) Source() *rest.Config {
	return r.source
}

func (r *RsyncTransfer) SetSource(source *rest.Config) {
	r.source = source
}

func (r *RsyncTransfer) Destination() *rest.Config {
	return r.destination
}

func (r *RsyncTransfer) SetDestination(destination *rest.Config) {
	r.destination = destination
}

func (r *RsyncTransfer) Username() string {
	return r.username
}

func (r *RsyncTransfer) SetUsername(username string) {
	r.username = username
}

func (r *RsyncTransfer) Password() string {
	return r.password
}

func (r *RsyncTransfer) SetPassword(password string) {
	r.password = password
}

func (r *RsyncTransfer) Port() int32 {
	return r.port
}

func (r *RsyncTransfer) SetPort(transferPort int32) {
	r.port = transferPort
}
