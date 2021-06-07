package jsonpatch

import (
	jsonpatch "github.com/evanphx/json-patch"
	"github.com/pkg/errors"
)

func Equal(patch1, patch2 jsonpatch.Patch) (bool, error) {
	found := []bool{}
	for _, o := range patch1 {
		for _, n := range patch2 {
			if o.Kind() == n.Kind() {
				path1, err := o.Path()
				if err != nil {
					return false, err
				}
				path2, err := n.Path()
				if err != nil {
					return false, err
				}
				// If they are not the same, move to the operation
				if path1 != path2 {
					continue
				}
				val1, err := o.ValueInterface()
				err1 := errors.Cause(err)
				if err != nil && err1 != jsonpatch.ErrMissing {
					return false, err
				}
				val2, err := n.ValueInterface()
				err2 := errors.Cause(err)
				if err != nil && err2 != jsonpatch.ErrMissing {
					return false, err
				}
				if val1 != val2 && !(err2 == jsonpatch.ErrMissing && err1 == jsonpatch.ErrMissing) {
					continue
				}
				found = append(found, true)
			}
		}
	}

	if len(found) == len(patch1) && len(found) == len(patch2) {
		return true, nil
	}
	return false, nil
}

//TODO: diff function that makes it easy to determine the diffs between two things
