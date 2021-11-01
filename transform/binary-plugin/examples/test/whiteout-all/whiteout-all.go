package main

import (
	jsonpatch "github.com/evanphx/json-patch"
	"github.com/konveyor/crane-lib/transform"
	"github.com/konveyor/crane-lib/transform/cli"
)

func main() {
	cli.RunAndExit(cli.NewCustomPlugin("WhiteoutPluginAll", "v1", nil, Run))
}

func Run(request transform.PluginRequest) (transform.PluginResponse, error) {
	// plugin writers need to write custome code here.
	var patch jsonpatch.Patch
	return transform.PluginResponse{
		Version:    "v1",
		IsWhiteOut: true,
		Patches:    patch,
	}, nil
}
