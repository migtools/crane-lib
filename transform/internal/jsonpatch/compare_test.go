package jsonpatch_test

import (
	"testing"

	jpatch "github.com/evanphx/json-patch"
	internaljsonpatch "github.com/konveyor/crane-lib/transform/internal/jsonpatch"
)

func TestCompare(t *testing.T) {
	cases := []struct {
		Name        string
		Patch1      string
		Patch2      string
		ShouldError bool
		IsEqual     bool
	}{
		{
			Name:        "EqualPatches",
			Patch1:      `[{"op": "add", "path": "/spec/testing", "value": "value1"}]`,
			Patch2:      `[{"op": "add", "path": "/spec/testing", "value": "value1"}]`,
			ShouldError: false,
			IsEqual:     true,
		},
		{
			Name:        "EqualPatchesArrayValue",
			Patch1:      `[{"op": "add", "path": "/spec/testing", "value": ["value1", "value2"]}]`,
			Patch2:      `[{"op": "add", "path": "/spec/testing", "value": ["value1", "value2"]}]`,
			ShouldError: false,
			IsEqual:     true,
		},
		{
			Name:        "DifferentKinds",
			Patch1:      `[{"op": "add", "path": "/spec/testing", "value": "value1"}]`,
			Patch2:      `[{"op": "remove", "path": "/spec/testing"}]`,
			ShouldError: false,
			IsEqual:     false,
		},
		{
			Name:        "DifferentPaths",
			Patch1:      `[{"op": "add", "path": "/spec/testing", "value": "value1"}]`,
			Patch2:      `[{"op": "add", "path": "/spec/testingNotSame", "value": "value1"}]`,
			ShouldError: false,
			IsEqual:     false,
		},
		{
			Name:        "DifferentValues",
			Patch1:      `[{"op": "add", "path": "/spec/testing", "value": "value1"}]`,
			Patch2:      `[{"op": "add", "path": "/spec/testing", "value": "valueNotSame"}]`,
			ShouldError: false,
			IsEqual:     false,
		},
		{
			Name:        "DifferentArrayValues",
			Patch1:      `[{"op": "add", "path": "/spec/testing", "value": ["value1", "value2"]}]`,
			Patch2:      `[{"op": "add", "path": "/spec/testing", "value": ["value1", "value3"]}]`,
			ShouldError: false,
			IsEqual:     false,
		},
		{
			Name:        "EqualPatchesDifferentOrders",
			Patch1:      `[{"op": "add", "path": "/spec/testingDiff", "value": "valueDiff"},{"op": "add", "path": "/spec/testing", "value": "value1"}]`,
			Patch2:      `[{"op": "add", "path": "/spec/testing", "value": "value1"},{"op": "add", "path": "/spec/testingDiff", "value": "valueDiff"}]`,
			ShouldError: false,
			IsEqual:     true,
		},
		{
			Name:        "SameMoveFrom",
			Patch1:      `[{"op": "move", "from": "/spec/from1", "path": "/spec/testing"}]`,
			Patch2:      `[{"op": "move", "from": "/spec/from1", "path": "/spec/testing"}]`,
			ShouldError: false,
			IsEqual:     true,
		},
		{
			Name:        "DifferentMoveFrom",
			Patch1:      `[{"op": "move", "from": "/spec/from1", "path": "/spec/testing"}]`,
			Patch2:      `[{"op": "move", "from": "/spec/from2", "path": "/spec/testing"}]`,
			ShouldError: false,
			IsEqual:     false,
		},
	}

	for _, c := range cases {
		t.Run(c.Name, func(t *testing.T) {
			patch1, err := jpatch.DecodePatch([]byte(c.Patch1))
			if err != nil {
				t.Error(err)
			}
			patch2, err := jpatch.DecodePatch([]byte(c.Patch2))
			if err != nil {
				t.Error(err)
			}
			actualValue, err := internaljsonpatch.Equal(patch1, patch2)
			if err != nil && !c.ShouldError {
				t.Error(err)
			}

			if actualValue != c.IsEqual {
				t.Errorf("Comparison was not successful, acutalValue: %v, expectedValue: %v", actualValue, c.IsEqual)
			}
		})
	}

}
