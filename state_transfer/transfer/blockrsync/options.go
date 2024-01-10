package blockrsync

import "github.com/konveyor/crane-lib/state_transfer/transfer"

type TransferOptions struct {
	SourcePodMeta         transfer.ResourceMetadata
	DestinationPodMeta    transfer.ResourceMetadata
	username              string
	password              string
	blockrsyncServerImage string
	blockrsyncClientImage string
}

func (t *TransferOptions) GetBlockrsyncServerImage() string {
	if t.blockrsyncServerImage == "" {
		return blockrsyncImage
	}
	return t.blockrsyncServerImage
}

func (t *TransferOptions) GetBlockrsyncClientImage() string {
	if t.blockrsyncClientImage == "" {
		return blockrsyncImage
	}
	return t.blockrsyncClientImage
}
