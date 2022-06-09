package models

type RepositoryRpm struct {
	Base
	// The rpm package name
	Name string `json:"name" gorm:"index:pkgname;not null"`
	// The architecture that this package belong to
	Arch string `json:"arch" gorm:"primaryKey;not null"`
	// The version for this package
	Version string `json:"version" gorm:"primaryKey;not null"`
	// The release for this package
	Release string `json:"release" gorm:"primaryKey;null"`
	// Epoch is a way to define weighted dependencies based
	// on version numbers. It's default value is 0 and this
	// is assumed if an Epoch directive is not listed in the RPM SPEC file.
	// https://access.redhat.com/documentation/en-us/red_hat_enterprise_linux/8/html/packaging_and_distributing_software/advanced-topics#packaging-epoch_epoch-scriplets-and-triggers
	Epoch *int32 `json:"epoch" gorm:"default:0;not null"`
	// RepositoryConfig reference
	Repo RepositoryConfiguration `json:"repository_config_id" gorm:"foreignkey:uuid"`
}
