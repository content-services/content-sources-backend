package sync

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/content-services/content-sources-backend/pkg/clients/jira_client"
	"github.com/content-services/content-sources-backend/pkg/utils"
	cvss20 "github.com/pandatix/go-cvss/20"
	cvss30 "github.com/pandatix/go-cvss/30"
	cvss31 "github.com/pandatix/go-cvss/31"
	cvss40 "github.com/pandatix/go-cvss/40"
)

const (
	fieldPURL     = "customfield_10632"
	fieldCWE      = "customfield_10630"
	fieldSeverity = "customfield_10840"
	fieldCVSS     = "customfield_10859"
	fieldEmbargo  = "customfield_10860"
)

type Vulnerability struct {
	VulnerabilityKey   string
	VulnerabilityID    string
	PURL               *string
	ComponentName      string
	ComponentVersion   string
	Title              *string
	CWE                *string
	Description        *string
	Severity           string
	CVSS               *float64
	CVSSVector         *string
	ExploitTested      bool
	ReproducerIncluded bool
	CustomerPriority   *string
	Stage              string
	Language           *string
	Complexity         string
	SubmittedDate      time.Time
	LastUpdated        time.Time
	Embargo            bool
	Duplicate          bool
}

// TicketLink associates a vulnerability with a customer account from an epic.
// TicketID is the issue that epic is triggered by when a support ticket exists.
type TicketLink struct {
	TicketID   string
	CustomerID string
}

type adfNode struct {
	Type    string    `json:"type"`
	Text    string    `json:"text"`
	Content []adfNode `json:"content"`
}

type issueLink struct {
	Type struct {
		Inward  string `json:"inward"`
		Outward string `json:"outward"`
	} `json:"type"`
	InwardIssue  *linkedIssue `json:"inwardIssue"`
	OutwardIssue *linkedIssue `json:"outwardIssue"`
}

type linkedIssue struct {
	Key string `json:"key"`
}

func mapVulnerability(issue jira_client.JiraIssue) (Vulnerability, error) {
	if issue.Key == "" {
		return Vulnerability{}, errors.New("issue has no key")
	}
	summary := rawString(issue.Fields["summary"])
	vulnerabilityID := vulnerabilityIDFromSummary(summary)
	if vulnerabilityID == "" {
		return Vulnerability{}, fmt.Errorf("issue %s has no LW- or CVE- vulnerability id in summary", issue.Key)
	}

	created, err := parseJiraTime(rawString(issue.Fields["created"]))
	if err != nil {
		return Vulnerability{}, fmt.Errorf("issue %s created timestamp: %w", issue.Key, err)
	}
	updated, err := parseJiraTime(rawString(issue.Fields["updated"]))
	if err != nil {
		return Vulnerability{}, fmt.Errorf("issue %s updated timestamp: %w", issue.Key, err)
	}

	description := flattenADF(issue.Fields["description"])
	customerPriority := descriptionValue(description, "customer priority")
	cvss, vector := parseCVSS(issue.Fields[fieldCVSS])
	purl := packageURL(issue.Fields[fieldPURL], description)
	var parsedPURL *utils.PURL
	if purl != nil {
		parsedPURL = utils.ParsePURL(*purl)
	}

	return Vulnerability{
		VulnerabilityKey: issue.Key,
		VulnerabilityID:  vulnerabilityID,
		PURL:             purl,
		ComponentName:    componentName(descriptionValue(description, "component"), parsedPURL),
		ComponentVersion: componentVersion(descriptionValue(description, "version"), parsedPURL),
		Title:            optionalString(title(description, summary)),
		CWE:              joinedValues(issue.Fields[fieldCWE]),
		Description:      optionalString(description),
		Severity:         severity(issue.Fields[fieldSeverity], issue.Fields["labels"]),
		CVSS:             cvss,
		CVSSVector:       vector,
		CustomerPriority: optionalString(customerPriority),
		Stage:            stage(issue.Fields["status"]),
		Language:         language(issue.Fields["labels"], parsedPURL),
		Complexity:       "",
		SubmittedDate:    created.UTC(),
		LastUpdated:      updated.UTC(),
		Embargo:          rawBool(issue.Fields[fieldEmbargo]),
	}, nil
}

func parseJiraTime(value string) (time.Time, error) {
	if value == "" {
		return time.Time{}, errors.New("missing timestamp")
	}
	for _, layout := range []string{time.RFC3339Nano, "2006-01-02T15:04:05.000-0700", "2006-01-02T15:04:05-0700"} {
		if parsed, err := time.Parse(layout, value); err == nil {
			return parsed, nil
		}
	}
	return time.Time{}, fmt.Errorf("unsupported timestamp %q", value)
}

