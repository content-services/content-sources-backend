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
