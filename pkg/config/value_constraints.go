package config

import (
	"fmt"
	"time"
)

const (
	StatusValid       = "Valid"       // Repository introspected successfully
	StatusUnavailable = "Unavailable" // Repository introspected at least once, but now errors
	StatusInvalid     = "Invalid"     // Repository has never introspected due to error
	StatusPending     = "Pending"     // Repository not introspected yet.
)

const (
	ContentTypeRpm = "rpm"
)

const (
	OriginExternal  = "external"
	OriginRedHat    = "red_hat"
	OriginUpload    = "upload"
	OriginCommunity = "community"
)

const RedHatOrg = "-1"
const RedHatDomainName = "cs-redhat" // Note the RH domain name may not be set to this in all environments
const RedHatGpgKeyPath = "file:///etc/pki/rpm-gpg/RPM-GPG-KEY-redhat-release"

const CommunityOrg = "-2"
const CommunityDomainName = "cs-community"

const IntrospectTimeInterval = time.Hour * 23

const ANY_VERSION = "any"
const El7 = "7"
const El8 = "8"
const El9 = "9"
const El10 = "10"

const FailedIntrospectionsLimit = 20
const SnapshotForceInterval = 24 // In hours

const FailedSnapshotLimit = 10 // Number of times to retry a snapshot before stopping

const (
	EPEL10Url = "https://dl.fedoraproject.org/pub/epel/10/Everything/x86_64/"
	EPEL9Url  = "https://dl.fedoraproject.org/pub/epel/9/Everything/x86_64/"
	EPEL8Url  = "https://dl.fedoraproject.org/pub/epel/8/Everything/x86_64/"
)

var EPELUrls = []string{EPEL10Url, EPEL9Url, EPEL8Url}

type DistributionVersion struct {
	Name  string `json:"name"`  // Human-readable form of the version
	Label string `json:"label"` // Static label of the version
}

type DistributionMinorVersion struct {
	Name                   string   `json:"name"`
	Label                  string   `json:"label"`
	Major                  string   `json:"major"`
	ExtendedReleaseStreams []string `json:"extended_release_streams"`
}
type DistributionArch struct {
	Name  string `json:"name"`  // Human-readable form of the architecture
	Label string `json:"label"` // Static label of the architecture
}

var DistributionVersions = [...]DistributionVersion{
	{
		Name:  "Any",
		Label: ANY_VERSION,
	},
	{
		Name:  "RHEL 7",
		Label: El7,
	}, {
		Name:  "RHEL 8",
		Label: El8,
	}, {
		Name:  "RHEL 9",
		Label: El9,
	}, {
		Name:  "RHEL 10",
		Label: El10,
	},
}

var ExtendedReleaseLabelToFeature = map[[2]string]string{
	{EUS, X8664}:    "RHEL-EUS-x86_64",
	{EUS, AARCH64}:  "RHEL-EUS-aarch64",
	{E4S, X8664}:    "RHEL-E4S-x86_64",
	{EEUS, X8664}:   "RHEL-EEUS-x86_64",
	{EEUS, AARCH64}: "RHEL-EEUS-aarch64",
}

var DistributionMinorVersions = [...]DistributionMinorVersion{
	{Name: "RHEL 8.6", Label: "8.6", Major: El8, ExtendedReleaseStreams: []string{E4S}},
	{Name: "RHEL 8.8", Label: "8.8", Major: El8, ExtendedReleaseStreams: []string{E4S}},
	{Name: "RHEL 9.0", Label: "9.0", Major: El9, ExtendedReleaseStreams: []string{E4S, EEUS}},
	{Name: "RHEL 9.2", Label: "9.2", Major: El9, ExtendedReleaseStreams: []string{E4S, EEUS}},
	{Name: "RHEL 9.4", Label: "9.4", Major: El9, ExtendedReleaseStreams: []string{EUS, E4S, EEUS}},
	{Name: "RHEL 9.6", Label: "9.6", Major: El9, ExtendedReleaseStreams: []string{EUS, E4S, EEUS}},
	{Name: "RHEL 10.0", Label: "10.0", Major: El10, ExtendedReleaseStreams: []string{EUS, EEUS}},
}

const ANY_ARCH = "any"
const X8664 = "x86_64"
const S390x = "s390x"
const PPC64LE = "ppc64le"
const AARCH64 = "aarch64"

var DistributionArches = [...]DistributionArch{
	{
		Name:  "Any",
		Label: ANY_ARCH,
	},
	{
		Name:  "aarch64",
		Label: AARCH64,
	},
	{
		Name:  "ppc64le",
		Label: PPC64LE,
	},
	{
		Name:  "s390x",
		Label: S390x,
	},
	{
		Name:  "x86_64",
		Label: X8664,
	},
}

const EUS = "eus"
const E4S = "e4s"
const EEUS = "eeus"

// ExtendedReleaseStreamNames maps stream labels to display names
var ExtendedReleaseStreamNames = map[string]string{
	EUS:  "Extended Update Support (EUS)",
	E4S:  "Update Services for SAP Solutions (E4S)",
	EEUS: "Enhanced Extended Update Support (EEUS)",
}

// Features that do not currently use a subscription check, available to all users
var SubscriptionFeaturesIgnored = []string{"RHEL-OS-x86_64", ""}

// Memo Keys
var MemoPulpLastSuccessfulPulpLogParse = "last_successful_pulp_log_date"

// ValidDistributionVersionLabels Given a list of labels, return true
// if every item of the list is a valid distribution version.  If at least one
// is not valid, returns false and the first invalid version
func ValidDistributionVersionLabels(labels []string) (bool, string) {
	for j := 0; j < len(labels); j++ {
		found := false
		for i := 0; i < len(DistributionVersions); i++ {
			if DistributionVersions[i].Label == labels[j] {
				found = true
				break
			}
		}
		if !found {
			return false, labels[j]
		}
	}
	return true, ""
}

// ValidArchLabel Given a label, verifies that the label is a valid distribution
// architecture label
func ValidArchLabel(label string) bool {
	for i := 0; i < len(DistributionArches); i++ {
		if DistributionArches[i].Label == label {
			return true
		}
	}
	return false
}

func ValidDistributionMinorVersionLabel(label string) bool {
	for i := 0; i < len(DistributionMinorVersions); i++ {
		if DistributionMinorVersions[i].Label == label {
			return true
		}
	}
	return false
}

func ValidExtendedReleaseLabel(label string) bool {
	_, exists := ExtendedReleaseStreamNames[label]
	return exists
}

func SnapshotInterval(redHat bool) string {
	if redHat {
		return fmt.Sprintf("%v minutes", 45)
	}
	// 24 hours - 1. Subtract 1, as the next run will be more than 24 hours
	return fmt.Sprintf("%v hours", 23)
}