func flattenADF(raw json.RawMessage) string {
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		return ""
	}
	var plain string
	if json.Unmarshal(raw, &plain) == nil {
		return strings.TrimSpace(plain)
	}

	var root adfNode
	if json.Unmarshal(raw, &root) != nil {
		return ""
	}
	lines := make([]string, 0, len(root.Content))
	for _, child := range root.Content {
		var line strings.Builder
		renderADF(child, &line)
		if value := strings.TrimSpace(line.String()); value != "" {
			lines = append(lines, value)
		}
	}
	return strings.Join(lines, "\n")
}

func renderADF(node adfNode, target *strings.Builder) {
	switch node.Type {
	case "text":
		target.WriteString(node.Text)
	case "hardBreak":
		target.WriteByte('\n')
	}
	for index, child := range node.Content {
		if index > 0 && (node.Type == "bulletList" || node.Type == "orderedList") {
			target.WriteByte('\n')
		}
		renderADF(child, target)
	}
}

func vulnerabilityIDFromSummary(summary string) string {
	id, _, _ := strings.Cut(strings.TrimSpace(summary), " ")
	prefix := strings.ToUpper(id)
	if strings.HasPrefix(prefix, "CVE-") || strings.HasPrefix(prefix, "LW-") {
		return id
	}
	return ""
}

func title(description, summary string) string {
	if value := descriptionValue(description, "title"); value != "" {
		return value
	}
	_, rest, found := strings.Cut(strings.TrimSpace(summary), " ")
	if !found {
		return ""
	}
	return strings.TrimSpace(rest)
}

