package rsync

import (
	"fmt"
	"regexp"
	"strings"

	labels "github.com/konveyor/crane-lib/state_transfer/labels"
	transfer "github.com/konveyor/crane-lib/state_transfer/transfer"
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
	sourceResourceMetadata      transfer.ResourceMetadata
	destinationResourceMetadata transfer.ResourceMetadata
}

// TransferOption
type TransferOption interface {
	ApplyTo(*TransferOptions) error
}

func (rto *TransferOptions) Apply(opts ...TransferOption) (err error) {
	errs := []string{}
	for _, opt := range opts {
		if err := opt.ApplyTo(rto); err != nil {
			errs = append(errs, err.Error())
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("failed applying options with errs: ['%s']", strings.Join(errs, ","))
	}
	return nil
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
	BwLimit       int
	HumanReadable bool
	LogFile       string
	Info          []string
	Extras        []string
}

// AsRsyncCommandOptions returns validated rsync options and validation errors as two lists
func (rco *CommandOptions) AsRsyncCommandOptions() ([]string, error) {
	var errs []error
	opts := []string{}
	if rco.Recursive {
		opts = append(opts, optRecursive)
	}
	if rco.SymLinks {
		opts = append(opts, optSymLinks)
	}
	if rco.Permissions {
		opts = append(opts, optPermissions)
	}
	if rco.DeviceFiles {
		opts = append(opts, optDeviceFiles)
	}
	if rco.SpecialFiles {
		opts = append(opts, optSpecialFiles)
	}
	if rco.ModTimes {
		opts = append(opts, optModTimes)
	}
	if rco.Owners {
		opts = append(opts, optOwner)
	}
	if rco.Groups {
		opts = append(opts, optGroup)
	}
	if rco.HardLinks {
		opts = append(opts, optHardLinks)
	}
	if rco.Delete {
		opts = append(opts, optDelete)
	}
	if rco.Partial {
		opts = append(opts, optPartial)
	}
	if rco.BwLimit > 0 {
		opts = append(opts,
			fmt.Sprintf(optBwLimit, rco.BwLimit))
	} else {
		errs = append(errs, fmt.Errorf("rsync bwlimit value must be a positive integer"))
	}
	if rco.HumanReadable {
		opts = append(opts, optHumanReadable)
	}
	if rco.LogFile != "" {
		opts = append(opts, fmt.Sprintf(optLogFile, rco.LogFile))
	}
	if len(rco.Info) > 0 {
		validatedOptions, err := filterRsyncInfoOptions(rco.Info)
		errs = append(errs, err)
		opts = append(opts,
			fmt.Sprintf(
				optInfo, strings.Join(validatedOptions, ",")))
	}
	if len(rco.Extras) > 0 {
		extraOpts, err := filterRsyncExtraOptions(rco.Extras)
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
	r := regexp.MustCompile(`^\-{1,2}([a-z]+\-)?[a-z]+$`)
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

func (rca ArchiveFiles) ApplyTo(opts *TransferOptions) error {
	opts.Recursive = bool(rca)
	opts.SymLinks = bool(rca)
	opts.Permissions = bool(rca)
	opts.ModTimes = bool(rca)
	opts.Groups = bool(rca)
	opts.Owners = bool(rca)
	opts.DeviceFiles = bool(rca)
	opts.SpecialFiles = bool(rca)
	return nil
}

type PreserveOwnership bool

func (rca PreserveOwnership) ApplyTo(opts *TransferOptions) error {
	opts.Owners = bool(rca)
	opts.Groups = bool(rca)
	return nil
}

type StandardProgress bool

func (rca StandardProgress) ApplyTo(opts *TransferOptions) error {
	opts.Info = []string{
		"COPY2", "DEL2", "REMOVE2", "SKIP2", "FLIST2", "PROGRESS2", "STATS2",
	}
	opts.HumanReadable = true
	opts.LogFile = logFileStdOut
	return nil
}

type DeleteDestination bool

func (rcad DeleteDestination) ApplyTo(opts *TransferOptions) error {
	opts.Delete = bool(rcad)
	return nil
}

type WithSourcePodLabels map[string]string

func (wspa WithSourcePodLabels) ApplyTo(opts *TransferOptions) error {
	err := labels.ValidateLabels(wspa)
	if err != nil {
		return err
	}
	opts.sourceResourceMetadata.Labels = wspa
	return nil
}

type WithDestinationPodLabels map[string]string

func (wdpa WithDestinationPodLabels) ApplyTo(opts *TransferOptions) error {
	err := labels.ValidateLabels(wdpa)
	if err != nil {
		return err
	}
	opts.destinationResourceMetadata.Labels = wdpa
	return nil
}

type WithOwnerReferences []metav1.OwnerReference

func (woa WithOwnerReferences) ApplyTo(opts *TransferOptions) error {
	for _, ref := range woa {
		if len(ref.Kind)*len(ref.Name)*len(ref.UID) == 0 {
			return fmt.Errorf("all OwnerReferences must have Kind, Name and UID set")
		}
	}
	opts.sourceResourceMetadata.OwnerReferences = woa
	opts.destinationResourceMetadata.OwnerReferences = woa
	return nil
}
