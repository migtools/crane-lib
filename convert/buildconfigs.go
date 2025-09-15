package convert

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	buildv1 "github.com/openshift/api/build/v1"
	imagev1 "github.com/openshift/api/image/v1"
	shipwrightv1beta1 "github.com/shipwright-io/build/pkg/apis/build/v1beta1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"
)

const (
	// Type of "From" image for Docker Strategy
	ImageStreamTag   = "ImageStreamTag"
	ImageStreamImage = "ImageStreamImage"
	DockerImage      = "DockerImage"
)

func (t *ConvertOptions) convertBuildConfigs() error {
	bcList := buildv1.BuildConfigList{}
	err := t.Client.List(context.TODO(), &bcList, client.InNamespace(t.Namespace))
	if err != nil {
		return err
	}

	err = t.writeBuildConfigs(bcList)
	if err != nil {
		return err
	}

	for _, bc := range bcList.Items {
		b := &shipwrightv1beta1.Build{}
		b.Name = bc.Name
		b.Kind = "Build"
		b.APIVersion = "shipwright.io/v1beta1"
		b.Spec.ParamValues = []shipwrightv1beta1.ParamValue{}

		switch strategyType := bc.Spec.Strategy.Type; strategyType {
		case "Docker":
			ClusterBuildStrategyKind := shipwrightv1beta1.ClusterBuildStrategyKind
			b.Spec.Strategy = shipwrightv1beta1.Strategy{
				Kind: &ClusterBuildStrategyKind,
				Name: "buildah",
			}

			// process from field
			if bc.Spec.Strategy.DockerStrategy.From != nil {
				err := t.processDockerStrategyFromField(&bc, b)
				if err != nil {
					return err
				}
			}

			// process pull secret
			pullSecret := bc.Spec.Strategy.DockerStrategy.PullSecret
			if pullSecret != nil {
				// Validate pull secret
				if err := t.validatePullSecret(&bc, pullSecret); err != nil {
					return err
				}

				// Generate ServiceAccount for pull secret
				if err := t.generateServiceAccountForPullSecret(&bc); err != nil {
					return err
				}
			}

			// process env fields
			if bc.Spec.Strategy.DockerStrategy.Env != nil {
				b.Spec.Env = append(b.Spec.Env, bc.Spec.Strategy.DockerStrategy.Env...)
			}

			if bc.Spec.Strategy.DockerStrategy.DockerfilePath != "" {
				dockerfile := shipwrightv1beta1.ParamValue{
					Name: "dockerfile",
					SingleValue: &shipwrightv1beta1.SingleValue{
						Value: &bc.Spec.Strategy.DockerStrategy.DockerfilePath,
					},
				}
				b.Spec.ParamValues = append(b.Spec.ParamValues, dockerfile)
			}

			if len(bc.Spec.Strategy.DockerStrategy.Volumes) > 0 {
				if err := t.processDockerStrategyVolumes(&bc, b); err != nil {
					return err
				}
			}

			t.processBuildArgs(bc, b)
		case "Source":
			ClusterBuildStrategyKind := shipwrightv1beta1.ClusterBuildStrategyKind
			b.Spec.Strategy = shipwrightv1beta1.Strategy{
				Kind: &ClusterBuildStrategyKind,
				Name: "source-to-image",
			}

			// process From field
			if bc.Spec.Strategy.SourceStrategy.From.Name != "" {
				err := t.processSourceStrategyFromField(&bc, b)
				if err != nil {
					return err
				}
			}

			// process pull secret
			pullSecret := bc.Spec.Strategy.SourceStrategy.PullSecret
			if pullSecret != nil {
				// Validate pull secret
				if err := t.validatePullSecret(&bc, pullSecret); err != nil {
					return err
				}

				// Generate ServiceAccount for pull secret
				if err := t.generateServiceAccountForPullSecret(&bc); err != nil {
					return err
				}
			}

			// process env fields
			if bc.Spec.Strategy.SourceStrategy.Env != nil {
				b.Spec.Env = append(b.Spec.Env, bc.Spec.Strategy.SourceStrategy.Env...)
			}

		// TODO: What do we do for custom?
		// TODO: Jenkins Pipeline?
		default:
			fmt.Println("Strategy type", bc.Spec.Strategy.Type, "is unknown for BuildConfig", bc.Name)
		}

		if bc.Spec.Output.PushSecret != nil && bc.Spec.Output.PushSecret.Name != "" {
			b.Spec.Output.PushSecret = &bc.Spec.Output.PushSecret.Name
		}

		t.processSource(bc, b)
		t.processOutput(bc, b)
		t.addRegistries(b)
		t.writeBuild(b)
	}

	return nil
}

