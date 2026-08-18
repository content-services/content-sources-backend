package parser

import "strings"

// parsePURL extracts package info from a Package URL string.
// Returns nil for unsupported ecosystems or malformed PURLs.
func parsePURL(raw string) *Package {
	rest, ok := strings.CutPrefix(raw, "pkg:")
	if !ok {
		return nil
	}

	typePart, remainder, ok := strings.Cut(rest, "/")
	if !ok || remainder == "" {
		return nil
	}

	var ecosystem string
	switch typePart {
	case "maven":
		ecosystem = EcosystemJava
	case "pypi":
		ecosystem = EcosystemPython
	default:
		return nil
	}

	if idx := strings.IndexAny(remainder, "?#"); idx >= 0 {
		remainder = remainder[:idx]
	}

	var nameBlock, version string
	if idx := strings.LastIndex(remainder, "@"); idx >= 0 {
		nameBlock = remainder[:idx]
		version = remainder[idx+1:]
	} else {
		nameBlock = remainder
	}

	var namespace, name string
	if idx := strings.LastIndex(nameBlock, "/"); idx >= 0 {
		namespace = nameBlock[:idx]
		name = nameBlock[idx+1:]
	} else {
		name = nameBlock
	}

	if name == "" {
		return nil
	}

	return &Package{
		Ecosystem: ecosystem,
		Name:      name,
		Version:   version,
		Namespace: namespace,
	}
}
