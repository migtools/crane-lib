package util

import (
	"errors"
	"fmt"
	"strings"

	jsonpatch "github.com/evanphx/json-patch"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/validation"
)

const (
	opReplace = `[
{"op": "replace", "path": "%v", "value": "%v"}
]`
	PVCPathCronJobString  = "/spec/jobTemplate/spec/template/spec/volumes/%d/persistentVolumeClaim/claimName"
	PVCPathPodString      = "/spec/volumes/%d/persistentVolumeClaim/claimName"
	PVCPathGenericString  = "/spec/template/spec/volumes/%d/persistentVolumeClaim/claimName"
	PVCPathTemplateString = "/spec/volumeClaimTemplates/%d/metadata/name"
)

func ProcessPVCMap(pvcString string) (map[string]string, error) {
	pvcRenameList := strings.Split(pvcString, ",")
	pvcMap := map[string]string{}
	for _, pair := range pvcRenameList {
		split := strings.Split(pair, ":")
		if errs := validation.IsDNS1123Subdomain(split[0]); len(errs) != 0 {
			return map[string]string{}, errors.New("Invalid PVC remap: " + pair + ", " + strings.Join(errs[:], ","))
		} else if errs := validation.IsDNS1123Subdomain(split[1]); len(errs) != 0 {
			return map[string]string{}, errors.New("Invalid PVC remap: " + pair + ", " + strings.Join(errs[:], ","))
		}
		pvcMap[split[0]] = split[1]
	}
	return pvcMap, nil
}

func RenamePVCs(volumes []v1.Volume, PVCRenameMap map[string]string, path string) (jsonpatch.Patch, error) {
	var patches jsonpatch.Patch
	if len(PVCRenameMap) > 0 && len(volumes) > 0 {
		for i, volume := range volumes {
			if volume.PersistentVolumeClaim != nil {
				if pvcName, ok := PVCRenameMap[volume.PersistentVolumeClaim.ClaimName]; ok {
					pvcPath := fmt.Sprintf(path, i)
					patch, err := jsonpatch.DecodePatch([]byte(fmt.Sprintf(opReplace, pvcPath, pvcName)))
					if err != nil {
						return nil, err
					}
					patches = append(patches, patch...)
				}
			}
		}
	}
	return patches, nil
}
