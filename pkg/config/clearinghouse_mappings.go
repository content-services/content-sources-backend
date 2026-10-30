package config

import (
	"bytes"
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/rs/zerolog/log"
)

// ClearinghouseMapping is the in-memory form of the lightwell-stams mappings
// document. CID is derived from an org ID when a submission is returned and is
// not stored.
type ClearinghouseMapping struct {
	orgIDByCID map[string]string
	cidByOrgID map[string]string
	tamsByCID  map[string][]string
}

type clearinghouseMappingsDocument struct {
	CidOrgId json.RawMessage `json:"CidOrgId"`
	CidTam   json.RawMessage `json:"CidTam"`
}

// ParseClearinghouseMappings parses the mappings JSON. An empty document
// returns an empty mapping. Invalid JSON is logged and also returns an empty
// mapping so the rest of the service can still start. A CID that appears more
// than once in either section is kept (last value wins) and logged as an error.
func ParseClearinghouseMappings(raw string) ClearinghouseMapping {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ClearinghouseMapping{}
	}
	var doc clearinghouseMappingsDocument
	if err := json.Unmarshal([]byte(raw), &doc); err != nil {
		log.Error().Err(err).Msg("CLEARINGHOUSE MAPPING WARNING: mappings JSON could not be parsed. Clearinghouse CID mapping is empty.")
		return ClearinghouseMapping{}
	}

	orgIDByCID, err := parseCIDOrgIDs(doc.CidOrgId)
	if err != nil {
		log.Error().Err(err).Msg("CLEARINGHOUSE MAPPING WARNING: CidOrgId could not be parsed. Clearinghouse CID mapping is empty.")
		return ClearinghouseMapping{}
	}
	tamsByCID, err := parseCIDTAMs(doc.CidTam)
	if err != nil {
		log.Error().Err(err).Msg("CLEARINGHOUSE MAPPING WARNING: CidTam could not be parsed. Clearinghouse CID mapping is empty.")
		return ClearinghouseMapping{}
	}
	return ClearinghouseMapping{
		orgIDByCID: orgIDByCID,
		cidByOrgID: cidByOrgID(orgIDByCID),
		tamsByCID:  tamsByCID,
	}
}

func parseCIDOrgIDs(raw json.RawMessage) (map[string]string, error) {
	if len(bytes.TrimSpace(raw)) == 0 || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return map[string]string{}, nil
	}
	warnRepeatedCIDs("CidOrgId", raw)
	var parsed map[string]string
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil, err
	}
	if parsed == nil {
		parsed = map[string]string{}
	}
	return parsed, nil
}

func parseCIDTAMs(raw json.RawMessage) (map[string][]string, error) {
	if len(bytes.TrimSpace(raw)) == 0 || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return map[string][]string{}, nil
	}
	warnRepeatedCIDs("CidTam", raw)
	var parsed map[string][]string
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil, err
	}
	if parsed == nil {
		parsed = map[string][]string{}
	}
	return parsed, nil
}

func warnRepeatedCIDs(section string, raw json.RawMessage) {
	keys, err := jsonObjectKeys(raw)
	if err != nil {
		return
	}
	repeated := repeatedKeys(keys)
	if len(repeated) == 0 {
		return
	}
	log.Error().
		Str("section", section).
		Strs("cids", repeated).
		Msg("CLEARINGHOUSE MAPPING WARNING: CID is repeated. Each CID must appear only once. The last value for that CID is kept and earlier values are ignored.")
}

func cidByOrgID(orgIDByCID map[string]string) map[string]string {
	out := make(map[string]string, len(orgIDByCID))
	cids := make([]string, 0, len(orgIDByCID))
	for cid := range orgIDByCID {
		cids = append(cids, cid)
	}
	slices.Sort(cids)
	for _, cid := range cids {
		orgID := orgIDByCID[cid]
		if orgID == "" || cid == "" {
			continue
		}
		if existing, ok := out[orgID]; ok && existing != cid {
			log.Error().
				Str("org_id", orgID).
				Str("kept_cid", existing).
				Str("ignored_cid", cid).
				Msg("CLEARINGHOUSE MAPPING WARNING: org ID is assigned to more than one CID. The lexicographically first CID is kept.")
			continue
		}
		out[orgID] = cid
	}
	return out
}

// CIDForOrg returns the CID mapped to orgID, or an empty string when the org
// is not in the mapping.
func (m ClearinghouseMapping) CIDForOrg(orgID string) string {
	if m.cidByOrgID == nil {
		return ""
	}
	return m.cidByOrgID[orgID]
}

// Allows reports whether username is a TAM for the CID that owns orgID.
func (m ClearinghouseMapping) Allows(username, orgID string) bool {
	cid := m.CIDForOrg(orgID)
	if cid == "" || username == "" {
		return false
	}
	return slices.Contains(m.tamsByCID[cid], username)
}

// OrgIDsForUser returns the org IDs whose CID lists username, in sorted order.
func (m ClearinghouseMapping) OrgIDsForUser(username string) []string {
	if username == "" || len(m.tamsByCID) == 0 {
		return []string{}
	}
	orgs := make([]string, 0, 1)
	for cid, tams := range m.tamsByCID {
		if !slices.Contains(tams, username) {
			continue
		}
		orgID := m.orgIDByCID[cid]
		if orgID == "" || m.CIDForOrg(orgID) != cid {
			continue
		}
		orgs = append(orgs, orgID)
	}
	slices.Sort(orgs)
	return orgs
}

func jsonObjectKeys(raw []byte) ([]string, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	tok, err := dec.Token()
	if err != nil {
		return nil, err
	}
	delim, ok := tok.(json.Delim)
	if !ok || delim != '{' {
		return nil, fmt.Errorf("expected JSON object")
	}
	var keys []string
	for dec.More() {
		tok, err = dec.Token()
		if err != nil {
			return nil, err
		}
		key, ok := tok.(string)
		if !ok {
			return nil, fmt.Errorf("expected string key")
		}
		keys = append(keys, key)
		if err = skipJSONValue(dec); err != nil {
			return nil, err
		}
	}
	tok, err = dec.Token()
	if err != nil {
		return nil, err
	}
	delim, ok = tok.(json.Delim)
	if !ok || delim != '}' {
		return nil, fmt.Errorf("expected end of JSON object")
	}
	return keys, nil
}

func skipJSONValue(dec *json.Decoder) error {
	tok, err := dec.Token()
	if err != nil {
		return err
	}
	delim, ok := tok.(json.Delim)
	if !ok {
		return nil
	}
	switch delim {
	case '{', '[':
		for dec.More() {
			if delim == '{' {
				if _, err = dec.Token(); err != nil {
					return err
				}
			}
			if err = skipJSONValue(dec); err != nil {
				return err
			}
		}
		tok, err = dec.Token()
		if err != nil {
			return err
		}
		end, ok := tok.(json.Delim)
		if !ok || (delim == '{' && end != '}') || (delim == '[' && end != ']') {
			return fmt.Errorf("expected end of JSON value")
		}
		return nil
	default:
		return fmt.Errorf("unexpected JSON delimiter")
	}
}

func repeatedKeys(keys []string) []string {
	counts := make(map[string]int, len(keys))
	var repeated []string
	for _, key := range keys {
		counts[key]++
		if counts[key] == 2 {
			repeated = append(repeated, key)
		}
	}
	return repeated
}
