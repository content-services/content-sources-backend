package test

import (
	"embed"
)

//go:embed gpg/repomd.xml
//go:embed gpg/repomd.xml.asc
//go:embed gpg/gpgkey.pub
var f embed.FS

func GpgKey() string {
	data, _ := f.ReadFile("gpg/gpgkey.pub")
	return string(data)
}

func SignedRepomd() string {
	data, _ := f.ReadFile("gpg/repomd.xml")
	return string(data)
}

func RepomdSignature() string {
	data, _ := f.ReadFile("gpg/repomd.xml.asc")
	return string(data)
}