// processSourceStrategyFromField processes From field for Source strategy
func (t *ConvertOptions) processSourceStrategyFromField(bc *buildv1.BuildConfig, b *shipwrightv1beta1.Build) error {
	if bc.Spec.Strategy.SourceStrategy.From.Name == "" {
		return nil
	}

	switch fromKind := bc.Spec.Strategy.SourceStrategy.From.Kind; fromKind {
	case ImageStreamTag:
		imageRef, err := t.resolveImageStreamRef(bc.Spec.Strategy.SourceStrategy.From.Name, bc.Spec.Strategy.SourceStrategy.From.Namespace)
		if err != nil {
			return err
		}
		builderImage := shipwrightv1beta1.ParamValue{
			Name: "builder-image",
			SingleValue: &shipwrightv1beta1.SingleValue{
				Value: &imageRef,
			},
		}
		b.Spec.ParamValues = append(b.Spec.ParamValues, builderImage)
	case ImageStreamImage:
		imageRef, err := t.resolveImageStreamRef(bc.Spec.Strategy.SourceStrategy.From.Name, bc.Spec.Strategy.SourceStrategy.From.Namespace)
		if err != nil {
			return err
		}
		builderImage := shipwrightv1beta1.ParamValue{
			Name: "builder-image",
			SingleValue: &shipwrightv1beta1.SingleValue{
				Value: &imageRef,
			},
		}
		b.Spec.ParamValues = append(b.Spec.ParamValues, builderImage)
	case DockerImage:
		// we can use the name directly
		builderImage := shipwrightv1beta1.ParamValue{
			Name: "builder-image",
			SingleValue: &shipwrightv1beta1.SingleValue{
				Value: &bc.Spec.Strategy.SourceStrategy.From.Name,
			},
		}
		b.Spec.ParamValues = append(b.Spec.ParamValues, builderImage)
	default:
		return fmt.Errorf("source strategy From kind %s is unknown for BuildConfig %s", fromKind, bc.Name)
	}
	return nil
}

// processDockerStrategyFromField processes From field for Docker Strategy
func (t *ConvertOptions) processDockerStrategyFromField(bc *buildv1.BuildConfig, b *shipwrightv1beta1.Build) error {
	if bc.Spec.Strategy.DockerStrategy.From == nil {
		return nil
	}

	switch fromKind := bc.Spec.Strategy.DockerStrategy.From.Kind; fromKind {
	case ImageStreamTag:
		imageRef, err := t.resolveImageStreamRef(bc.Spec.Strategy.DockerStrategy.From.Name, bc.Spec.Strategy.DockerStrategy.From.Namespace)
		if err != nil {
			return err
		}
		fromImage := shipwrightv1beta1.ParamValue{
			Name: "from",
			SingleValue: &shipwrightv1beta1.SingleValue{
				Value: &imageRef,
			},
		}
		b.Spec.ParamValues = append(b.Spec.ParamValues, fromImage)
	case ImageStreamImage:
		imageRef, err := t.resolveImageStreamRef(bc.Spec.Strategy.DockerStrategy.From.Name, bc.Spec.Strategy.DockerStrategy.From.Namespace)
		if err != nil {
			return err
		}
		fromImage := shipwrightv1beta1.ParamValue{
			Name: "from",
			SingleValue: &shipwrightv1beta1.SingleValue{
				Value: &imageRef,
			},
		}
		b.Spec.ParamValues = append(b.Spec.ParamValues, fromImage)
	case DockerImage:
		// we can use the name directly
		fromImage := shipwrightv1beta1.ParamValue{
			Name: "from",
			SingleValue: &shipwrightv1beta1.SingleValue{
				Value: &bc.Spec.Strategy.DockerStrategy.From.Name,
			},
		}
		b.Spec.ParamValues = append(b.Spec.ParamValues, fromImage)
	default:
		return fmt.Errorf("docker strategy From kind %s is unknown for BuildConfig %s", fromKind, bc.Name)
	}
	return nil
}

