package transform

import (
	"encoding/json"

	jsonpatch "github.com/evanphx/json-patch"
	ijsonpatch "github.com/konveyor/crane-lib/transform/internal/jsonpatch"
	"github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

type Runner struct {
	// This is where we need to put extra info
	// This should include generic args to be passed to each Plugin
	// This also needs to handle the options that it will need.
	// TODO: Figure out options that the runner will need and implement here.
	Log *logrus.Logger
}

// RunnerResponse will be responsble for
type RunnerResponse struct {
	TransformFile  []byte
	HaveWhiteOut   bool
	IgnoredPatches []byte
}

func (r *Runner) Run(object unstructured.Unstructured, plugins []Plugin) (RunnerResponse, error) {
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
			patches = append(patches, resp.Patches...)
		}
	}
	response := RunnerResponse{}

	// TODO: in the future we should consider a way to speed this up with go routines.
	if len(errs) > 0 {
		// TODO: handle error in a reasonable way. Probably needs an enhancement
		// Should Consider option to ignore errors
		return response, errs[0]
	}
	if haveWhiteOut {
		// TODO: handle if we should skip whiteOut if there is a transform
		response.HaveWhiteOut = haveWhiteOut
		return response, nil
	}

	if havePatches {
		patches, ignoredPatches, err := r.sanitizePatches(patches)
		if err != nil {
			return response, err
		}

		// for each patch, we should make sure the patch can be applied
		// We may need to break the transform file into two parts to handle this correctly
		if len(patches) != 0 {
			response.TransformFile, err = json.Marshal(patches)
			if err != nil {
				return response, err
			}
		}
		if len(ignoredPatches) != 0 {
			response.IgnoredPatches, err = json.Marshal(ignoredPatches)
			if err != nil {
				return response, err
			}
		}

		return response, err
	}
	return response, nil
}

type operatationKey struct {
	Kind string
	Path string
}

// sanitizePatches removes duplicate patch operatations as well as find
// conflicting operations where path and operation are the same, but different values.
// TODO: Handle where paths are the same, but operations are different.
func (r *Runner) sanitizePatches(patch jsonpatch.Patch) (jsonpatch.Patch, jsonpatch.Patch, error) {
	patchMap := map[operatationKey]jsonpatch.Operation{}
	ignoredPatches := jsonpatch.Patch{}
	for _, o := range patch {
		p, err := o.Path()
		if err != nil {
			return nil, nil, err
		}
		key := operatationKey{
			Kind: o.Kind(),
			Path: p,
		}
		if operation, ok := patchMap[key]; ok && !ijsonpatch.EqualOperation(operation, o) {
			// Handle Collision
			val, err := o.ValueInterface()
			if err != nil {
				return nil, nil, err
			}
			selectedVal, err := operation.ValueInterface()
			if err != nil {
				return nil, nil, err
			}
			r.Log.Debugf("Same operation: %v on path: %v with different values selected value: %v value that will be ignored: %v", key.Kind, key.Path, selectedVal, val)
			ignoredPatches = append(ignoredPatches, operation)
		}
		patchMap[key] = o
	}

	dedupedPatch := jsonpatch.Patch{}

	for _, p := range patchMap {
		dedupedPatch = append(dedupedPatch, p)
	}
	return dedupedPatch, ignoredPatches, nil
}
