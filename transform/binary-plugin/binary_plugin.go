package binary_plugin

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"

	"github.com/konveyor/crane-lib/transform"
	"github.com/sirupsen/logrus"
)

const (
	MetadataRequest string = `{}`
)

type BinaryPlugin struct {
	commandRunner
	pluginMetadata transform.PluginMetadata
	log            logrus.FieldLogger
}

// NewBinaryPlugin -
func NewBinaryPlugin(path string, logger *logrus.Logger) (transform.Plugin, error) {

	commandRunner := &binaryRunner{pluginPath: path}
	log := logger.WithField("pluginPath", path)

	out, errBytes, err := commandRunner.Metadata(log)
	// TODO: Create specific error for command not being run.
	if err != nil {
		log.Errorf("error running the plugin metadata command")
		return nil, fmt.Errorf("error running the plugin metadata command: %v", err)
	}

	if len(errBytes) != 0 {
		log.Errorf("error from plugin binary")
		return nil, fmt.Errorf("error from plugin binary: %s", string(errBytes))
	}

	metadata := transform.PluginMetadata{}
	err = json.Unmarshal(out, &metadata)
	if err != nil {
		log.Errorf("unable to decode json sent by the plugin")
		return nil, fmt.Errorf("unable to decode metadata sent by the plugin: %s, err: %v", string(out), err)
	}

	// Validate version return error
	if !contain(transform.RequestVersion, metadata.RequestVersion) || !contain(transform.ResponseVersion, metadata.ResponseVersion) {
		return nil, fmt.Errorf("invalid versions supported by plugin defined by caller responseVersions: %v, requestVersions: %v", metadata.ResponseVersion, metadata.RequestVersion)
	}

	// TODO: Validate Versions contain the versions that this wrapper can use.
	return &BinaryPlugin{commandRunner: commandRunner, pluginMetadata: metadata, log: log}, nil
}

func contain(validVersion transform.Version, versions []transform.Version) bool {
	for _, v := range versions {
		if validVersion == v {
			return true
		}
	}
	return false
}

func (b *BinaryPlugin) Run(request transform.PluginRequest) (transform.PluginResponse, error) {
	p := transform.PluginResponse{}

	out, errBytes, err := b.commandRunner.Run(request, b.log)
	if len(errBytes) != 0 {
		logs := strings.Split(string(errBytes), "\n")
		for _, line := range logs {
			b.log.Debug("Plugin Log line: ", line)
		}
	}
	if err != nil {
		b.log.Errorf("error running the plugin command")
		return p, fmt.Errorf("error running the plugin command: %v", err)
	}


	err = json.Unmarshal(out, &p)
	if err != nil {
		b.log.Errorf("unable to decode json sent by the plugin")
		return p, fmt.Errorf("unable to decode object sent by the plugin: %s, err: %v", string(out), err)
	}

	return p, nil
}

func (b *BinaryPlugin) Metadata() transform.PluginMetadata {
	return b.pluginMetadata
}

type commandRunner interface {
	Run(request transform.PluginRequest, log logrus.FieldLogger) ([]byte, []byte, error)
	Metadata(log logrus.FieldLogger) ([]byte, []byte, error)
}

type binaryRunner struct {
	pluginPath string
}

// Type to use for
type execContext func(name string, arg ...string) *exec.Cmd

func (e execContext) getCommand(name string, arg ...string) *exec.Cmd {
	if e != nil {
		return e(name, arg...)
	}
	return exec.Command(name, arg...)
}

var cliContext execContext

func (b *binaryRunner) Metadata(log logrus.FieldLogger) ([]byte, []byte, error) {
	command := cliContext.getCommand(b.pluginPath)

	// set var to get the output
	var out bytes.Buffer
	var errorBytes bytes.Buffer

	// set the output to our variable
	command.Stdout = &out
	command.Stdin = bytes.NewBufferString(MetadataRequest)
	command.Stderr = &errorBytes
	err := command.Run()
	if err != nil {
		log.Errorf("unable to run the plugin binary")
		return nil, nil, fmt.Errorf("unable to run the plugin binary, err: %v", err)
	}

	return out.Bytes(), errorBytes.Bytes(), nil

}

func (b *binaryRunner) Run(request transform.PluginRequest, log logrus.FieldLogger) ([]byte, []byte, error) {
	unstructuredJson, err := request.MarshalJSON()
	objMap := map[string]interface{}{}
	json.Unmarshal(unstructuredJson, &objMap)
	objMap["extras"] = request.Extras
	objJson, err := json.Marshal(objMap)
	if err != nil {
		log.Errorf("unable to marshal unstructured Object")
		return nil, nil, fmt.Errorf("unable to marshal unstructured Object: %s, err: %v", request, err)
	}
	command := cliContext.getCommand(b.pluginPath)

	// set var to get the output
	var out bytes.Buffer
	var errorBytes bytes.Buffer

	// set the output to our variable
	command.Stdout = &out
	command.Stdin = bytes.NewBuffer(objJson)
	command.Stderr = &errorBytes
	err = command.Run()
	if err != nil {
		log.Errorf("unable to run the plugin binary")
		return nil, errorBytes.Bytes(), fmt.Errorf("unable to run the plugin binary, err: %v", err)
	}
	return out.Bytes(), errorBytes.Bytes(), nil
}
