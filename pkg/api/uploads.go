package api

type AddUploadsRequest struct {
	Uploads   []Upload
	Artifacts []Artifact
}

type Upload struct {
	Href   string
	Sha256 string
}

type Artifact struct {
	Href   string
	Sha256 string
}
