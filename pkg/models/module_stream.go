package models

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"

	"github.com/lib/pq"
	"gorm.io/gorm"
)

const TableNameModuleStream = "module_streams"

type ModuleStream struct {
	Base
	Name         string               `json:"name" gorm:"not null"`
	Stream       string               `json:"stream"`
	Version      string               `json:"version" gorm:"type:text"`
	Context      string               `json:"context"`
	Arch         string               `json:"arch"`
	Summary      string               `json:"summary"`
	Description  string               `json:"description"`
	PackageNames pq.StringArray       `json:"package_names" gorm:"type:text"`
	Packages     pq.StringArray       `json:"packages" gorm:"type:text"`
	Profiles     ModuleStreamProfiles `json:"profiles" gorm:"type:jsonb,not null,default:{}"`
	HashValue    string               `json:"hash" gorm:"not null"`
	Repositories []Repository         `gorm:"many2many:repositories_module_streams"`
}

func (r *ModuleStream) ToHashString() string {
	return fmt.Sprintf("%v-%v-%v-%v-%v-%v-%v", r.Name, r.Stream, r.Version, r.Context, r.Arch, r.Description, r.PackageNames)
}

// BeforeCreate hook performs validations and sets UUID of RepositoryPackageGroup
func (r *ModuleStream) BeforeCreate(tx *gorm.DB) (err error) {
	if err := r.Base.BeforeCreate(tx); err != nil {
		return err
	}
	// Ensure a default of empty
	if r.Profiles == nil {
		r.Profiles = ModuleStreamProfiles{}
	}
	if r.Packages == nil {
		r.Packages = []string{}
	}
	if r.PackageNames == nil {
		r.PackageNames = []string{}
	}
	return nil
}

func (in *ModuleStream) DeepCopy() *ModuleStream {
	out := &ModuleStream{}
	in.DeepCopyInto(out)
	return out
}

func (in *ModuleStream) DeepCopyInto(out *ModuleStream) {
	if in == nil || out == nil || in == out {
		return
	}
	in.Base.DeepCopyInto(&out.Base)
	out.Name = in.Name
	out.Description = in.Description
	out.Stream = in.Stream
	out.Version = in.Version
	out.Context = in.Context
	out.Arch = in.Arch
	out.Summary = in.Summary
	out.Packages = in.Packages
	out.Profiles = in.Profiles
	out.PackageNames = in.PackageNames

	out.Repositories = make([]Repository, len(in.Repositories))
	for i, item := range in.Repositories {
		item.DeepCopyInto(&out.Repositories[i])
	}
}

type ModuleStreamProfiles map[string][]string

func (p *ModuleStreamProfiles) Value() (driver.Value, error) {
	if *p == nil {
		return "{}", nil
	}
	j, err := json.Marshal(p)
	return j, err
}

func (p *ModuleStreamProfiles) Scan(src interface{}) error {
	source, ok := src.([]byte)
	if !ok {
		return fmt.Errorf("type assertion .([]byte) failed")
	}

	var profiles ModuleStreamProfiles
	err := json.Unmarshal(source, &profiles)
	if err != nil {
		return err
	}

	*p = profiles
	return nil
}