// validatePullSecret validates a pull secret for any build strategy
func (t *ConvertOptions) validatePullSecret(bc *buildv1.BuildConfig, secretRef *corev1.LocalObjectReference) error {
	if secretRef == nil {
		return nil
	}

	secretName := secretRef.Name
	if secretName == "" {
		return fmt.Errorf("pullSecret name is empty for BuildConfig %s", bc.Name)
	}

	var secret corev1.Secret
	if err := t.Client.Get(context.Background(), client.ObjectKey{
		Namespace: bc.Namespace,
		Name:      secretName,
	}, &secret); err != nil {
		return fmt.Errorf("failed to get pull secret %q for BuildConfig %s: %w", secretName, bc.Name, err)
	}

	switch secret.Type {
	case corev1.SecretTypeDockerConfigJson:
		data, ok := secret.Data[corev1.DockerConfigJsonKey]
		if !ok || len(data) == 0 {
			return fmt.Errorf("secret %q must contain key %q for type %q",
				secretName, corev1.DockerConfigJsonKey, corev1.SecretTypeDockerConfigJson)
		}
	case corev1.SecretTypeDockercfg:
		data, ok := secret.Data[corev1.DockerConfigKey]
		if !ok || len(data) == 0 {
			return fmt.Errorf("secret %q must contain key %q for type %q",
				secretName, corev1.DockerConfigKey, corev1.SecretTypeDockercfg)
		}
	default:
		return fmt.Errorf("unsupported pull secret type %q for secret %q; supported types are %q and %q",
			string(secret.Type), secretName, corev1.SecretTypeDockerConfigJson, corev1.SecretTypeDockercfg)
	}

	return nil
}

func (t *ConvertOptions) generateServiceAccountForPullSecret(bc *buildv1.BuildConfig) error {
	// Determine ServiceAccount name
	saName := bc.Spec.ServiceAccount
	if saName == "" {
		saName = bc.Name + "-sa"
	}

	// Create ServiceAccount object
	serviceAccount := &corev1.ServiceAccount{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "ServiceAccount",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      saName,
			Namespace: bc.Namespace,
		},
		ImagePullSecrets: []corev1.LocalObjectReference{
			{
				Name: bc.Spec.Strategy.DockerStrategy.PullSecret.Name,
			},
		},
	}

	// Write ServiceAccount YAML
	return t.writeServiceAccount(serviceAccount)
}

func (t *ConvertOptions) writeServiceAccount(sa *corev1.ServiceAccount) error {
	targetDir := filepath.Join(t.ExportDir, "serviceaccounts", sa.Namespace)
	err := os.MkdirAll(targetDir, 0700)
	switch {
	case os.IsExist(err):
	case err != nil:
		t.logger.Errorf("error creating the serviceaccounts directory: %#v", err)
		return err
	}

	path := filepath.Join(targetDir, getServiceAccountFilePath(*sa))
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	objBytes, err := yaml.Marshal(sa)
	if err != nil {
		return err
	}

	_, err = f.Write(objBytes)
	return err
}

func getServiceAccountFilePath(sa corev1.ServiceAccount) string {
	return strings.Join([]string{sa.GetObjectKind().GroupVersionKind().GroupKind().Kind, sa.GetObjectKind().GroupVersionKind().GroupKind().Group, sa.GetObjectKind().GroupVersionKind().Version, sa.Namespace, sa.Name}, "_") + ".yaml"
}

