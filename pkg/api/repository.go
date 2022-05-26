package api

import "github.com/content-services/content-sources-backend/pkg/models"

// Repository holds data received from, or returned by, a repositories API request or response
type Repository struct {
	UUID                string `json:"uuid" readonly:"true"`
	Name                string `json:"name"`
	URL                 string `json:"url"`                                //URL of the remote yum repository
	DistributionVersion string `json:"distribution_version" example:"7"`   //Version to restrict client usage to
	DistributionArch    string `json:"distribution_arch" example:"x86_64"` //Architecture to restrict client usage to
	AccountID           string `json:"account_id" readonly:"true"`         //Account ID of the owner
	OrgID               string `json:"org_id" readonly:"true"`             //Organization ID of the owner
}

// CreateRepository holds data received from request to create repository
type CreateRepository struct {
	UUID                string `json:"uuid" readonly:"true" swaggerignore:"true"`
	Name                string `json:"name"`
	URL                 string `json:"url"`                                             //URL of the remote yum repository
	DistributionVersion string `json:"distribution_version" example:"7"`                //Version to restrict client usage to
	DistributionArch    string `json:"distribution_arch" example:"x86_64"`              //Architecture to restrict client usage to
	AccountID           string `json:"account_id" readonly:"true" swaggerignore:"true"` //Account ID of the owner
	OrgID               string `json:"org_id" readonly:"true" swaggerignore:"true"`     //Organization ID of the owner
}

func (r *Repository) FromRepositoryConfiguration(repoConfig models.RepositoryConfiguration) {
	r.UUID = repoConfig.UUID
	r.Name = repoConfig.Name
	r.URL = repoConfig.URL
	r.DistributionVersion = repoConfig.Version
	r.DistributionArch = repoConfig.Arch
	r.AccountID = repoConfig.AccountID
	r.OrgID = repoConfig.OrgID
}

type RepositoryCollectionResponse struct {
	Data  []Repository     `json:"data"`  //Requested Data
	Meta  ResponseMetadata `json:"meta"`  //Metadata about the request
	Links Links            `json:"links"` //Links to other pages of results
}

func (r *RepositoryCollectionResponse) SetMetadata(meta ResponseMetadata, links Links) {
	r.Meta = meta
	r.Links = links
}
