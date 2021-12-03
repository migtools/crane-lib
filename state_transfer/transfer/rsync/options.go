package rsync

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/konveyor/crane-lib/state_transfer/meta"
	metadata "github.com/konveyor/crane-lib/state_transfer/meta"
	transfer "github.com/konveyor/crane-lib/state_transfer/transfer"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	errorsutil "k8s.io/apimachinery/pkg/util/errors"
)

const (
	optRecursive     = "--recursive"
	optSymLinks      = "--links"
	optPermissions   = "--perms"
	optModTimes      = "--times"
	optDeviceFiles   = "--devices"
	optSpecialFiles  = "--specials"
	optOwner         = "--owner"
	optGroup         = "--group"
	optHardLinks     = "--hard-links"
	optPartial       = "--partial"
	optDelete        = "--delete"
	optBwLimit       = "--bwlimit=%d"
	optInfo          = "--info=%s"
	optHumanReadable = "--human-readable"
	optLogFile       = "--log-file=%s"
)

const (
	logFileStdOut = "/dev/stdout"
)

// TransferOptions defines customizeable options for Rsync Transfer
type TransferOptions struct {
	CommandOptions
	SourcePodMeta            transfer.ResourceMetadata
	DestinationPodMeta       transfer.ResourceMetadata
	SourcePodMutations       []meta.PodSpecMutation
	DestinationPodMutations  []meta.PodSpecMutation
	SourceContainerMutations []meta.ContainerMutation
	DestContainerMutations   []meta.ContainerMutation
	username                 string
	password                 string
	rsyncServerImage         string
	rsyncClientImage         string
}

// TransferOption knows how to apply a user provided option to a given TransferOptions
type TransferOption interface {
	ApplyTo(*TransferOptions) error
}

func (t *TransferOptions) Apply(opts ...TransferOption) error {
	errs := []error{}
	for _, opt := range opts {
		if err := opt.ApplyTo(t); err != nil {
			errs = append(errs, err)
		}
	}
	return errorsutil.NewAggregate(errs)
}

// CommandOptions defines options that can be customized in the Rsync command
type CommandOptions struct {
	Recursive     bool
	SymLinks      bool
	Permissions   bool
	ModTimes      bool
	DeviceFiles   bool
	SpecialFiles  bool
	Groups        bool
	Owners        bool
	HardLinks     bool
	Delete        bool
	Partial       bool
	BwLimit       *int
	HumanReadable bool
	LogFile       string
	Info          []string
	Extras        []string
}

// AsRsyncCommandOptions returns validated rsync options and validation errors as two lists
func (c *CommandOptions) AsRsyncCommandOptions() ([]string, error) {
	var errs []error
	opts := []string{}
	if c.Recursive {
		opts = append(opts, optRecursive)
	}
	if c.SymLinks {
		opts = append(opts, optSymLinks)
	}
	if c.Permissions {
		opts = append(opts, optPermissions)
	}
	if c.DeviceFiles {
		opts = append(opts, optDeviceFiles)
	}
	if c.SpecialFiles {
		opts = append(opts, optSpecialFiles)
	}
	if c.ModTimes {
		opts = append(opts, optModTimes)
	}
	if c.Owners {
		opts = append(opts, optOwner)
	}
	if c.Groups {
		opts = append(opts, optGroup)
	}
	if c.HardLinks {
		opts = append(opts, optHardLinks)
	}
	if c.Delete {
		opts = append(opts, optDelete)
	}
	if c.Partial {
		opts = append(opts, optPartial)
	}
	if c.BwLimit != nil {
		if *c.BwLimit > 0 {
			opts = append(opts,
				fmt.Sprintf(optBwLimit, *c.BwLimit))
		} else {
			errs = append(errs, fmt.Errorf("rsync bwlimit value must be a positive integer"))
		}
	}
	if c.HumanReadable {
		opts = append(opts, optHumanReadable)
	}
	if c.LogFile != "" {
		opts = append(opts, fmt.Sprintf(optLogFile, c.LogFile))
	}
	if len(c.Info) > 0 {
		validatedOptions, err := filterRsyncInfoOptions(c.Info)
		errs = append(errs, err)
		opts = append(opts,
			fmt.Sprintf(
				optInfo, strings.Join(validatedOptions, ",")))
	}
	if len(c.Extras) > 0 {
		extraOpts, err := filterRsyncExtraOptions(c.Extras)
		errs = append(errs, err)
		opts = append(opts, extraOpts...)
	}
	return opts, errorsutil.NewAggregate(errs)
}

func filterRsyncInfoOptions(options []string) (validatedOptions []string, err error) {
	var errs []error
	r := regexp.MustCompile(`^[A-Z]+\d?$`)
	for _, opt := range options {
		if r.MatchString(opt) {
			validatedOptions = append(validatedOptions, strings.TrimSpace(opt))
		} else {
			errs = append(errs, fmt.Errorf("invalid value %s for Rsync option --info", opt))
		}
	}
	return validatedOptions, errorsutil.NewAggregate(errs)
}

