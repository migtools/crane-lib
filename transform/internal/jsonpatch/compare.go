package jsonpatch

import (
	jsonpatch "github.com/evanphx/json-patch"
	"github.com/pkg/errors"
)

func Equal(patch1, patch2 jsonpatch.Patch) (bool, error) {
	found := []bool{}
	for _, o := range patch1 {
		for _, n := range patch2 {
			if EqualOperation(o, n) {
				found = append(found, true)
			}
		}
	}

	if len(found) == len(patch1) && len(found) == len(patch2) {
		return true, nil
	}
	return false, nil
}

func EqualOperation(operation1, operation2 jsonpatch.Operation) bool {
	if operation1.Kind() == operation2.Kind() {
		path1, err := operation1.Path()
		if err != nil {
			return false
		}
		path2, err := operation2.Path()
		if err != nil {
			return false
		}
		// If they are not the same, move to the operation
		if path1 != path2 {
			return false
		}
		val1, err := operation1.ValueInterface()
		err1 := errors.Cause(err)
		if err != nil && err1 != jsonpatch.ErrMissing {
			return false
		}
		val2, err := operation2.ValueInterface()
		err2 := errors.Cause(err)
		if err != nil && err2 != jsonpatch.ErrMissing {
			return false
		}
		if val1 != val2 && !(err2 == jsonpatch.ErrMissing && err1 == jsonpatch.ErrMissing) {
			return false
		}
		return true
	}
	return false
}

//TODO: diff function that makes it easy to determine the diffs between two things
