package models

import (
	"strings"
	"time"

	"github.com/openlyinc/pointy"
	"gorm.io/gorm"
)

// https://stackoverflow.com/questions/43587610/preventing-null-or-empty-string-values-in-the-db
type Repository struct {
	Base
	URL                          string `gorm:"unique;not null;default:null"`
	RepomdChecksum               string `gorm:"default:null"`
	Public                       bool
	LastIntrospectionTime        *time.Time                `gorm:"default:null"`
	LastIntrospectionSuccessTime *time.Time                `gorm:"default:null"`
	LastIntrospectionUpdateTime  *time.Time                `gorm:"default:null"`
	LastIntrospectionError       *string                   `gorm:"default:null"`
	Status                       string                    `gorm:"default:Pending"`
	PackageCount                 int                       `gorm:"default:0;not null"`
	FailedIntrospectionsCount    int                       `gorm:"default:0;not null"`
	RepositoryConfigurations     []RepositoryConfiguration `gorm:"foreignKey:RepositoryUUID"`
	Rpms                         []Rpm                     `gorm:"many2many:repositories_rpms"`
	Origin                       string                    `gorm:"default:external;not null"`
	ContentType                  string                    `gorm:"default:rpm;not null"`
}

func (r *Repository) BeforeCreate(tx *gorm.DB) (err error) {
	if err := r.Base.BeforeCreate(tx); err != nil {
		return err
	}
	if err := r.validate(); err != nil {
		return err
	}
	r.URL = CleanupURL(r.URL)
	return nil
}

func (r *Repository) validate() error {
	if r.URL == "" {
		return Error{Message: "URL cannot be blank.", Validation: true}
	}
	if stringContainsInternalWhitespace(r.URL) {
		return Error{Message: "URL cannot contain whitespace.", Validation: true}
	}
	return nil
}

// stringContainsInternalWhitespace returns true if string has whitespace, excluding leading/trailing whitespace
func stringContainsInternalWhitespace(s string) bool {
	return strings.ContainsAny(strings.TrimSpace(s), " \t\n\v\r\f")
}

// CleanupURL removes leading and trailing whitespace and adds trailing slash
func CleanupURL(url string) string {
	url = strings.TrimSpace(url)
	// remove all trailing slashes
	for len(url) > 0 && url[len(url)-1] == '/' {
		url = url[0 : len(url)-1]
	}
	if url != "" {
		url += "/" // make sure URL has one trailing slash
	}
	return url
}

func (in *Repository) DeepCopy() *Repository {
	out := &Repository{}
	in.DeepCopyInto(out)
	return out
}

func (in *Repository) DeepCopyInto(out *Repository) {
	if in == nil || out == nil || in == out {
		return
	}
	in.Base.DeepCopyInto(&out.Base)

	var (
		lastIntrospectionTime        *time.Time
		lastIntrospectionUpdateTime  *time.Time
		lastIntrospectionSuccessTime *time.Time
		lastIntrospectionError       *string
	)

	if in.LastIntrospectionTime != nil {
		lastIntrospectionTime = &time.Time{}
		*lastIntrospectionTime = *in.LastIntrospectionTime
	}
	if in.LastIntrospectionUpdateTime != nil {
		lastIntrospectionUpdateTime = &time.Time{}
		*lastIntrospectionUpdateTime = *in.LastIntrospectionUpdateTime
	}
	if in.LastIntrospectionSuccessTime != nil {
		lastIntrospectionSuccessTime = &time.Time{}
		*lastIntrospectionSuccessTime = *in.LastIntrospectionSuccessTime
	}
	if in.LastIntrospectionError != nil {
		lastIntrospectionError = pointy.String(*in.LastIntrospectionError)
	}
	out.URL = in.URL
	out.Public = in.Public
	out.LastIntrospectionTime = lastIntrospectionTime
	out.LastIntrospectionSuccessTime = lastIntrospectionSuccessTime
	out.LastIntrospectionUpdateTime = lastIntrospectionUpdateTime
	out.LastIntrospectionError = lastIntrospectionError
	out.Status = in.Status
	out.PackageCount = in.PackageCount
	out.FailedIntrospectionsCount = in.FailedIntrospectionsCount

	// Duplicate the slices
	out.RepositoryConfigurations = make([]RepositoryConfiguration, len(in.RepositoryConfigurations))
	for i, item := range in.RepositoryConfigurations {
		item.DeepCopyInto(&out.RepositoryConfigurations[i])
	}
	out.Rpms = make([]Rpm, len(in.Rpms))
	for i, item := range in.Rpms {
		item.DeepCopyInto(&out.Rpms[i])
	}
}

func (r *Repository) MapForUpdate() map[string]interface{} {
	forUpdate := make(map[string]interface{})
	forUpdate["URL"] = r.URL
	forUpdate["Public"] = r.Public
	forUpdate["RepomdChecksum"] = r.RepomdChecksum
	forUpdate["LastIntrospectionTime"] = r.LastIntrospectionTime
	forUpdate["LastIntrospectionError"] = trimString(r.LastIntrospectionError, 255)
	forUpdate["LastIntrospectionSuccessTime"] = r.LastIntrospectionSuccessTime
	forUpdate["LastIntrospectionUpdateTime"] = r.LastIntrospectionUpdateTime
	forUpdate["Status"] = r.Status
	forUpdate["PackageCount"] = r.PackageCount
	forUpdate["FailedIntrospectionsCount"] = r.FailedIntrospectionsCount
	return forUpdate
}
