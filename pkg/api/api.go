package api

const IdentityHeader = "x-rh-identity"

// CollectionMetadataSettable a collection response with settable metadata
type CollectionMetadataSettable interface {
	SetMetadata(meta ResponseMetadata, links Links)
}

type PaginationData struct {
	Limit  int `query:"limit" json:"limit" `  //Number of results to return
	Offset int `query:"offset" json:"offset"` //Offset into the total results
}

type FilterData struct {
	Search              string `query:"search" json:"search" `                              //Search string based query to optionally filter-on
	Arch                string `query:"arch" json:"arch" `                                  //Comma separated list of architecture to optionally filter-on (IE 'x86_64,s390x' would return Repositories with x86_64 or s390x only)
	Version             string `query:"version" json:"version"`                             //Comma separated list of versions to optionally filter-on  (IE '7,8' would return Repositories with versions 7 or 8 only)
	AvailableForArch    string `query:"available_for_arch" json:"available_for_arch"`       //Filter by compatible arch (IE 'x86_64' would return Repositories with the 'x86_64' arch and Repositories where arch is not set)
	AvailableForVersion string `query:"available_for_version" json:"available_for_version"` //Filter by compatible version (IE 7 would return Repositories with the version 7 or where version is not set)
}

type ResponseMetadata struct {
	Limit  int   `query:"limit" json:"limit"`   //Limit of results used for the request
	Offset int   `query:"offset" json:"offset"` //Offset into results used for the request
	Count  int64 `json:"count"`                 //Total count of results
}

type Links struct {
	First string `json:"first"`          //Path to first page of results
	Next  string `json:"next,omitempty"` //Path to next page of results
	Prev  string `json:"prev,omitempty"` //Path to previous page of results
	Last  string `json:"last"`           //Path to last page of results
}
