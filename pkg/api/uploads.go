package api

type AddUploadsRequest struct {
	Uploads   []Upload   `json:"uploads"`   // List of unfinished uploads
	Artifacts []Artifact `json:"artifacts"` // List of created artifacts
}

type Upload struct {
	Href   string `json:"href"`   // HREF to the unfinished upload
	Sha256 string `json:"sha256"` // SHA256 sum of the uploaded file
}

type Artifact struct {
	Href   string // HREF to the  completed artifact
	Sha256 string // SHA256 sum of the completed artifact
}
