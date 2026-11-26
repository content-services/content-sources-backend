package api

// PythonPackageFileResponse describes a discovered distribution file. Unknown
// metadata remains null, including size when it is not known to be zero bytes.
type PythonPackageFileResponse struct {
	Filename    string  `json:"filename" binding:"required"`                                 // Actual inventory filename
	Category    string  `json:"category" binding:"required" enums:"package,unknown"`         // File category; clients should tolerate future values
	Packagetype *string `json:"packagetype" binding:"required" extensions:"x-nullable"`      // Pulp type, inferred from the filename only when absent
	Size        *int64  `json:"size" binding:"required" minimum:"0" extensions:"x-nullable"` // File size in bytes, or null when unknown
	Sha256      *string `json:"sha256" binding:"required" extensions:"x-nullable"`           // Lowercase SHA-256 digest, or null when unknown
}

type PythonPackageFileCollectionResponse struct {
	Data  []PythonPackageFileResponse `json:"data"`
	Meta  ResponseMetadata            `json:"meta"`
	Links Links                       `json:"links"`
}

func (r *PythonPackageFileCollectionResponse) SetMetadata(meta ResponseMetadata, links Links) {
	r.Meta = meta
	r.Links = links
}
