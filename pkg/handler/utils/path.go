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
	idx := 0
	if lenOutput < 4 {
		return []string{}
	}
	if output[idx] == "beta" {
		if lenOutput < 5 {
			return []string{}
		}
		idx++
	}
	if output[idx] != "api" {
		return []string{}
	}
	idx++
	if output[idx] != "content-sources" {
		return []string{}
	}
	idx++
	if output[idx][0] != 'v' {
		return []string{}
	}
	idx++
	return output[idx:]
}

func (p Path) StartWithResources(resources ...[]string) bool {
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
