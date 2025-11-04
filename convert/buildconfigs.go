package convert

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	buildv1 "github.com/openshift/api/build/v1"
	imagev1 "github.com/openshift/api/image/v1"
	shipwrightv1beta1 "github.com/shipwright-io/build/pkg/apis/build/v1beta1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"
)

const (
	// Build Strategy Types
	BuildStrategyDockerType = "Docker"
	BuildStrategySourceType = "Source"

	// Type of "From" image for Docker Strategy
	ImageStreamTag   = "ImageStreamTag"
	ImageStreamImage = "ImageStreamImage"
	DockerImage      = "DockerImage"

	// Build Source Types
	BuildSourceGit        = "Git"
	BuildSourceDockerfile = "Dockerfile"
	BuildSourceBinary     = "Binary"
	BuildSourceImage      = "Image"
	BuildSourceNone       = "None"

	// Git Proxy Environment Variables
	GitHTTPProxy  = "HTTP_PROXY"
	GitHTTPSProxy = "HTTPS_PROXY"
	GitNoProxy    = "NO_PROXY"

	Timeout = 10 * time.Minute
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
		case BuildStrategyDockerType:
			ClusterBuildStrategyKind := shipwrightv1beta1.ClusterBuildStrategyKind
			b.Spec.Strategy = shipwrightv1beta1.Strategy{
				Kind: &ClusterBuildStrategyKind,
				Name: "buildah",
			}

			// process from field
			if bc.Spec.Strategy.DockerStrategy.From != nil {
				err := t.processStrategyFromField(&bc, b)
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

			// process docker file path
			if bc.Spec.Strategy.DockerStrategy.DockerfilePath != "" {
				dockerfile := shipwrightv1beta1.ParamValue{
					Name: "dockerfile",
					SingleValue: &shipwrightv1beta1.SingleValue{
						Value: &bc.Spec.Strategy.DockerStrategy.DockerfilePath,
					},
				}
				b.Spec.ParamValues = append(b.Spec.ParamValues, dockerfile)
			}

			// process volumes
			if len(bc.Spec.Strategy.DockerStrategy.Volumes) > 0 {
				if err := t.processDockerStrategyVolumes(&bc, b); err != nil {
					return err
				}
			}

			// process args
			t.processBuildArgs(bc, b)
		case BuildStrategySourceType:
			ClusterBuildStrategyKind := shipwrightv1beta1.ClusterBuildStrategyKind
			b.Spec.Strategy = shipwrightv1beta1.Strategy{
				Kind: &ClusterBuildStrategyKind,
				Name: "source-to-image",
			}

			// process From field
			if bc.Spec.Strategy.SourceStrategy.From.Name != "" {
				err := t.processStrategyFromField(&bc, b)
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

			// process volumes
			if len(bc.Spec.Strategy.SourceStrategy.Volumes) > 0 {
				if err := t.processSourceStrategyVolumes(&bc, b); err != nil {
					return err
				}
			}

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

// processStrategyFromField processes From field for any strategy
func (t *ConvertOptions) processStrategyFromField(bc *buildv1.BuildConfig, b *shipwrightv1beta1.Build) error {
	// Extract From field from whichever strategy is present
	var from *corev1.ObjectReference
	if bc.Spec.Strategy.DockerStrategy != nil && bc.Spec.Strategy.DockerStrategy.From != nil {
		from = bc.Spec.Strategy.DockerStrategy.From
	} else if bc.Spec.Strategy.SourceStrategy != nil && bc.Spec.Strategy.SourceStrategy.From.Name != "" {
		from = &bc.Spec.Strategy.SourceStrategy.From
	} else {
		t.logger.Debugf("No From field to process for BuildConfig %s", bc.Name)
		return nil
	}

	if from.Kind == "" {
		t.logger.Debugf("From.Kind is empty for BuildConfig %s, skipping", bc.Name)
		return nil
	}

	if from.Name == "" {
		t.logger.Debugf("From.Name is empty for BuildConfig %s, skipping", bc.Name)
		return nil
	}

	switch fromKind := from.Kind; fromKind {
	case ImageStreamTag:
		imageRef, err := t.resolveImageStreamRef(from.Name, from.Namespace)
		if err != nil {
			return err
		}
		b.Spec.Source = &shipwrightv1beta1.Source{
			Type: shipwrightv1beta1.OCIArtifactType,
			OCIArtifact: &shipwrightv1beta1.OCIArtifact{
				Image: imageRef,
			},
		}
	case ImageStreamImage:
		imageRef, err := t.resolveImageStreamRef(from.Name, from.Namespace)
		if err != nil {
			return err
		}
		b.Spec.Source = &shipwrightv1beta1.Source{
			Type: shipwrightv1beta1.OCIArtifactType,
			OCIArtifact: &shipwrightv1beta1.OCIArtifact{
				Image: imageRef,
			},
		}
	case DockerImage:
		// we can use the name directly
		b.Spec.Source = &shipwrightv1beta1.Source{
			Type: shipwrightv1beta1.OCIArtifactType,
			OCIArtifact: &shipwrightv1beta1.OCIArtifact{
				Image: from.Name,
			},
		}
	default:
		return fmt.Errorf("strategy 'From' kind %s is unknown type %s for BuildConfig %s", fromKind, bc.Spec.Strategy.Type, bc.Name)
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

// processStrategyVolumes is a generic function that processes volumes for any build strategy
func (t *ConvertOptions) processStrategyVolumes(bc *buildv1.BuildConfig, volumes []buildv1.BuildVolume, b *shipwrightv1beta1.Build) error {
	if len(volumes) == 0 {
		return nil
	}

	// Convert BuildConfig volumes to Shipwright volumes
	for _, bcVolume := range volumes {
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

func (t *ConvertOptions) processDockerStrategyVolumes(bc *buildv1.BuildConfig, b *shipwrightv1beta1.Build) error {
	return t.processStrategyVolumes(bc, bc.Spec.Strategy.DockerStrategy.Volumes, b)
}

func (t *ConvertOptions) processSourceStrategyVolumes(bc *buildv1.BuildConfig, b *shipwrightv1beta1.Build) error {
	return t.processStrategyVolumes(bc, bc.Spec.Strategy.SourceStrategy.Volumes, b)
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

// processSource processes the source for the Buildah and Souce Strategies
// Shipwright allows building source only from one type in a single build - Either Git, Local or OCI Images.
func (t *ConvertOptions) processSource(bc buildv1.BuildConfig, b *shipwrightv1beta1.Build) {
	// all possible sources
	git := bc.Spec.Source.Git
	binary := bc.Spec.Source.Binary
	images := bc.Spec.Source.Images
	dockerfile := bc.Spec.Source.Dockerfile

	// Inline dockerfiles are not supported in buildah strategy
	// And this field is not relevant for source strategy
	if dockerfile != nil && bc.Spec.Strategy.Type == BuildStrategyDockerType {
		t.logger.Errorf("Inline Dockerfiles are not supported in buildah strategy. BuildConfig: %s, Filename: %v", bc.Name, dockerfile)
	}

	sourceCount := 0
	if git != nil {
		sourceCount++
	}
	if binary != nil {
		sourceCount++
	}
	if len(images) > 0 {
		sourceCount++
	}

	if sourceCount > 1 {
		t.logger.Errorf("Multiple source types are not supported in a single build in Shipwright. BuildConfig: %s", bc.Name)
		return
		// TODO: May be provide a default Git path to handle this case
	}

	if sourceCount == 0 {
		t.logger.Warnf("No source type is specified for the build in Shipwright. BuildConfig: %s", bc.Name)
		return
	}

	if git != nil {
		var cloneSecret *string
		if bc.Spec.Source.SourceSecret != nil {
			cloneSecret = &bc.Spec.Source.SourceSecret.Name
		}

		git := &shipwrightv1beta1.Git{
			URL:         bc.Spec.Source.Git.URI,
			Revision:    &bc.Spec.Source.Git.Ref,
			CloneSecret: cloneSecret,
		}

		source := &shipwrightv1beta1.Source{
			Git:  git,
			Type: shipwrightv1beta1.GitType,
		}
		b.Spec.Source = source
		t.processGitProxyConfig(bc, b)

	} else if binary != nil {
		source := &shipwrightv1beta1.Source{
			Type: shipwrightv1beta1.LocalType,
			Local: &shipwrightv1beta1.Local{
				Name: "local-copy",
				Timeout: &metav1.Duration{
					Duration: Timeout,
				},
			},
		}

		if bc.Spec.Source.ContextDir != "" {
			source.ContextDir = &bc.Spec.Source.ContextDir
		}

		// BuildConfig supports both archive and single file as binary source
		// Shipwright does not support archive
		if bc.Spec.Source.Binary.AsFile != "" {
			t.logger.Errorf("Archive Source is not supported in Shipwright. BuildConfig: %s", bc.Name)
			return
		}

		t.logger.Infof("Stream files to build pod using 'shp build upload' command. BuildConfig: %s", bc.Name)
		b.Spec.Source = source

	} else if len(images) != 0 {
		if len(images) > 1 {
			t.logger.Errorf("Multiple image sources are not supported in a single build in Shipwright. BuildConfig: %s", bc.Name)
			return
		}

		image := images[0]

		if image.As != nil {
			t.logger.Errorf("Image Source 'As' field is not supported in Shipwright. BuildConfig: %s, Image: %s", bc.Name, image.From.Name)
			return
		}

		if image.Paths != nil {
			t.logger.Errorf("Image Source 'Paths' field is not supported in Shipwright. BuildConfig: %s, Image: %s", bc.Name, image.From.Name)
			return
		}

		err := t.processBuildSourceFromField(&bc, b, 0)
		if err != nil {
			t.logger.Errorf("Failed to process image source for BuildConfig %s: Err: %v", bc.Name, err)
			return
		}

		source := &shipwrightv1beta1.Source{
			Type: shipwrightv1beta1.OCIArtifactType,
			OCIArtifact: &shipwrightv1beta1.OCIArtifact{
				Image: image.From.Name,
			},
		}

		if image.PullSecret != nil {
			source.OCIArtifact.PullSecret = &image.PullSecret.Name
		}

		b.Spec.Source = source
	}

	var contextDir *string
	if bc.Spec.Source.ContextDir != "" {
		contextDir = &bc.Spec.Source.ContextDir
	}

	b.Spec.Source.ContextDir = contextDir
}

// processGitProxyConfig processes Git proxy configuration from BuildConfig and adds it as environment variables to Shipwright Build
func (t *ConvertOptions) processGitProxyConfig(bc buildv1.BuildConfig, b *shipwrightv1beta1.Build) {
	// Check if Git source has proxy configuration
	if bc.Spec.Source.Git == nil {
		return
	}

	proxyConfig := bc.Spec.Source.Git.ProxyConfig
	proxyEnvVars := []corev1.EnvVar{}

	// Add HTTP proxy if specified
	if proxyConfig.HTTPProxy != nil && *proxyConfig.HTTPProxy != "" {
		proxyEnvVars = append(proxyEnvVars, corev1.EnvVar{
			Name:  GitHTTPProxy,
			Value: *proxyConfig.HTTPProxy,
		})
	}

	// Add HTTPS proxy if specified
	if proxyConfig.HTTPSProxy != nil && *proxyConfig.HTTPSProxy != "" {
		proxyEnvVars = append(proxyEnvVars, corev1.EnvVar{
			Name:  GitHTTPSProxy,
			Value: *proxyConfig.HTTPSProxy,
		})
	}

	// Add NO_PROXY if specified
	if proxyConfig.NoProxy != nil && *proxyConfig.NoProxy != "" {
		proxyEnvVars = append(proxyEnvVars, corev1.EnvVar{
			Name:  GitNoProxy,
			Value: *proxyConfig.NoProxy,
		})
	}

	// Add proxy environment variables to the Build spec
	if len(proxyEnvVars) > 0 {
		b.Spec.Env = append(b.Spec.Env, proxyEnvVars...)
		t.logger.Debugf("Added %d proxy environment variables for BuildConfig %s", len(proxyEnvVars), bc.Name)
	}
}

// processBuildSourceFromField processes From field for any strategy
func (t *ConvertOptions) processBuildSourceFromField(bc *buildv1.BuildConfig, b *shipwrightv1beta1.Build, index int) error {
	fromImage := bc.Spec.Source.Images[index].From
	if fromImage.Name == "" {
		return fmt.Errorf("image name is empty")
	}

	switch fromKind := fromImage.Kind; fromKind {
	case ImageStreamTag:
		imageRef, err := t.resolveImageStreamRef(fromImage.Name, fromImage.Namespace)
		if err != nil {
			return fmt.Errorf("failed to resolve image stream tag: %v", err)
		}
		b.Spec.Source = &shipwrightv1beta1.Source{
			Type: shipwrightv1beta1.OCIArtifactType,
			OCIArtifact: &shipwrightv1beta1.OCIArtifact{
				Image: imageRef,
			},
		}
	case ImageStreamImage:
		imageRef, err := t.resolveImageStreamRef(fromImage.Name, fromImage.Namespace)
		if err != nil {
			return fmt.Errorf("failed to resolve image stream image: %v", err)
		}
		b.Spec.Source = &shipwrightv1beta1.Source{
			Type: shipwrightv1beta1.OCIArtifactType,
			OCIArtifact: &shipwrightv1beta1.OCIArtifact{
				Image: imageRef,
			},
		}
	case DockerImage:
		// we can use the name directly
		b.Spec.Source = &shipwrightv1beta1.Source{
			Type: shipwrightv1beta1.OCIArtifactType,
			OCIArtifact: &shipwrightv1beta1.OCIArtifact{
				Image: fromImage.Name,
			},
		}
	default:
		return fmt.Errorf("BuildSource 'From' kind %s is unknown type %s for BuildConfig %s", fromKind, bc.Spec.Source.Type, bc.Name)
	}
	return nil
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
