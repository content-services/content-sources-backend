package parser

import (
	"bufio"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"strings"
)

type cdxComponent struct {
	Group      string         `xml:"group"`
	Name       string         `xml:"name"`
	Version    string         `xml:"version"`
	PURL       string         `xml:"purl"`
	Components []cdxComponent `xml:"components>component"`
}

// parseCycloneDX dispatches JSON or XML from the first non-space byte.
func parseCycloneDX(r *bufio.Reader, skipped *int) ([]Package, error) {
	start, err := peekSBOMStart(r)
	if err != nil {
		return nil, fmt.Errorf("reading CycloneDX SBOM: %w", err)
	}
	switch start {
	case '{', '[':
		return parseCycloneDXJSON(r, skipped)
	case '<':
		return parseCycloneDXXML(r, skipped)
	default:
		return nil, fmt.Errorf("unsupported CycloneDX encoding (supported: JSON, XML)")
	}
}

// parseCycloneDXJSON streams metadata.component and components[] without loading the rest of the BOM.
func parseCycloneDXJSON(r io.Reader, skipped *int) ([]Package, error) {
	dec := json.NewDecoder(r)
	var pkgs []Package

	err := readJSONObject(dec, map[string]jsonFieldHandler{
		"metadata": func(d *json.Decoder) error {
			return readJSONObject(d, map[string]jsonFieldHandler{
				"component": func(d *json.Decoder) error {
					extracted, err := parseCDXJSONComponent(d, skipped)
					pkgs = append(pkgs, extracted...)
					return err
				},
			})
		},
		"components": func(d *json.Decoder) error {
			return appendParsedJSONArray(d, &pkgs, func(d *json.Decoder) ([]Package, error) { return parseCDXJSONComponent(d, skipped) })
		},
	})
	return pkgs, wrapParse("CycloneDX JSON", err)
}

// parseCDXJSONComponent streams one component and any nested components[] under it.
func parseCDXJSONComponent(dec *json.Decoder, skipped *int) ([]Package, error) {
	var purl, group, name, version string
	var pkgs []Package

	err := readJSONObject(dec, map[string]jsonFieldHandler{
		"purl":    func(d *json.Decoder) error { return decodeJSONString(d, &purl) },
		"group":   func(d *json.Decoder) error { return decodeJSONString(d, &group) },
		"name":    func(d *json.Decoder) error { return decodeJSONString(d, &name) },
		"version": func(d *json.Decoder) error { return decodeJSONString(d, &version) },
		"components": func(d *json.Decoder) error {
			return appendParsedJSONArray(d, &pkgs, func(d *json.Decoder) ([]Package, error) { return parseCDXJSONComponent(d, skipped) })
		},
	})
	if err != nil {
		return nil, err
	}
	before := len(pkgs)
	pkgs = appendFromCDXIdentity(pkgs, purl, group, name, version)
	if len(pkgs) == before {
		*skipped++
	}
	return pkgs, nil
}

// parseCycloneDXXML walks component elements and skips tools/vulnerabilities so they are not treated as inventory.
func parseCycloneDXXML(r io.Reader, skipped *int) ([]Package, error) {
	var pkgs []Package
	err := forEachXMLStart(r, func(dec *xml.Decoder, se xml.StartElement) error {
		switch se.Name.Local {
		case "tools", "vulnerabilities", "dependencies", "services", "compositions",
			"formulation", "annotations", "declarations", "externalReferences":
			return dec.Skip()
		case "component":
			extracted, err := decodeCDXXMLComponent(dec, se, skipped)
			pkgs = append(pkgs, extracted...)
			return err
		}
		return nil
	})
	return pkgs, wrapParse("CycloneDX XML", err)
}

// decodeCDXXMLComponent reads one <component> subtree, including nested children.
func decodeCDXXMLComponent(dec *xml.Decoder, start xml.StartElement, skipped *int) ([]Package, error) {
	var c cdxComponent
	if err := dec.DecodeElement(&c, &start); err != nil {
		return nil, err
	}
	return flattenCDXComponent(c, skipped), nil
}

// flattenCDXComponent turns a component and its nested children into packages.
func flattenCDXComponent(c cdxComponent, skipped *int) []Package {
	pkgs := appendFromCDXIdentity(nil, c.PURL, c.Group, c.Name, c.Version)
	if len(pkgs) == 0 {
		*skipped++
	}
	for _, child := range c.Components {
		pkgs = append(pkgs, flattenCDXComponent(child, skipped)...)
	}
	return pkgs
}

// appendFromCDXIdentity prefers a purl, then treats CycloneDX group+name as Maven when no purl is present.
func appendFromCDXIdentity(pkgs []Package, purl, group, name, version string) []Package {
	pkgs = appendFromPURL(pkgs, purl)
	if parsePURL(strings.TrimSpace(purl)) != nil || group == "" || name == "" {
		return pkgs
	}
	constructed := "pkg:maven/" + group + "/" + name
	if version != "" {
		constructed += "@" + version
	}
	return appendFromPURL(pkgs, constructed)
}
