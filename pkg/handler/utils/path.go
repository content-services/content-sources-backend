package utils

import (
	"strings"
)

type Path []string

func NewPathWithString(path string) Path {
	output := []string{}
	if path == "" || path == "/" {
		return Path(output)
	}
	if path[0] != '/' {
		return Path(output)
	}
	output = strings.Split(path, "/")[1:]
	return Path(output)
}

func (p Path) RemovePrefixes() Path {
	output := []string(p)
	lenOutput := len(output)
	if lenOutput < 4 {
		return Path([]string{})
	}
	if output[0] == "beta" {
		if lenOutput < 5 {
			return Path([]string{})
		}
		output = output[1:]
		lenOutput--
	}
	if output[0] != "api" {
		return []string{}
	}
	output = output[1:]
	lenOutput--
	if output[0] != "content-sources" {
		return []string{}
	}
	output = output[1:]
	lenOutput--
	if output[0][0] != 'v' {
		return []string{}
	}
	output = output[1:]
	return Path(output)
}

func (p Path) HasResources(resources ...[]string) bool {
	lenComponents := len(p)
	for _, r := range resources {
		lenResource := len(r)
		if lenComponents < lenResource {
			continue
		}
		flag := true
		for i := 0; i < lenResource; i++ {
			if p[i] != r[i] {
				flag = false
				break
			}
		}
		if flag {
			return true
		}
	}
	return false
}
