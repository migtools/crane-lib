package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/konveyor/crane-lib/transform"
	"github.com/konveyor/crane-lib/transform/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

type CustomPlugin struct {
	// TODO: figure out a way to include the name of the plugin in the error messages.
	name    string
	runFunc func(*unstructured.Unstructured) (transform.PluginResponse, error)
}

func (c *CustomPlugin) Run(u *unstructured.Unstructured) (transform.PluginResponse, error) {
	if c.runFunc == nil {
		return transform.PluginResponse{}, nil
	}
	return c.runFunc(u)
}

func NewCustomPlugin(name string, runFunc func(*unstructured.Unstructured) (transform.PluginResponse, error)) transform.Plugin {
	return &CustomPlugin{
		name:    name,
		runFunc: runFunc,
	}
}

func Unstructured(reader io.Reader) (*unstructured.Unstructured, error) {
	decoder := json.NewDecoder(reader)
	u := &unstructured.Unstructured{}
	err := decoder.Decode(u)
	if err != nil {
		return nil, &errors.PluginError{
			Type:         errors.PluginInvalidIOError,
			Message:      "unable to decode valid json from the reader",
			ErrorMessage: err.Error(),
		}
	}
	return u, nil
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

func WriterErrorAndExit(err error) {
	fmt.Fprintf(stdOut(), err.Error())
	// TODO: provide different exit codes using the Is* methods on the errors
	os.Exit(1)
}

func RunAndExit(plugin transform.Plugin, u *unstructured.Unstructured) {
	resp, err := plugin.Run(u)
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
