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
	t.Logger.Infof("Converting BuildConfigs in namespace: %s", t.Namespace)
	bcList := buildv1.BuildConfigList{}
	err := t.Client.List(context.TODO(), &bcList, client.InNamespace(t.Namespace))
	if err != nil {
		return err
	}

	err = t.writeBuildConfigs(bcList)
	if err != nil {
		return err
	}

	if len(bcList.Items) > 1 {
		t.Logger.Infof("Found %d BuildConfigs to convert", len(bcList.Items))
	} else if len(bcList.Items) == 1 {
		t.Logger.Infof("Found %d BuildConfig to convert", len(bcList.Items))
	} else {
		t.Logger.Infof("No BuildConfigs found to convert")
		return nil
	}

	for i, bc := range bcList.Items {
		t.Logger.Infof("--------------------------------------------------------")
		t.Logger.Infof("Processing BuildConfig: %s at index %d", bc.Name, i)
		t.Logger.Infof("--------------------------------------------------------")

		b := &shipwrightv1beta1.Build{}
		b.Name = bc.Name
		b.Kind = "Build"
		b.APIVersion = "shipwright.io/v1beta1"
		b.Spec.ParamValues = []shipwrightv1beta1.ParamValue{}
		b.Namespace = bc.Namespace
		b.CreationTimestamp = metav1.NewTime(time.Now())

		switch strategyType := bc.Spec.Strategy.Type; strategyType {
		case BuildStrategyDockerType:
			t.Logger.Infof("Docker strategy detected")
			ClusterBuildStrategyKind := shipwrightv1beta1.ClusterBuildStrategyKind
			b.Spec.Strategy = shipwrightv1beta1.Strategy{
				Kind: &ClusterBuildStrategyKind,
				Name: "buildah",
			}

			// process From field
			if bc.Spec.Strategy.DockerStrategy.From != nil {
				t.Logger.Warnf("From Field in BuildConfig's Docker strategy is not yet supported in built-in Buildah ClusterBuildStrategy in Shipwright. RFE: %s", RuntimeStageFromRFE)
			}

			// process PullSecret field
			pullSecret := t.getPullSecret(&bc)
			if pullSecret != nil {
				// Validate pull secret
				if err := t.validatePullSecret(&bc, pullSecret); err != nil {
					t.Logger.Error("Error validating registry PullSecret")
					return err
				}

				// Generate ServiceAccount for pull secret
				if err := t.generateServiceAccountForPullSecret(&bc); err != nil {
					t.Logger.Error("Error generating service account for registry PullSecret")
					return err
				}
				saName := t.getServiceAccountName(&bc)
				t.Logger.Infof("Registry PullSecret validated and service account %q generated", saName)
			}

			// process NoCache field
			if bc.Spec.Strategy.DockerStrategy.NoCache {
				t.Logger.Warnf("NoCache flag is not yet supported in the built-in Buildah ClusterBuildStrategy in Shipwright. RFE: %s", NoCacheFlagRFE)
			}

			// process env fields
			if bc.Spec.Strategy.DockerStrategy.Env != nil {
				t.Logger.Infof("Processing Environment Variables")
				b.Spec.Env = append(b.Spec.Env, bc.Spec.Strategy.DockerStrategy.Env...)
			}

			// process force pull field
			if bc.Spec.Strategy.DockerStrategy.ForcePull {
				t.Logger.Warnf("ForcePull flag is not yet supported in the built-in Buildah ClusterBuildStrategy in Shipwright. RFE: %s", ForcePullFlagRFE)
			}

			// process docker file path
			if bc.Spec.Strategy.DockerStrategy.DockerfilePath != "" {
				t.Logger.Infof("Processing Dockerfile path")
				dockerfile := shipwrightv1beta1.ParamValue{
					Name: "dockerfile",
					SingleValue: &shipwrightv1beta1.SingleValue{
						Value: &bc.Spec.Strategy.DockerStrategy.DockerfilePath,
					},
				}
				b.Spec.ParamValues = append(b.Spec.ParamValues, dockerfile)
			}

			// process args
			t.processBuildArgs(bc, b)

			// process ImageOptimizationPolicy field
			if bc.Spec.Strategy.DockerStrategy.ImageOptimizationPolicy != nil {
				t.Logger.Warnf("ImageOptimizationPolicy (--squash) flag is not yet supported in the built-in Buildah ClusterBuildStrategy in Shipwright. RFE: %s", SqashFlagRFE)
			}

			// process volumes
			if len(bc.Spec.Strategy.DockerStrategy.Volumes) > 0 {
				t.Logger.Warnf("Unlike BuildConfig, Volumes have to be supported in the Buildah Strategy first in Shipwright. Please raise your requirements here: %s", DockerStrategyVolumesRFE)
			}
		case BuildStrategySourceType:
			t.Logger.Infof("Source strategy detected for BuildConfig: %s", bc.Name)
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

			// process PullSecret field
			pullSecret := t.getPullSecret(&bc)
			if pullSecret != nil {
				// Validate pull secret
				if err := t.validatePullSecret(&bc, pullSecret); err != nil {
					t.Logger.Error("Error validating registry PullSecret")
					return err
				}

				// Generate ServiceAccount for pull secret
				if err := t.generateServiceAccountForPullSecret(&bc); err != nil {
					t.Logger.Error("Error generating service account for registry PullSecret")
					return err
				}
				saName := t.getServiceAccountName(&bc)
				t.Logger.Infof("Registry PullSecret validated and service account %q generated", saName)
			}

			// process env fields
			if bc.Spec.Strategy.SourceStrategy.Env != nil {
				t.Logger.Infof("Processing Environment Variables")
				b.Spec.Env = append(b.Spec.Env, bc.Spec.Strategy.SourceStrategy.Env...)
			}

			// process custom scripts
			if bc.Spec.Strategy.SourceStrategy.Scripts != "" {
				t.Logger.Warnf("Custom scripts are not yet supported in the built-in Source-to-Image ClusterBuildStrategy in Shipwright. RFE: %s", CustomScriptsRFE)
			}

			// process incremental build
			if bc.Spec.Strategy.SourceStrategy.Incremental != nil {
				t.Logger.Warnf("Incremental build is not yet supported in the built-in Source-to-Image ClusterBuildStrategy in Shipwright. RFE: %s", IncrementalBuildRFE)
			}

			// process force pull field
			if bc.Spec.Strategy.SourceStrategy.ForcePull {
				t.Logger.Warnf("ForcePull flag is not yet supported in the built-in Source-to-Image ClusterBuildStrategy in Shipwright. RFE: %s", ForcePullFlagS2iRFE)
			}

			// process volumes
			if len(bc.Spec.Strategy.SourceStrategy.Volumes) > 0 {
				t.Logger.Warnf("Unlike BuildConfig, Volumes have to be supported in the Source-to-Image Strategy first in Shipwright. Please raise your requirements here: %s", DockerStrategyVolumesRFE)
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

// processStrategyFromField processes From field for Source-to-Image strategy
func (t *ConvertOptions) processStrategyFromField(bc *buildv1.BuildConfig, b *shipwrightv1beta1.Build) error {
	// Extract From field from whichever strategy is present
	var from *corev1.ObjectReference
	if bc.Spec.Strategy.DockerStrategy != nil && bc.Spec.Strategy.DockerStrategy.From != nil {
		from = bc.Spec.Strategy.DockerStrategy.From
	} else if bc.Spec.Strategy.SourceStrategy != nil && bc.Spec.Strategy.SourceStrategy.From.Name != "" {
		from = &bc.Spec.Strategy.SourceStrategy.From
	} else {
		t.Logger.Debugf("No From field to process for BuildConfig %s", bc.Name)
		return nil
	}
	t.Logger.Infof("Processing From field: %s", from.Name)

	if from.Kind == "" {
		t.Logger.Debugf("From.Kind is empty for BuildConfig %s, skipping", from.Kind, bc.Name)
		return nil
	}

	if from.Name == "" {
		t.Logger.Debugf("From.Name is empty for BuildConfig %s, skipping", bc.Name)
		return nil
	}

	if from.Namespace == "" {
		ns := bc.Namespace
		from.Namespace = ns
	}

	if b.Spec.ParamValues == nil {
		b.Spec.ParamValues = []shipwrightv1beta1.ParamValue{}
	}

	switch fromKind := from.Kind; fromKind {
	case ImageStreamTag:
		imageRef, err := t.resolveImageStreamRef(from.Name, from.Namespace)
		if err != nil {
			return err
		}
		paramValue := shipwrightv1beta1.ParamValue{
			Name: "builder-image",
			SingleValue: &shipwrightv1beta1.SingleValue{
				Value: &imageRef,
			},
		}
		b.Spec.ParamValues = append(b.Spec.ParamValues, paramValue)
	case ImageStreamImage:
		imageRef, err := t.resolveImageStreamRef(from.Name, from.Namespace)
		if err != nil {
			return err
		}
		paramValue := shipwrightv1beta1.ParamValue{
			Name: "builder-image",
			SingleValue: &shipwrightv1beta1.SingleValue{
				Value: &imageRef,
			},
		}
		b.Spec.ParamValues = append(b.Spec.ParamValues, paramValue)
	case DockerImage:
		// we can use the name directly
		paramValue := shipwrightv1beta1.ParamValue{
			Name: "builder-image",
			SingleValue: &shipwrightv1beta1.SingleValue{
				Value: &from.Name,
			},
		}
		b.Spec.ParamValues = append(b.Spec.ParamValues, paramValue)
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

// getPullSecret returns the PullSecret from either DockerStrategy or SourceStrategy
func (t *ConvertOptions) getPullSecret(bc *buildv1.BuildConfig) *corev1.LocalObjectReference {
	if bc.Spec.Strategy.DockerStrategy != nil && bc.Spec.Strategy.DockerStrategy.PullSecret != nil {
		return bc.Spec.Strategy.DockerStrategy.PullSecret
	}
	if bc.Spec.Strategy.SourceStrategy != nil && bc.Spec.Strategy.SourceStrategy.PullSecret != nil {
		return bc.Spec.Strategy.SourceStrategy.PullSecret
	}
	return nil
}

func (t *ConvertOptions) generateServiceAccountForPullSecret(bc *buildv1.BuildConfig) error {
	// Get PullSecret from either strategy type
	pullSecret := t.getPullSecret(bc)
	if pullSecret == nil {
		return fmt.Errorf("no PullSecret found for BuildConfig %s", bc.Name)
	}

	// Determine ServiceAccount name
	saName := t.getServiceAccountName(bc)

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
				Name: pullSecret.Name,
			},
		},
		Secrets: []corev1.ObjectReference{
			{
				Name: pullSecret.Name,
			},
		},
	}

	serviceAccount.CreationTimestamp = metav1.Now()

	// Write ServiceAccount YAML
	return t.writeServiceAccount(serviceAccount)
}

func (t *ConvertOptions) getServiceAccountName(bc *buildv1.BuildConfig) string {
	saName := bc.Spec.ServiceAccount
	if saName == "" {
		saName = bc.Name
	}
	return saName
}

func (t *ConvertOptions) writeServiceAccount(sa *corev1.ServiceAccount) error {
	targetDir := filepath.Join(t.ExportDir, "builds", sa.Namespace)
	err := os.MkdirAll(targetDir, 0700)
	switch {
	case os.IsExist(err):
	case err != nil:
		t.Logger.Errorf("error creating the serviceaccounts directory: %#v", err)
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
	return strings.Join([]string{sa.GroupVersionKind().Kind, sa.GroupVersionKind().Version, sa.Namespace, sa.Name}, "_") + ".yaml"
}

// processStrategyVolumes is a generic function that processes volumes for any build strategy
func (t *ConvertOptions) processStrategyVolumes(bc *buildv1.BuildConfig, volumes []buildv1.BuildVolume, b *shipwrightv1beta1.Build) error {
	if len(volumes) == 0 {
		return nil
	}

	// Convert BuildConfig volumes to Shipwright volumes
	for _, bcVolume := range volumes {
		t.Logger.Infof("Processing volume: %q", bcVolume.Name)
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
			t.Logger.Errorf("Volume mount paths can not be migrated to Shipwright Build. Mount paths are defined in the BuildStrategy. Original mounts: %v", bcVolume.Mounts)
		}
	}
	return nil
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
		t.Logger.Errorf("Inline Dockerfile is not supported in buildah strategy. Consider moving it to a separate file.")
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
		t.Logger.Errorf("Multiple source types are not supported in a single build in Shipwright. BuildConfig: %s", bc.Name)
		return
		// TODO: May be provide a default Git path to handle this case
	}

	if sourceCount == 0 {
		t.Logger.Warnf("No source type is specified for the build in Shipwright. BuildConfig: %s", bc.Name)
		return
	}

	if git != nil {
		t.Logger.Infof("Processing Git Source")
		var cloneSecret *string
		if bc.Spec.Source.SourceSecret != nil {
			t.Logger.Infof("Processing Git CloneSecret")
			cloneSecret = &bc.Spec.Source.SourceSecret.Name
		}

		git := &shipwrightv1beta1.Git{
			URL:         bc.Spec.Source.Git.URI,
			Revision:    &bc.Spec.Source.Git.Ref,
			CloneSecret: cloneSecret,
		}

		var source *shipwrightv1beta1.Source
		if b.Spec.Source != nil {
			source = b.Spec.Source
			source.Git = git
			source.Type = shipwrightv1beta1.GitType
		} else {
			source = &shipwrightv1beta1.Source{
				Git:  git,
				Type: shipwrightv1beta1.GitType,
			}
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
			t.Logger.Errorf("Archive Source is not supported in Shipwright. BuildConfig: %s", bc.Name)
			return
		}

		t.Logger.Infof("Stream files to build pod using 'shp build upload' command. BuildConfig: %s", bc.Name)
		b.Spec.Source = source

	} else if len(images) != 0 {
		if len(images) > 1 {
			t.Logger.Errorf("Multiple image sources are not supported in a single build in Shipwright. BuildConfig: %s", bc.Name)
			return
		}

		image := images[0]

		if image.As != nil {
			t.Logger.Errorf("Image Source 'As' field is not supported in Shipwright. BuildConfig: %s, Image: %s", bc.Name, image.From.Name)
			return
		}

		if image.Paths != nil {
			t.Logger.Errorf("Image Source 'Paths' field is not supported in Shipwright. BuildConfig: %s, Image: %s", bc.Name, image.From.Name)
			return
		}

		err := t.processBuildSourceFromField(&bc, b, 0)
		if err != nil {
			t.Logger.Errorf("Failed to process image source for BuildConfig %s: Err: %v", bc.Name, err)
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
		t.Logger.Infof("Processing Context Directory")
		contextDir = &bc.Spec.Source.ContextDir
	}
	b.Spec.Source.ContextDir = contextDir

	if bc.Spec.Source.ConfigMaps != nil {
		t.Logger.Warnf("ConfigMaps are not yet supported in Shipwright build environment. RFE: %s", ConfigMapsRFE)
	}

	if bc.Spec.Source.Secrets != nil {
		t.Logger.Warnf("Secrets are not yet supported in Shipwright build environment. RFE: %s", SecretsRFE)
	}
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
		t.Logger.Infof("Added %d proxy environment variables for BuildConfig %s", len(proxyEnvVars), bc.Name)
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
		t.Logger.Warnf("Push to Openshift ImageStreams is not yet supported in Shipwright. RFE: %s", ImageStreamsPushRFE)
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
		t.Logger.Errorf("error creating the resources directory: %#v", err)
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
	return strings.Join([]string{bc.GroupVersionKind().Kind, bc.GroupVersionKind().Group, bc.GroupVersionKind().Version, bc.Namespace, bc.Name}, "_") + ".yaml"
}

func (t *ConvertOptions) writeBuild(b *shipwrightv1beta1.Build) error {
	targetDir := filepath.Join(t.ExportDir, "builds", t.Namespace)
	err := os.MkdirAll(targetDir, 0700)
	switch {
	case os.IsExist(err):
	case err != nil:
		t.Logger.Errorf("error creating the resources directory: %#v", err)
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
		t.Logger.Infof("Processing Build Args")
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
	return strings.Join([]string{b.GroupVersionKind().Kind, b.GroupVersionKind().Group, b.GroupVersionKind().Version, b.Namespace, b.Name}, "_") + ".yaml"
}
