package config

import "time"

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
	OriginExternal = "external"
	OriginRedHat   = "red_hat"
	OriginUpload   = "upload"
)

const RedHatOrg = "-1"
const RedHatDomainName = "cs-redhat" // Note the RH domain name may not be set to this in all environments
const RedHatGpgKeyPath = "file:///etc/pki/rpm-gpg/RPM-GPG-KEY-redhat-release"

const IntrospectTimeInterval = time.Hour * 23

const ANY_VERSION = "any"
const El7 = "7"
const El8 = "8"
const El9 = "9"

const FailedIntrospectionsLimit = 20
const SnapshotForceInterval = 24 // In hours

type DistributionVersion struct {
	Name  string `json:"name"`  // Human-readable form of the version
	Label string `json:"label"` // Static label of the version
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
		Name:  "el7",
		Label: El7,
	}, {
		Name:  "el8",
		Label: El8,
	}, {
		Name:  "el9",
		Label: El9,
	},
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

// Features that do not currently use a subscription check, available to all users
var SubscriptionFeaturesIgnored = []string{"RHEL-OS-x86_64", ""}

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