func descriptionValue(description, prefix string) string {
	for _, line := range strings.Split(description, "\n") {
		key, value, found := strings.Cut(line, ":")
		if found && strings.EqualFold(strings.TrimSpace(key), prefix) {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func rawString(raw json.RawMessage) string {
	var value string
	if json.Unmarshal(raw, &value) != nil {
		return ""
	}
	return strings.TrimSpace(value)
}

func rawBool(raw json.RawMessage) bool {
	var value bool
	return json.Unmarshal(raw, &value) == nil && value
}

func optionalString(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

func joinedValues(raw json.RawMessage) *string {
	values := normalizedValues(raw)
	if len(values) == 0 {
		return nil
	}
	value := strings.Join(values, ",")
	return &value
}

func normalizedValues(raw json.RawMessage) []string {
	var value any
	if len(raw) == 0 || json.Unmarshal(raw, &value) != nil {
		return nil
	}
	values := make([]string, 0)
	seen := make(map[string]struct{})
	var collect func(any)
	collect = func(item any) {
		switch typed := item.(type) {
		case string:
			trimmed := strings.TrimSpace(typed)
			if trimmed != "" {
				if _, exists := seen[trimmed]; !exists {
					seen[trimmed] = struct{}{}
					values = append(values, trimmed)
				}
			}
		case []any:
			for _, child := range typed {
				collect(child)
			}
		case map[string]any:
			collect(typed["value"])
		}
	}
	collect(value)
	return values
}

const severityLabelPrefix = "severity::"

func severity(field, labels json.RawMessage) string {
	if value := severityFromField(field); value != "" {
		return value
	}
	return severityFromLabels(labels)
}

func severityFromField(raw json.RawMessage) string {
	var option struct {
		ID string `json:"id"`
	}
	if json.Unmarshal(raw, &option) != nil || option.ID == "" {
		option.ID = rawString(raw)
	}
	return map[string]string{
		"19917": "Critical",
		"19918": "Important",
		"19919": "Moderate",
		"19920": "Low",
	}[option.ID]
}

func severityFromLabels(raw json.RawMessage) string {
	var labels []string
	if json.Unmarshal(raw, &labels) != nil {
		return ""
	}
	for _, label := range labels {
		trimmed := strings.TrimSpace(label)
		if len(trimmed) < len(severityLabelPrefix) {
			continue
		}
		if !strings.EqualFold(trimmed[:len(severityLabelPrefix)], severityLabelPrefix) {
			continue
		}
		if mapped := mapSeverityName(trimmed[len(severityLabelPrefix):]); mapped != "" {
			return mapped
		}
	}
	return ""
}

func mapSeverityName(value string) string {
	return map[string]string{
		"critical":  "Critical",
		"important": "Important",
		"high":      "Important",
		"moderate":  "Moderate",
		"medium":    "Moderate",
		"low":       "Low",
	}[strings.ToLower(strings.TrimSpace(value))]
}

var discardedResolutions = map[string]struct{}{
	"not a bug": {},
	"duplicate": {},
	"won't do":  {},
}

func discardedResolution(raw json.RawMessage) bool {
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		return false
	}
	var resolution struct {
		Name string `json:"name"`
	}
	if json.Unmarshal(raw, &resolution) != nil {
		return false
	}
	_, ok := discardedResolutions[strings.ToLower(strings.TrimSpace(resolution.Name))]
	return ok
}

func stage(raw json.RawMessage) string {
	var status struct {
		Name string `json:"name"`
	}
	if json.Unmarshal(raw, &status) != nil {
		return ""
	}

	statusMap := map[string]string{
		"new":             "Submitted",
		"backlog":         "Classified",
		"to do":           "Classified",
		"in progress":     "Fix in Progress",
		"review":          "Fix in Progress",
		"verified":        "Validation",
		"release pending": "Validation",
		"released":        "Validation",
		"closed":          "Validation",
	}

	if s, ok := statusMap[strings.ToLower(strings.TrimSpace(status.Name))]; ok {
		return s
	} else {
		return "Submitted"
	}
}

func packageURL(raw json.RawMessage, description string) *string {
	if value := rawString(raw); value != "" {
		return &value
	}
	return optionalString(descriptionValue(description, "purl"))
}

func language(raw json.RawMessage, purl *utils.PURL) *string {
	var labels []string
	if json.Unmarshal(raw, &labels) == nil {
		for _, label := range labels {
			value := strings.ToLower(strings.TrimSpace(label))
			if value == "java" || value == "python" || value == "javascript" {
				return &value
			}
		}
	}

	if purl == nil {
		return nil
	}
	return purl.Language()
}

func componentName(value string, purl *utils.PURL) string {
	if purl != nil && purl.FullName() != "" {
		return purl.FullName()
	}
	return value
}

func componentVersion(value string, purl *utils.PURL) string {
	if purl != nil && purl.Version != "" {
		return purl.Version
	}
	return value
}

func parseCVSS(raw json.RawMessage) (*float64, *string) {
	if len(raw) == 0 {
		return nil, nil
	}
	var number float64
	if json.Unmarshal(raw, &number) == nil {
		return &number, nil
	}
	value := rawString(raw)
	if value == "" {
		return nil, nil
	}
	if parsed, err := strconv.ParseFloat(value, 64); err == nil {
		return &parsed, nil
	}

	var score float64
	var err error
	switch {
	case strings.HasPrefix(value, "CVSS:4.0/"):
		var parsed *cvss40.CVSS40
		parsed, err = cvss40.ParseVector(value)
		if err == nil {
			score = parsed.Score()
		}
	case strings.HasPrefix(value, "CVSS:3.1/"):
		var parsed *cvss31.CVSS31
		parsed, err = cvss31.ParseVector(value)
		if err == nil {
			score = parsed.BaseScore()
		}
	case strings.HasPrefix(value, "CVSS:3.0/"):
		var parsed *cvss30.CVSS30
		parsed, err = cvss30.ParseVector(value)
		if err == nil {
			score = parsed.BaseScore()
		}
	default:
		vector := strings.TrimPrefix(value, "CVSS:2.0/")
		var parsed *cvss20.CVSS20
		parsed, err = cvss20.ParseVector(vector)
		if err == nil {
			score = parsed.BaseScore()
		}
	}
	if err != nil {
		return nil, nil
	}
	return &score, &value
}

func linkedIssueKey(issue jira_client.JiraIssue, label string) string {
	keys := linkedIssueKeys(issue, label)
	if len(keys) == 0 {
		return ""
	}
	return keys[0]
}

// linkedIssueKeys returns the keys of every issue linked to the given issue in
// the named direction. A vulnerability can relate to multiple batch tickets, so
// callers that need all of them use this rather than linkedIssueKey.
func linkedIssueKeys(issue jira_client.JiraIssue, label string) []string {
	var links []issueLink
	if json.Unmarshal(issue.Fields["issuelinks"], &links) != nil {
		return nil
	}
	keys := make([]string, 0, len(links))
	for _, link := range links {
		if link.InwardIssue != nil && link.Type.Inward == label {
			keys = append(keys, link.InwardIssue.Key)
		}
		if link.OutwardIssue != nil && link.Type.Outward == label {
			keys = append(keys, link.OutwardIssue.Key)
		}
	}
	return keys
}

func parentIssueKey(issue jira_client.JiraIssue) string {
	var parent linkedIssue
	if json.Unmarshal(issue.Fields["parent"], &parent) != nil {
		return ""
	}
	return parent.Key
}
