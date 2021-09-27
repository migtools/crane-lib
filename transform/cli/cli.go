package cli

import (
	"bytes"
	"encoding/json"
	goerrors "errors"
	"fmt"
	"io"
	"os"

	"github.com/sirupsen/logrus"

	"github.com/konveyor/crane-lib/transform"
	"github.com/konveyor/crane-lib/transform/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

var (
	stdErr io.Writer
	stdOut io.Writer
	reader io.Reader
	exiter func(int)
	logger *logrus.Logger
)

func init() {
	stdErr = os.Stderr
	stdOut = os.Stdout
	reader = os.Stdin

	exiter = os.Exit
	logger = logrus.New()
	logger.SetOutput(stdErr)
}

type customPlugin struct {
	// TODO: figure out a way to include the name of the plugin in the error messages.
	metadata transform.PluginMetadata
	runFunc  func(*unstructured.Unstructured, map[string]string) (transform.PluginResponse, error)
}

func (c *customPlugin) Run(u *unstructured.Unstructured, extras map[string]string) (transform.PluginResponse, error) {
	if c.runFunc == nil {
		return transform.PluginResponse{}, nil
	}
	return c.runFunc(u, extras)
}

func (c *customPlugin) Metadata() transform.PluginMetadata {
	return c.metadata
}

func NewCustomPlugin(name, version string, optionalFields []transform.OptionalFields, runFunc func(*unstructured.Unstructured, map[string]string) (transform.PluginResponse, error)) transform.Plugin {
	return &customPlugin{
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

// Will write the error the standard error and will exit with 1
func WriterErrorAndExit(err error) {
	fmt.Fprint(stdErr, err.Error())
	exiter(1)
}

func Logger() *logrus.Logger {
	return logger
}

func RunAndExit(plugin transform.Plugin) {
	// Get the reader from Standard In.
	decoder := json.NewDecoder(reader)
	m := map[string]interface{}{}

	err := decoder.Decode(&m)
	if err != nil {
		WriterErrorAndExit(&errors.PluginError{
			Type:         errors.PluginInvalidIOError,
			Message:      "error reading plugin input from input",
			ErrorMessage: err.Error(),
		})
	}

	// Determine if Metadata Call
	if len(m) == 0 {
		err := json.NewEncoder(stdOut).Encode(plugin.Metadata())
		if err != nil {
			WriterErrorAndExit(&errors.PluginError{
				Type:         errors.PluginInvalidIOError,
				Message:      "error writing plugin response to stdOut",
				ErrorMessage: err.Error(),
			})
		}
		return
	}

	// Ignoring this error as anthing wrong here will be caught in the unmarshalJSON below
	b, _ := json.Marshal(m)
	u := unstructured.Unstructured{}
	err = u.UnmarshalJSON(b)
	if err != nil {
		WriterErrorAndExit(&errors.PluginError{
			Type:         errors.PluginInvalidInputError,
			Message:      "error writing plugin response to stdOut",
			ErrorMessage: err.Error(),
		})
	}

	extrasIn := map[string]interface{}{}

	err = decoder.Decode(&extrasIn)
	if err != nil && !goerrors.Is(err, io.EOF) {
		WriterErrorAndExit(&errors.PluginError{
			Type:         errors.PluginInvalidIOError,
			Message:      "error reading extras input",
			ErrorMessage: err.Error(),
		})
	}
	extras := map[string]string{}
	for key, value := range extrasIn {
		switch value.(type) {
		case string:
			extras[key] = value.(string)
		default:
			WriterErrorAndExit(&errors.PluginError{
				Type:         errors.PluginInvalidIOError,
				Message:      "error getting extras value string",
				ErrorMessage: fmt.Sprintf("value %v for param %v is not a string", value, key),
			})
		}
	}

	resp, err := plugin.Run(&u, extras)
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

	_, err = io.Copy(stdOut, bytes.NewReader(respBytes))
	if err != nil {
		WriterErrorAndExit(&errors.PluginError{
			Type:         errors.PluginInvalidIOError,
			Message:      "error writing plugin response to stdOut",
			ErrorMessage: err.Error(),
		})
	}
}
