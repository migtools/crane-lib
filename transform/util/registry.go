package util

import (
	"fmt"
	"strings"

	jsonpatch "github.com/evanphx/json-patch"
)

const (
	updateImageString = `[
{"op": "replace", "path": "%v", "value": "%v"}
]`

)
func UpdateImageRegistry(registryReplacements map[string]string, oldImageName string) (string, bool) {
	// Break up oldImage to get the registry URL. Assume all manifests are using fully qualified image paths, if not ignore.
	imageParts := strings.Split(oldImageName, "/")
	for i := len(imageParts); i > 0; i-- {
		if replacedImageParts, ok := registryReplacements[strings.Join(imageParts[:i], "/")]; ok {
			if i == len(imageParts) {
				return replacedImageParts, true
			}
			return fmt.Sprintf("%s/%s", replacedImageParts, strings.Join(imageParts[i:], "/")), true
		}
	}
	return "", false
}

func UpdateImage(containerImagePath, updatedImagePath string) (jsonpatch.Patch, error) {
	patchJSON := fmt.Sprintf(updateImageString, containerImagePath, updatedImagePath)

	patch, err := jsonpatch.DecodePatch([]byte(patchJSON))
	if err != nil {
		return nil, err
	}
	return patch, nil
}

