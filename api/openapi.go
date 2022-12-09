// Used for embedding & fetching the openapi doc at build time, so it can be retrieved
package api

import (
	"embed"
)

//go:embed openapi.json
var fs embed.FS

func Openapi() ([]byte, error) {
	return fs.ReadFile("openapi.json")
}
