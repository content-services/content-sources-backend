package config

import (
	"bytes"
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/rs/zerolog/log"
)

// ClearinghouseMapping is the in-memory form of the lightwell-stams CID
// mappings. CID is derived from an org ID when a submission is returned and is
// not stored.
type ClearinghouseMapping struct {
	orgIDsByCID map[string][]string
	cidByOrgID  map[string]string
	usersByCID  map[string][]string
}

// ParseClearinghouseMappings parses the two mapping objects. Each value is the
// content of the vault key "mapping", for example {"CID-00000": ["20007461"]}.
// Vault-secret stringifies a nested object with single quotes, so those are
// replaced before parsing. An empty value is an empty side of the mapping.
// Invalid JSON is logged and that side is left empty so the rest of the service
// can still start. A CID that appears more than once is kept (last value wins)
// and logged as an error.
func ParseClearinghouseMappings(orgIDsRaw, usersRaw string) ClearinghouseMapping {
	orgIDsByCID, err := parseCIDStringLists("cid_org_id", orgIDsRaw)
	if err != nil {
		log.Error().Err(err).Msg("CLEARINGHOUSE MAPPING WARNING: cid_org_id could not be parsed. Org to CID mapping is empty.")
		orgIDsByCID = nil
	}
	usersByCID, err := parseCIDStringLists("cid_user_mapping", usersRaw)
	if err != nil {
		log.Error().Err(err).Msg("CLEARINGHOUSE MAPPING WARNING: cid_user_mapping could not be parsed. CID user mapping is empty.")
		usersByCID = nil
	}
	return ClearinghouseMapping{
		orgIDsByCID: orgIDsByCID,
		cidByOrgID:  cidByOrgID(orgIDsByCID),
		usersByCID:  usersByCID,
	}
}

func parseCIDStringLists(section, raw string) (map[string][]string, error) {
	raw = strings.ReplaceAll(strings.TrimSpace(raw), "'", "\"")
	if raw == "" {
		return nil, nil
	}
	warnRepeatedCIDs(section, json.RawMessage(raw))
	var parsed map[string][]string
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		return nil, err
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

func cidByOrgID(orgIDsByCID map[string][]string) map[string]string {
	if len(orgIDsByCID) == 0 {
		return nil
	}
	out := make(map[string]string, len(orgIDsByCID))
	cids := make([]string, 0, len(orgIDsByCID))
	for cid := range orgIDsByCID {
		cids = append(cids, cid)
	}
	slices.Sort(cids)
	for _, cid := range cids {
		if cid == "" {
			continue
		}
		for _, orgID := range orgIDsByCID[cid] {
			if orgID == "" {
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
	return slices.Contains(m.usersByCID[cid], username)
}

// OrgIDsForUser returns the org IDs whose CID lists username, in sorted order.
func (m ClearinghouseMapping) OrgIDsForUser(username string) []string {
	if username == "" || len(m.usersByCID) == 0 {
		return []string{}
	}
	seen := make(map[string]struct{})
	orgs := make([]string, 0, 1)
	for cid, users := range m.usersByCID {
		if !slices.Contains(users, username) {
			continue
		}
		for _, orgID := range m.orgIDsByCID[cid] {
			if orgID == "" || m.CIDForOrg(orgID) != cid {
				continue
			}
			if _, ok := seen[orgID]; ok {
				continue
			}
			seen[orgID] = struct{}{}
			orgs = append(orgs, orgID)
		}
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
