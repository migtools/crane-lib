package apply

import (
	"fmt"

	jsonpatch "github.com/evanphx/json-patch"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

const (
	newAnnotationsPatch = `[{"op": "add", "path": "/metadata/annotations", "value": {}}]`
)

// We will need to eventualy have some set of options. These are not cureently defined.
type Applier struct {
}

// Apply will assume that if white out file already exists this will not be called.
// We will also assume that their is data in the patchedFileData and will check to make sure.
// Returns a byte array of valid kubernetes resource JSON.
func (a Applier) Apply(u unstructured.Unstructured, patchFileData []byte) ([]byte, error) {

	// Guard against invalid fileData
	if len(patchFileData) == 0 {
		return nil, fmt.Errorf("invalid patch file - no data")
	}

	// In the future, if there is more than json patches
	// then we will need to handle the diff here. For now
	// we don't have to.

	// for now, pull out the patches.

	patch, err := jsonpatch.DecodePatch(patchFileData)
	if err != nil {
		// In the future we will need to wrap these errors, so that users can make the right choice based on the jsonpatch errors
		return nil, err
	}

	// Get json document from unstrucutred

	doc, err := u.MarshalJSON()
	if err != nil {
		return nil, fmt.Errorf("invalid resource file - %v", err)
	}

	// Handle new Annotations
	// If there no annotations, we should create an empty annotations struct in the patches.
	// because they are omitempty.
	if len(u.GetAnnotations()) == 0 {
		annotationPatch, err := jsonpatch.DecodePatch([]byte(newAnnotationsPatch))
		if err != nil {
			// If we have a bad patch definition then this is a huge bug.
			return nil, err
		}
		doc, err = annotationPatch.Apply(doc)
		if err != nil {
			return nil, fmt.Errorf("unable to add empty annotations - %v", err)
		}
	}

	// Apply the rest of the patches
	doc, err = patch.Apply(doc)
	if err != nil {
		return nil, fmt.Errorf("unable to apply patches - %v", err)
	}

	//Validate the the doc can still be an unstrucutred Object.

	// This will also clean anything that does not fit into an object slightly, but that might be ok as a sanitazation step.

	err = u.UnmarshalJSON(doc)
	if err != nil {
		return nil, fmt.Errorf("unable to apply transformations to create a valid kubernetes object - %v", err)
	}

	return u.MarshalJSON()
}
