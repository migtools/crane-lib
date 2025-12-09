package convert

import (
	"log"

	"github.com/sirupsen/logrus"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type ConvertOptions struct {
	configFlags *genericclioptions.ConfigFlags
	genericclioptions.IOStreams

	Client             client.Client
	Namespace          string
	ResourceType       string
	SearchRegistries   []string
	InsecureRegistries []string
	BlockRegistries    []string
	ExportDir          string
	Logger             logrus.FieldLogger
}

func (t *ConvertOptions) Convert() error {
	switch convertType := t.ResourceType; convertType {
	case "BuildConfigs":
		err := t.convertBuildConfigs()
		if err != nil {
			return err
		}
	default:
		log.Fatal("crane cannot convert resource type " + t.ResourceType)
	}

	return nil
}
