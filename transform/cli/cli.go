package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/konveyor/crane-lib/transform"

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
	return u, err
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
	fmt.Fprintf(stdErr(), err.Error())
	os.Exit(1)
}

func RunAndExit(plugin transform.Plugin, u *unstructured.Unstructured) {
	resp, err := plugin.Run(u)
	if err != nil {
		fmt.Fprintf(stdErr(), fmt.Errorf("error when running plugin: %#v", err).Error())
		os.Exit(1)
	}

	err = json.NewEncoder(stdOut()).Encode(resp)
	if err != nil {
		fmt.Fprintf(stdErr(), fmt.Errorf("error writing plugin response to stdOut: %#v", err).Error())
		os.Exit(1)
	}
}
