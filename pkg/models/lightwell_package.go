package models

const (
	TableNameLightwellPackage        = "lightwell_packages"
	TableNameLightwellPackageVersion = "lightwell_package_versions"
)

// LightwellPackage is one mirrored package (per repo config, group, name).
type LightwellPackage struct {
	Base
	RepositoryConfigurationUUID string `json:"repository_configuration_uuid" gorm:"not null"`
	Name                        string `json:"name" gorm:"not null"`
	Group                       string `json:"group" gorm:"column:package_group;not null;default:''"`
}

func (LightwellPackage) TableName() string { return TableNameLightwellPackage }

// LightwellPackageVersion is one mirrored version of a LightwellPackage.
type LightwellPackageVersion struct {
	Base
	LightwellPackageUUID        string `json:"lightwell_package_uuid" gorm:"not null"`
	RepositoryConfigurationUUID string `json:"repository_configuration_uuid" gorm:"not null"`
	Version                     string `json:"version" gorm:"not null"`
	Release                     string `json:"release" gorm:"not null;default:''"`
	PublishedAt                 string `json:"published_at" gorm:"not null;default:''"`
	Purl                        string `json:"purl" gorm:"not null;default:''"`
	UpstreamVersion             string `json:"upstream_version" gorm:"not null;default:''"`
	ProjectURL                  string `json:"project_url" gorm:"not null;default:''"`
	License                     string `json:"license" gorm:"not null;default:''"`
	Summary                     string `json:"summary" gorm:"not null;default:''"`
	Description                 string `json:"description" gorm:"not null;default:''"`
	Author                      string `json:"author" gorm:"not null;default:''"`
	AuthorEmail                 string `json:"author_email" gorm:"not null;default:''"`
}

func (LightwellPackageVersion) TableName() string { return TableNameLightwellPackageVersion }
