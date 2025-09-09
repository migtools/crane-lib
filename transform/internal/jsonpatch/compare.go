package jsonpatch

import (
	"reflect"
	"strings"

	jsonpatch "github.com/evanphx/json-patch"
	"github.com/pkg/errors"
)

func Equal(patch1, patch2 jsonpatch.Patch) (bool, error) {
	if len(patch1) != len(patch2) {
		return false, nil
	}

	used := make([]bool, len(patch2))
	for _, o1 := range patch1 {
		found := false
		for i, o2 := range patch2 {
			if !used[i] && EqualOperation(o1, o2) {
				used[i] = true
				found = true
				break
			}
		}
		if !found {
			return false, nil
		}
	}
	return true, nil
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
		if operation1.Kind() == "move" || operation1.Kind() == "copy" {
			from1, err := operation1.From()
			if err != nil {
				return false
			}
			from2, err := operation2.From()
			if err != nil {
				return false
			}
			if from1 != from2 {
				return false
			}
		}
		val1, err1 := operation1.ValueInterface()
		val2, err2 := operation2.ValueInterface()
		
		// Check if both operations have missing values (like remove operations)
		isMissing1 := err1 != nil && (errors.Cause(err1) == jsonpatch.ErrMissing || strings.Contains(err1.Error(), "missing value"))
		isMissing2 := err2 != nil && (errors.Cause(err2) == jsonpatch.ErrMissing || strings.Contains(err2.Error(), "missing value"))
		
		// If one has missing value and the other doesn't, they're not equal
		if isMissing1 != isMissing2 {
			return false
		}
		
		// If both have missing values, they're equal (for operations like remove)
		if isMissing1 && isMissing2 {
			return true
		}
		
		// If neither has missing values, compare the actual values
		if !isMissing1 && !isMissing2 {
			return reflect.DeepEqual(val1, val2)
		}
		
		// One has an error but it's not a missing value error
		return false
	}
	return false
}

//TODO: diff function that makes it easy to determine the diffs between two things
