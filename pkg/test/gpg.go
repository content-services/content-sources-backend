package test

import (
	"embed"

	"github.com/content-services/yummy/pkg/yum"
)

//go:embed gpg/repomd.xml
//go:embed gpg/repomd.xml.asc
//go:embed gpg/gpgkey.pub
var f embed.FS

var Repomd = &yum.Repomd{
	RepomdString: SignedRepomd(),
}

func GpgKey() *string {
	data, _ := f.ReadFile("gpg/gpgkey.pub")
	dataString := string(data)
	return &dataString
}

func SignedRepomd() *string {
	data, _ := f.ReadFile("gpg/repomd.xml")
	dataString := string(data)
	return &dataString
}

func RepomdSignature() *string {
	data, _ := f.ReadFile("gpg/repomd.xml.asc")
	dataString := string(data)
	return &dataString
}
