package models

import (
	"fmt"
	"regexp"
	"sort"

	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/lib/pq"
	"gorm.io/gorm"
)

type RepositoryConfiguration struct {
	Base
	Name                 string         `json:"name" gorm:"default:null"`
	Versions             pq.StringArray `json:"version" gorm:"type:text[],default:null"`
	Arch                 string         `json:"arch" gorm:"default:''"`
	GpgKey               string         `json:"gpg_key" gorm:"default:''"`
	Label                string         `json:"label" gorm:"default:''"`
	MetadataVerification bool           `json:"metadata_verification" gorm:"default:false"`
	ModuleHotfixes       bool           `json:"module_hotfixes" gorm:"default:false"`
	AccountID            string         `json:"account_id" gorm:"default:null"`
	OrgID                string         `json:"org_id" gorm:"default:null"`
	RepositoryUUID       string         `json:"repository_uuid" gorm:"not null"`
	Repository           Repository     `json:"repository,omitempty"`
	Snapshot             bool           `json:"snapshot"`
	DeletedAt            gorm.DeletedAt `json:"deleted_at"`
	LastSnapshotUUID     string         `json:"last_snapshot_uuid" gorm:"default:null"`
	LastSnapshot         *Snapshot      `json:"last_snapshot,omitempty" gorm:"foreignKey:last_snapshot_uuid"`
	LastSnapshotTaskUUID string         `json:"last_snapshot_task_uuid" gorm:"default:null"`
	LastSnapshotTask     *TaskInfo      `json:"last_snapshot_task" gorm:"foreignKey:last_snapshot_task_uuid"`
	Templates            []Template     `gorm:"many2many:templates_repository_configurations"`
}

// When updating a model with gorm, we want to explicitly update any field that is set to
// empty string.  We always fetch the object and then update it before saving
// so every update is the full model of user changeable fields.
// So OrgId and account Id are excluded
func (rc *RepositoryConfiguration) MapForUpdate() map[string]interface{} {
	forUpdate := make(map[string]interface{})
	forUpdate["Name"] = rc.Name
	forUpdate["Arch"] = rc.Arch
	forUpdate["Versions"] = rc.Versions
	forUpdate["GpgKey"] = rc.GpgKey
	forUpdate["MetadataVerification"] = rc.MetadataVerification
	forUpdate["AccountID"] = rc.AccountID
	forUpdate["OrgID"] = rc.OrgID
	forUpdate["RepositoryUUID"] = rc.RepositoryUUID
	forUpdate["snapshot"] = rc.Snapshot
	forUpdate["module_hotfixes"] = rc.ModuleHotfixes
	return forUpdate
}

// BeforeCreate perform validations and sets UUID of Repository Configurations
func (rc *RepositoryConfiguration) BeforeCreate(tx *gorm.DB) error {
	if err := rc.Base.BeforeCreate(tx); err != nil {
		return err
	}
	if err := rc.DedupeVersions(tx); err != nil {
		return err
	}
	if err := rc.ReplaceEmptyValues(tx); err != nil {
		return err
	}
	if err := rc.validate(); err != nil {
		return err
	}
	if err := rc.SetLabel(); err != nil {
		return err
	}
	return nil
}

// BeforeUpdate perform validations of Repository Configurations
func (rc *RepositoryConfiguration) BeforeUpdate(tx *gorm.DB) error {
	if err := rc.DedupeVersions(tx); err != nil {
		return err
	}
	if err := rc.ReplaceEmptyValues(tx); err != nil {
		return err
	}
	if err := rc.validate(); err != nil {
		return err
	}
	return nil
}

func (rc *RepositoryConfiguration) DedupeVersions(tx *gorm.DB) error {
	var versionMap = make(map[string]bool)
	var unique = make(pq.StringArray, 0)
	for i := 0; i < len(rc.Versions); i++ {
		if _, found := versionMap[rc.Versions[i]]; !found {
			versionMap[rc.Versions[i]] = true
			unique = append(unique, rc.Versions[i])
		}
	}
	sort.Strings(unique)
	tx.Statement.SetColumn("Versions", unique)
	return nil
}

func (rc *RepositoryConfiguration) ReplaceEmptyValues(tx *gorm.DB) error {
	if rc.Versions != nil && len(rc.Versions) == 0 {
		tx.Statement.SetColumn("Versions", fmt.Sprintf("{%s}", config.ANY_VERSION))
	}
	if rc.Arch == "" {
		tx.Statement.SetColumn("Arch", config.ANY_ARCH)
	}
	return nil
}

func (rc *RepositoryConfiguration) SetLabel() error {
	label, err := getRepoLabel(*rc)
	if err != nil {
		return err
	}
	rc.Label = label
	return nil
}
func (rc *RepositoryConfiguration) IsRedHat() bool {
	return rc.OrgID == config.RedHatOrg
}

func (rc *RepositoryConfiguration) validate() error {
	var err error
	if rc.Name == "" {
		err = Error{Message: "Name cannot be blank.", Validation: true}
		return err
	}

	if rc.OrgID == "" {
		err = Error{Message: "Org ID cannot be blank.", Validation: true}
		return err
	}

	if rc.RepositoryUUID == "" {
		err = Error{Message: "Repository UUID foreign key cannot be blank.", Validation: true}
		return err
	}

	if rc.Arch != "" && !config.ValidArchLabel(rc.Arch) {
		return Error{Message: fmt.Sprintf("Specified distribution architecture %s is invalid.", rc.Arch),
			Validation: true}
	}
	valid, invalidVer := config.ValidDistributionVersionLabels(rc.Versions)
	if len(rc.Versions) > 0 && !valid {
		return Error{Message: fmt.Sprintf("Specified distribution version %s is invalid.", invalidVer),
			Validation: true}
	}

	if versionContainsAnyAndOthers(rc.Versions) {
		AnyOrErrMsg := fmt.Sprintf("Specified a distribution version of '%s' along with other version types, this is invalid.", config.ANY_VERSION)
		return Error{Message: AnyOrErrMsg, Validation: true}
	}

	return nil
}

func versionContainsAnyAndOthers(arr []string) bool {
	if len(arr) <= 1 {
		return false
	}
	for _, a := range arr {
		if a == config.ANY_VERSION {
			return true
		}
	}
	return false
}

func (in *RepositoryConfiguration) DeepCopyInto(out *RepositoryConfiguration) {
	if in == nil || out == nil || in == out {
		return
	}
	in.Base.DeepCopyInto(&out.Base)
	out.Name = in.Name
	out.Versions = in.Versions
	out.Arch = in.Arch
	out.GpgKey = in.GpgKey
	out.MetadataVerification = in.MetadataVerification
	out.AccountID = in.AccountID
	out.OrgID = in.OrgID
	out.RepositoryUUID = in.RepositoryUUID
}

func (in *RepositoryConfiguration) DeepCopy() *RepositoryConfiguration {
	var out = &RepositoryConfiguration{}
	in.DeepCopyInto(out)
	return out
}

func getRepoLabel(repoConfig RepositoryConfiguration) (string, error) {
	// Replace any nonalphanumeric characters with an underscore
	// e.g: "!!my repo?test15()" => "__my_repo_test15__"
	re, err := regexp.Compile(`[^a-zA-Z0-9:space]`)
	if err != nil {
		return "", err
	}

	if repoConfig.IsRedHat() {
		return repoConfig.Label, nil
	} else {
		return re.ReplaceAllString(repoConfig.Name, "_"), nil
	}
}
