package parser

import (
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"strings"
)

// parseCSV streams rows and extracts packages from a packageurl column (header may be preceded by metadata rows).
func parseCSV(r io.Reader) ([]Package, error) {
	reader := csv.NewReader(r)
	reader.FieldsPerRecord = -1
	reader.LazyQuotes = true

	purlCol := -1
	var packages []Package

	for {
		record, err := reader.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("CSV read error: %w", err)
		}

		if purlCol < 0 {
			for i, field := range record {
				if strings.TrimSpace(strings.ToLower(field)) == "packageurl" {
					purlCol = i
					break
				}
			}
			continue
		}

		if purlCol >= len(record) {
			continue
		}
		purl := strings.TrimSpace(record[purlCol])
		if purl == "" {
			continue
		}
		if pkg := parsePURL(purl); pkg != nil {
			packages = append(packages, *pkg)
		}
	}

	if purlCol < 0 {
		return nil, fmt.Errorf("CSV missing required 'packageurl' header column")
	}

	return packages, nil
}
