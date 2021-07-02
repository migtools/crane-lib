package binary_plugin

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"

	"github.com/konveyor/crane-lib/transform"
	"github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

type BinaryPlugin struct {
	commandRunner
	pluginMetadata transform.PluginMetadata
	log            logrus.FieldLogger
}

// NewBinaryPlugin -
func NewBinaryPlugin(path string) (transform.Plugin, error) {

	commandRunner := &binaryRunner{pluginPath: path}
	log := logrus.New().WithField("pluginPath", path)

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

	// TODO: Validate Versions contain the versions that this wrapper can use.
	return &BinaryPlugin{commandRunner: commandRunner, pluginMetadata: metadata, log: log}, nil
}

func (b *BinaryPlugin) Run(u *unstructured.Unstructured, extras map[string]string) (transform.PluginResponse, error) {
	p := transform.PluginResponse{}
	logs := []string{}

	out, errBytes, err := b.commandRunner.Run(u, b.log)
	if err != nil {
		b.log.Errorf("error running the plugin command")
		return p, fmt.Errorf("error running the plugin command: %v", err)
	}

	if len(errBytes) != 0 {
		logs = strings.Split(string(errBytes), "\n")
		for _, line := range logs {
			b.log.Debug("Plugin Log line: ", line)
		}
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
	Run(u *unstructured.Unstructured, log logrus.FieldLogger) ([]byte, []byte, error)
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
	command.Stdin = bytes.NewBufferString(transform.MetadataString)
	command.Stderr = &errorBytes
	err := command.Run()
	if err != nil {
		log.Errorf("unable to run the plugin binary")
		return nil, nil, fmt.Errorf("unable to run the plugin binary, err: %v", err)
	}

	return out.Bytes(), errorBytes.Bytes(), nil

}

func (b *binaryRunner) Run(u *unstructured.Unstructured, log logrus.FieldLogger) ([]byte, []byte, error) {
	objJson, err := u.MarshalJSON()
	if err != nil {
		log.Errorf("unable to marshal unstructured Object")
		return nil, nil, fmt.Errorf("unable to marshal unstructured Object: %s, err: %v", u, err)
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
		return nil, nil, fmt.Errorf("unable to run the plugin binary, err: %v", err)
	}

	return out.Bytes(), errorBytes.Bytes(), nil
}
