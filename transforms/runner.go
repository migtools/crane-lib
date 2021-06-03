package transform

import (
	"encoding/json"

	jsonpatch "github.com/evanphx/json-patch"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

type Runner struct {
	// This is where we need to put extra info
	// This should include generic args to be passed to each Plugin
	// This also needs to handle the options that it will need.
	// TODO: Figure out options that the runner will need and implement here.
}

func (r *Runner) Run(object unstructured.Unstructured, plugins []Plugin) ([]byte, bool, error) {
	haveWhiteOut := false
	havePatches := false
	patches := jsonpatch.Patch{}
	errs := []error{}

	for _, plugin := range plugins {
		// We want to keep the original while we run each plugin.
		c := object.DeepCopy()
		// TODO: Handle Version things here
		resp, err := plugin.Run(c)
		if err != nil {
			//TODO: add debug level logging here
			errs = append(errs, err)
			continue
		}
		if resp.IsWhiteOut {
			haveWhiteOut = true
		}
		if len(resp.Patches) > 0 {
			havePatches = true
			patches = append(patches, resp.Patches)
		}
	}
	// TODO: in the future we should consider a way to speed this up with go routines.
	if len(errs) > 0 {
		// TODO: handle error in a reasonable way. Probably needs an enhancement
		// Should Consider option to ignore errors
		return nil, false, err[0]
	}
	if haveWhiteOut {
		// TODO: handle if we should skip whiteOut if there is a transform
		return nil, true, nil
	}
	if havePatches {
		// TODO: Handle dedup
		// TODO: Handle conflicts with paths
		b, err := json.Marshal(patches)
		return b
	}
}
