package main

import (
	"encoding/json"
	"fmt"
	"os"
)

type OpenAPISpec struct {
	Paths      map[string]PathItem    `json:"paths"`
	Components map[string]interface{} `json:"components"`
}

type PathItem struct {
	Get    Operation `json:"get,omitempty"`
	Put    Operation `json:"put,omitempty"`
	Post   Operation `json:"post,omitempty"`
	Delete Operation `json:"delete,omitempty"`
	Patch  Operation `json:"patch,omitempty"`
}

type Operation struct {
	Tags []string `json:"tags"`
}

func main() {
	if len(os.Args) < 2 {
		panic("Usage: ./command openapi_spec")
	}

	openapiFile := os.Args[1]

	bytes, err := os.ReadFile(openapiFile)
	if err != nil {
		panic(err)
	}

	var document OpenAPISpec
	err = json.Unmarshal(bytes, &document)
	if err != nil {
		panic(err)
	}

	hasMultipleTags := false

	for path, pathItem := range document.Paths {
		methods := map[string]Operation{
			"get":    pathItem.Get,
			"put":    pathItem.Put,
			"post":   pathItem.Post,
			"delete": pathItem.Delete,
			"patch":  pathItem.Patch,
		}
		for method, operation := range methods {
			if len(operation.Tags) > 1 {
				fmt.Printf("Path '%s' method '%s' has multiple tags: %v\n", path, method, operation.Tags)
				hasMultipleTags = true
			}
		}
	}

	if hasMultipleTags {
		os.Exit(1)
	}
}
