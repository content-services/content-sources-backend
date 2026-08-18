package parser

import (
	"bytes"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"strings"
)

func parseCSV(data []byte) ([]Package, error) {
	reader := csv.NewReader(bytes.NewReader(data))
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
