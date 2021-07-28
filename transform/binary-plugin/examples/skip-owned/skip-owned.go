package main

import (
	jsonpatch "github.com/evanphx/json-patch"
	"github.com/konveyor/crane-lib/transform"
	"github.com/konveyor/crane-lib/transform/cli"
	"github.com/konveyor/crane-lib/transform/types"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func main() {
	cli.RunAndExit(cli.NewCustomPlugin("SkipOwnedResourcesPlugin", "v1", nil, Run))
}

// Skips Pods and PodSpecable resources with owner references
// Currently doesn't check the type of owner or any labels/annotations to modify behavior.
func Run(u *unstructured.Unstructured, extras map[string]string) (transform.PluginResponse, error) {
	// plugin writers need to write custome code here.
	var patch jsonpatch.Patch
	var considerWhiteout, whiteout bool
	if u.GetKind() == "Pod" {
		considerWhiteout = true
	} else if _, isPodSpecable := types.IsPodSpecable(*u); isPodSpecable {
		considerWhiteout = true
	}
	if considerWhiteout && len(u.GetOwnerReferences()) > 0 {
		whiteout = considerWhiteout
	}
	return transform.PluginResponse{
		Version:    "v1",
		IsWhiteOut: whiteout,
		Patches:    patch,
	}, nil
}