func filterRsyncExtraOptions(options []string) (validatedOptions []string, err error) {
	var errs []error
	r := regexp.MustCompile(`^\-{1,2}([a-z0-9]+\-){0,}?[a-z0-9]+$`)
	for _, opt := range options {
		if r.MatchString(opt) {
			validatedOptions = append(validatedOptions, opt)
		} else {
			errs = append(errs, fmt.Errorf("invalid Rsync option %s", opt))
		}
	}
	return validatedOptions, errorsutil.NewAggregate(errs)
}

func GetRsyncCommandDefaultOptions() []TransferOption {
	return []TransferOption{
		ArchiveFiles(true),
		StandardProgress(true),
	}
}

type ArchiveFiles bool

func (a ArchiveFiles) ApplyTo(opts *TransferOptions) error {
	opts.Recursive = bool(a)
	opts.SymLinks = bool(a)
	opts.Permissions = bool(a)
	opts.ModTimes = bool(a)
	opts.Groups = bool(a)
	opts.Owners = bool(a)
	opts.DeviceFiles = bool(a)
	opts.SpecialFiles = bool(a)
	return nil
}

type PreserveOwnership bool

func (p PreserveOwnership) ApplyTo(opts *TransferOptions) error {
	opts.Owners = bool(p)
	opts.Groups = bool(p)
	return nil
}

type StandardProgress bool

func (s StandardProgress) ApplyTo(opts *TransferOptions) error {
	opts.Info = []string{
		"COPY2", "DEL2", "REMOVE2", "SKIP2", "FLIST2", "PROGRESS2", "STATS2",
	}
	opts.HumanReadable = true
	opts.LogFile = logFileStdOut
	return nil
}

type DeleteDestination bool

func (d DeleteDestination) ApplyTo(opts *TransferOptions) error {
	opts.Delete = bool(d)
	return nil
}

type WithSourcePodLabels map[string]string

func (w WithSourcePodLabels) ApplyTo(opts *TransferOptions) error {
	err := metadata.ValidateLabels(w)
	if err != nil {
		return err
	}
	opts.SourcePodMeta.Labels = w
	return nil
}

type WithDestinationPodLabels map[string]string

func (w WithDestinationPodLabels) ApplyTo(opts *TransferOptions) error {
	err := metadata.ValidateLabels(w)
	if err != nil {
		return err
	}
	opts.DestinationPodMeta.Labels = w
	return nil
}

type WithOwnerReferences []metav1.OwnerReference

func (w WithOwnerReferences) ApplyTo(opts *TransferOptions) error {
	for _, ref := range w {
		if len(ref.Kind)*len(ref.Name)*len(ref.UID) == 0 {
			return fmt.Errorf("all OwnerReferences must have Kind, Name and UID set")
		}
	}
	opts.SourcePodMeta.OwnerReferences = w
	opts.DestinationPodMeta.OwnerReferences = w
	return nil
}

type SourcePodSpecMutation struct {
	Spec *v1.PodSpec
}

func (s *SourcePodSpecMutation) ApplyTo(opts *TransferOptions) error {
	opts.SourcePodMutations = append(opts.SourcePodMutations,
		meta.NewPodSpecMutation(s.Spec, meta.MutationTypeReplace))
	return nil
}

type DestinationPodSpecMutation struct {
	Spec *v1.PodSpec
}

func (s *DestinationPodSpecMutation) ApplyTo(opts *TransferOptions) error {
	opts.DestinationPodMutations = append(opts.SourcePodMutations,
		meta.NewPodSpecMutation(s.Spec, meta.MutationTypeReplace))
	return nil
}

type SourceContainerMutation struct {
	C *v1.Container
}

func (s SourceContainerMutation) ApplyTo(opts *TransferOptions) error {
	opts.SourceContainerMutations = append(opts.SourceContainerMutations,
		meta.NewContainerMutation(s.C, meta.MutationTypeReplace))
	return nil
}

type DestinationContainerMutation struct {
	C *v1.Container
}

func (s DestinationContainerMutation) ApplyTo(opts *TransferOptions) error {
	opts.DestContainerMutations = append(opts.SourceContainerMutations,
		meta.NewContainerMutation(s.C, meta.MutationTypeReplace))
	return nil
}

type Username string

func (u Username) ApplyTo(opts *TransferOptions) error {
	opts.username = string(u)
	return nil
}

type Password string

func (p Password) ApplyTo(opts *TransferOptions) error {
	opts.password = string(p)
	return nil
}

type RsyncServerImage string

func (r RsyncServerImage) ApplyTo(opts *TransferOptions) error {
	opts.rsyncServerImage = string(r)
	return nil
}

type RsyncClientImage string

func (r RsyncClientImage) ApplyTo(opts *TransferOptions) error {
	opts.rsyncClientImage = string(r)
	return nil
}
