package rsync

import (
	"github.com/konveyor/crane-lib/state_transfer/endpoint"
	"github.com/konveyor/crane-lib/state_transfer/transfer"
	"github.com/konveyor/crane-lib/state_transfer/transport"
	corev1 "k8s.io/api/core/v1"
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
	pvc         corev1.PersistentVolumeClaim
	transport   transport.Transport
	endpoint    endpoint.Endpoint
	port        int32
}

func CreateRsyncTransfer() *RsyncTransfer {
	return &RsyncTransfer{}
}

func NewTransfer(t transport.Transport, e endpoint.Endpoint, src *rest.Config, dest *rest.Config, pvc corev1.PersistentVolumeClaim) transfer.Transfer {
	return &RsyncTransfer{
		transport:   t,
		endpoint:    e,
		source:      src,
		destination: dest,
		pvc:         pvc,
	}
}

func (r *RsyncTransfer) PVC() corev1.PersistentVolumeClaim {
	return r.pvc
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