func (t *ConvertOptions) processDockerStrategyVolumes(bc *buildv1.BuildConfig, b *shipwrightv1beta1.Build) error {
	if len(bc.Spec.Strategy.DockerStrategy.Volumes) == 0 {
		return nil
	}

	// Convert BuildConfig volumes to Shipwright volumes
	for _, bcVolume := range bc.Spec.Strategy.DockerStrategy.Volumes {
		// Convert OpenShift BuildVolumeSource to Kubernetes VolumeSource, which is used by Shipwright
		volumeSource, err := t.convertBuildVolumeSource(bcVolume.Source)
		if err != nil {
			return fmt.Errorf("failed to convert volume %q for BuildConfig %s: %w", bcVolume.Name, bc.Name, err)
		}

		shpVolume := shipwrightv1beta1.BuildVolume{
			Name:         bcVolume.Name,
			VolumeSource: volumeSource,
		}
		b.Spec.Volumes = append(b.Spec.Volumes, shpVolume)

		// Note: BuildConfig volume mount paths are not migrated to Shipwright Build
		// Mount paths are defined in the BuildStrategy, not in the Build resource
		if len(bcVolume.Mounts) > 0 {
			t.logger.Warnf("BuildConfig %s volume %q has mount paths that cannot be migrated to Shipwright Build. Mount paths are defined in the BuildStrategy. Original mounts: %v",
				bc.Name, bcVolume.Name, bcVolume.Mounts)
		}
	}
	return nil
}

func (t *ConvertOptions) convertBuildVolumeSource(bcSource buildv1.BuildVolumeSource) (corev1.VolumeSource, error) {
	volumeSource := corev1.VolumeSource{}

	switch bcSource.Type {
	case "Secret":
		if bcSource.Secret == nil {
			return volumeSource, fmt.Errorf("secret volume source is nil")
		}
		volumeSource.Secret = bcSource.Secret
	case "ConfigMap":
		if bcSource.ConfigMap == nil {
			return volumeSource, fmt.Errorf("configMap volume source is nil")
		}
		volumeSource.ConfigMap = bcSource.ConfigMap
	default:
		return volumeSource, fmt.Errorf("unsupported volume source type %q; supported types are Secret and ConfigMap", bcSource.Type)
	}

	return volumeSource, nil
}

func (t *ConvertOptions) processSource(bc buildv1.BuildConfig, b *shipwrightv1beta1.Build) {
	switch stype := bc.Spec.Source.Type; stype {
	case "Git":
		git := &shipwrightv1beta1.Git{
			Revision: &bc.Spec.Source.Git.Ref,
			URL:      bc.Spec.Source.Git.URI,
		}
		source := &shipwrightv1beta1.Source{
			Git:        git,
			Type:       "Git",
			ContextDir: &bc.Spec.Source.ContextDir,
		}

		b.Spec.Source = source
	// TODO: Dockerfile
	// TODO: Binary
	// TODO: Image
	// TODO: None
	default:
		fmt.Println("Source type", bc.Spec.Source.Type, "is unknown for BuildConfig", bc.Name)
	}
}

func (t *ConvertOptions) processOutput(bc buildv1.BuildConfig, b *shipwrightv1beta1.Build) {
	if bc.Spec.Output.To.Kind == "ImageStreamTag" {
		var namespace string
		if bc.Spec.Output.To.Namespace != "" {
			namespace = bc.Spec.Output.To.Namespace
		} else {
			namespace = bc.Namespace
		}
		b.Spec.Output.Image = "image-registry.openshift-image-registry.svc:5000/" + namespace + "/" + bc.Spec.Output.To.Name
	} else {
		b.Spec.Output.Image = bc.Spec.Output.To.Name
	}
}

func (t *ConvertOptions) addRegistries(b *shipwrightv1beta1.Build) {
	if len(t.SearchRegistries) != 0 {
		values := parseRegistries(t.SearchRegistries)

		registryParam := shipwrightv1beta1.ParamValue{
			Name:   "registries-search",
			Values: values,
		}

		b.Spec.ParamValues = append(b.Spec.ParamValues, registryParam)
	}

	if len(t.InsecureRegistries) != 0 {
		values := parseRegistries(t.BlockRegistries)

		insecureRegistryParam := shipwrightv1beta1.ParamValue{
			Name:   "registries-insecure",
			Values: values,
		}

		b.Spec.ParamValues = append(b.Spec.ParamValues, insecureRegistryParam)
	}

	if len(t.BlockRegistries) != 0 {
		values := parseRegistries(t.BlockRegistries)

		insecureRegistryParam := shipwrightv1beta1.ParamValue{
			Name:   "registries-block",
			Values: values,
		}

		b.Spec.ParamValues = append(b.Spec.ParamValues, insecureRegistryParam)
	}
}

