package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/sirupsen/logrus"

	"github.com/konveyor/crane-lib/transform"
	"github.com/konveyor/crane-lib/transform/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

type CustomPlugin struct {
	// TODO: figure out a way to include the name of the plugin in the error messages.
	metadata transform.PluginMetadata
	runFunc  func(*unstructured.Unstructured) (transform.PluginResponse, error)
}

func (c *CustomPlugin) Run(u *unstructured.Unstructured, extras map[string]string) (transform.PluginResponse, error) {
	if c.runFunc == nil {
		return transform.PluginResponse{}, nil
	}
	return c.runFunc(u)
}

func (c *CustomPlugin) Metadata() transform.PluginMetadata {
	return c.metadata
}

func NewCustomPlugin(name, version string, optionalFields []string, runFunc func(*unstructured.Unstructured) (transform.PluginResponse, error)) transform.Plugin {
	return &CustomPlugin{
		metadata: transform.PluginMetadata{
			Name:            name,
			Version:         version,
			RequestVersion:  []transform.Version{transform.V1},
			ResponseVersion: []transform.Version{transform.V1},
			OptionalFields:  optionalFields,
		},
		runFunc: runFunc,
	}
}

func ObjectReaderOrDie() io.Reader {
	return os.Stdin
}

func stdOut() io.Writer {
	return os.Stdout
}

func stdErr() io.Writer {
	return os.Stderr
}

// Will write the error the standard error and will exit with 1
func WriterErrorAndExit(err error) {
	fmt.Fprint(stdErr(), err.Error())
	os.Exit(1)
}

func Logger() logrus.FieldLogger {
	return &logrus.Logger{}
}

func RunAndExit(plugin transform.Plugin) {
	// Get the reader from Standard In.

	var s string
	_, err := fmt.Scanln(&s)
	if err != nil {
		WriterErrorAndExit(fmt.Errorf("error getting unstructured object: %#v", err))
	}
	// Determine if Metadata Call
	if s == transform.MetadataString {
		err = json.NewEncoder(stdOut()).Encode(plugin.Metadata())
		if err != nil {
			WriterErrorAndExit(fmt.Errorf("error writing plugin response to stdOut: %#v", err))
		}
		return
	}

	// Get unstructured
	u := unstructured.Unstructured{}
	err = u.UnmarshalJSON([]byte(s))
	if err != nil {
		WriterErrorAndExit(fmt.Errorf("error getting unstructured object: %#v", err))
	}

	resp, err := plugin.Run(&u, nil)
	if err != nil {
		WriterErrorAndExit(&errors.PluginError{
			Type:         errors.PluginRunError,
			Message:      "error when running plugin",
			ErrorMessage: err.Error(),
		})
	}

	respBytes, err := json.Marshal(&resp)
	if err != nil {
		WriterErrorAndExit(&errors.PluginError{
			Type:         errors.PluginRunError,
			Message:      "invalid json plugin output, unable to marshal in",
			ErrorMessage: err.Error(),
		})
	}

	_, err = io.Copy(stdOut(), bytes.NewReader(respBytes))
	if err != nil {
		WriterErrorAndExit(&errors.PluginError{
			Type:         errors.PluginInvalidIOError,
			Message:      "error writing plugin response to stdOut",
			ErrorMessage: err.Error(),
		})
	}
}
