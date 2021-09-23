package transform

import (
	"encoding/json"

	jsonpatch "github.com/evanphx/json-patch"
	ijsonpatch "github.com/konveyor/crane-lib/transform/internal/jsonpatch"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

type Runner struct {
	// This is where we need to put extra info
	// This should include generic args to be passed to each Plugin
	// This also needs to handle the options that it will need.
	// TODO: Figure out options that the runner will need and implement here.
	PluginPriorities map[string]int
	OptionalFlags    map[string]string
	Log              *logrus.Logger
}

// RunnerResponse will be responsble for
// TransformFile is a marshaled jsonpatch.Patch
// IgnoredPatches is a marshaled []PluginOperation
type RunnerResponse struct {
	TransformFile  []byte
	HaveWhiteOut   bool
	IgnoredPatches []byte
}

type PluginOperation struct {
	PluginName string
	Operation  jsonpatch.Operation
}

func PluginOperationsFromPatch(pluginName string, patches jsonpatch.Patch) []PluginOperation {
	pluginOpList := []PluginOperation{}
	for _, op := range patches {
		pluginOpList = append(pluginOpList, PluginOperation{PluginName: pluginName, Operation: op})
	}
	return pluginOpList
}

func EqualPluginOperationList(pluginOps1, pluginOps2 []PluginOperation) bool {
	if len(pluginOps1) != len(pluginOps2) {
		return false
	}
	for i, op1 := range pluginOps1 {
		if !EqualPluginOperation(op1, pluginOps2[i]) {
			return false
		}
	}
	return true
}

func EqualPluginOperation(pluginOp1, pluginOp2 PluginOperation) bool {
	return pluginOp1.PluginName == pluginOp2.PluginName && ijsonpatch.EqualOperation(pluginOp1.Operation, pluginOp2.Operation)
}

func (r *Runner) Run(object unstructured.Unstructured, plugins []Plugin) (RunnerResponse, error) {
	haveWhiteOut := false
	havePatches := false
	patches := []PluginOperation{}
	errs := []error{}

	for _, plugin := range plugins {
		// We want to keep the original while we run each plugin.
		c := object.DeepCopy()
		// TODO: Handle Version things here
		resp, err := plugin.Run(PluginRequest{Unstructured:*c, Extras:r.OptionalFlags})
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
			patches = append(patches, PluginOperationsFromPatch(plugin.Metadata().Name, resp.Patches)...)
		}
	}
	response := RunnerResponse{
		TransformFile:  []byte(`[]`),
		HaveWhiteOut:   haveWhiteOut,
		IgnoredPatches: []byte(`[]`),
	}

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
		response.TransformFile, err = json.Marshal(patches)
		if err != nil {
			return response, err
		}
		response.IgnoredPatches, err = json.Marshal(ignoredPatches)
		if err != nil {
			return response, err
		}

		return response, err
	}
	return response, nil
}


// sanitizePatches removes duplicate patch operations as well as find
// conflicting operations where path is the same, but different kind or values.
// TODO: Handle where paths are the same, but operations are different.
func (r *Runner) sanitizePatches(pluginOps []PluginOperation) (jsonpatch.Patch, []PluginOperation, error) {
	patchMap := map[string]PluginOperation{}
	ignoredPatches := []PluginOperation{}
	for _, o := range pluginOps {
		key, err := o.Operation.Path()
		if err != nil {
			return nil, nil, err
		}
		if foundOp, ok := patchMap[key]; ok {
			currentPrio, currentOk := r.PluginPriorities[o.PluginName]
			previousPrio, previousOk := r.PluginPriorities[foundOp.PluginName]
			// replace value if current plugin is higher (lower int) priority than prior
			replaceVal := currentOk && (!previousOk || currentPrio < previousPrio)
			equalOp := ijsonpatch.EqualOperation(foundOp.Operation, o.Operation)
			// Handle Collision
			val, err := o.Operation.ValueInterface()
			err1 := errors.Cause(err)
			if err1 != nil && err1 != jsonpatch.ErrMissing {
				return nil, nil, err
			}
			previousVal, err := foundOp.Operation.ValueInterface()
			err1 = errors.Cause(err)
			if err1 != nil && err1 != jsonpatch.ErrMissing {
				return nil, nil, err
			}
			if replaceVal {
				patchMap[key] = o
			}
			if !equalOp {
				var selectedVal, rejectedVal interface{}
				var selectedPluginOp, rejectedPluginOp PluginOperation
				if replaceVal {
					selectedVal = val
					rejectedVal = previousVal
					selectedPluginOp = o
					rejectedPluginOp = foundOp
					ignoredPatches = append(ignoredPatches, foundOp)
				} else {
					selectedVal = previousVal
					rejectedVal = val
					selectedPluginOp = foundOp
					rejectedPluginOp = o
					ignoredPatches = append(ignoredPatches, o)
				}
				r.Log.Debugf("Operation on same path: %v with different kind or values selected kind, value: %v, %v (from plugin %v) kind, value that will be ignored: %v, %v (from plugin %v)",
					key,
					selectedPluginOp.Operation.Kind(),
					selectedVal,
					selectedPluginOp.PluginName,
					rejectedPluginOp.Operation.Kind(),
					rejectedVal,
					rejectedPluginOp.PluginName,
				)
			}
			continue
		}
		patchMap[key] = o
	}

	dedupedPatch := jsonpatch.Patch{}

	for _, p := range patchMap {
		dedupedPatch = append(dedupedPatch, p.Operation)
	}
	return dedupedPatch, ignoredPatches, nil
}