func parseRegistries(registries []string) []shipwrightv1beta1.SingleValue {
	values := []shipwrightv1beta1.SingleValue{}
	for _, r := range registries {
		singleValue := shipwrightv1beta1.SingleValue{
			Value: &r,
		}
		values = append(values, singleValue)
	}

	return values
}

func (t *ConvertOptions) resolveImageStreamRef(name string, namespace string) (string, error) {
	imageStreamTag := imagev1.ImageStreamTag{}

	err := t.Client.Get(context.Background(), client.ObjectKey{
		Namespace: namespace,
		Name:      name,
	}, &imageStreamTag)
	if err != nil {
		return "", err
	}
	imageRef := imageStreamTag.Tag.From.Name

	return imageRef, nil
}

func (t *ConvertOptions) writeBuildConfigs(bcList buildv1.BuildConfigList) error {
	errs := []error{}
	targetDir := filepath.Join(t.ExportDir, "buildconfigs", t.Namespace)
	err := os.MkdirAll(targetDir, 0700)
	switch {
	case os.IsExist(err):
	case err != nil:
		t.logger.Errorf("error creating the resources directory: %#v", err)
		return err
	}

	for _, bc := range bcList.Items {
		bc.Kind = "BuildConfig"
		bc.APIVersion = "build.openshift.io/v1"
		path := filepath.Join(targetDir, getBuildConfigFilePath(bc))
		f, err := os.Create(path)
		if err != nil {
			errs = append(errs, err)
			continue
		}

		objBytes, err := yaml.Marshal(bc)
		if err != nil {
			errs = append(errs, err)
			continue
		}

		_, err = f.Write(objBytes)
		if err != nil {
			errs = append(errs, err)
			continue
		}

		err = f.Close()
		if err != nil {
			errs = append(errs, err)
			continue
		}
	}

	return nil
}

func getBuildConfigFilePath(bc buildv1.BuildConfig) string {
	return strings.Join([]string{bc.GetObjectKind().GroupVersionKind().GroupKind().Kind, bc.GetObjectKind().GroupVersionKind().GroupKind().Group, bc.GetObjectKind().GroupVersionKind().Version, bc.Namespace, bc.Name}, "_") + ".yaml"
}

func (t *ConvertOptions) writeBuild(b *shipwrightv1beta1.Build) error {
	targetDir := filepath.Join(t.ExportDir, "builds", t.Namespace)
	err := os.MkdirAll(targetDir, 0700)
	switch {
	case os.IsExist(err):
	case err != nil:
		t.logger.Errorf("error creating the resources directory: %#v", err)
		return err
	}

	path := filepath.Join(targetDir, getBuildFilePath(*b))
	f, err := os.Create(path)
	if err != nil {
		return err
	}

	objBytes, err := yaml.Marshal(b)
	if err != nil {
		return err
	}

	_, err = f.Write(objBytes)
	if err != nil {
		return err
	}

	err = f.Close()
	if err != nil {
		return err
	}

	return nil
}

func (t *ConvertOptions) processBuildArgs(bc buildv1.BuildConfig, b *shipwrightv1beta1.Build) {
	if len(bc.Spec.Strategy.DockerStrategy.BuildArgs) != 0 {
		values := []shipwrightv1beta1.SingleValue{}

		for _, buildArg := range bc.Spec.Strategy.DockerStrategy.BuildArgs {
			envNameValue := buildArg.Name + "=" + buildArg.Value
			value := shipwrightv1beta1.SingleValue{
				Value: &envNameValue,
			}
			values = append(values, value)
		}

		buildArgsParam := shipwrightv1beta1.ParamValue{
			Name:   "build-args",
			Values: values,
		}
		b.Spec.ParamValues = append(b.Spec.ParamValues, buildArgsParam)
	}
}

func getBuildFilePath(b shipwrightv1beta1.Build) string {
	return strings.Join([]string{b.GetObjectKind().GroupVersionKind().GroupKind().Kind, b.GetObjectKind().GroupVersionKind().GroupKind().Group, b.GetObjectKind().GroupVersionKind().Version, b.Namespace, b.Name}, "_") + ".yaml"
}
