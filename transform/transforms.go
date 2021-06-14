package transform

import (
	"fmt"
	"path/filepath"
	"strings"
)

type TransformOpts struct {
	TransformDir string
	ResourceDir  string
}

func (opts *TransformOpts) GetWhiteOutFilePath(filePath string) string {
	return opts.updatePath(".wh.", filePath)
}

func (opts *TransformOpts) GetTransformPath(filePath string) string {
	return opts.updatePath("transform-", filePath)

}

func (opts *TransformOpts) updatePath(prefix, filePath string) string {
	dir, fname := filepath.Split(filePath)
	dir = strings.Replace(dir, opts.ResourceDir, opts.TransformDir, 1)
	fname = fmt.Sprintf("%v%v", prefix, fname)
	return filepath.Join(dir, fname)
}
