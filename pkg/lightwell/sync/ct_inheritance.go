package sync

import (
	"encoding/json"
	"slices"
	"strings"

	"github.com/content-services/content-sources-backend/pkg/clients/jira_client"
)

// ctIncorporateLink is the shape of a Jira issue link needed to find a Component
// Upgrade's incorporated members. The "Incorporates" link type appears in the data
// in BOTH directions (member on the outward side for Novel CTs, on the inward side
// for some Backport CTs), so membership is matched by the link TYPE NAME and the
// member taken from whichever side is present — mirroring the remediation-dashboard.
type ctIncorporateLink struct {
	Type struct {
		Name string `json:"name"`
	} `json:"type"`
	InwardIssue *struct {
		Key string `json:"key"`
	} `json:"inwardIssue"`
	OutwardIssue *struct {
		Key string `json:"key"`
	} `json:"outwardIssue"`
}

// ctIncorporatedKeys returns the IVT keys a Component Upgrade incorporates,
// direction-agnostically: every "Incorporates"-typed link's non-CT side.
func ctIncorporatedKeys(ct jira_client.JiraIssue) []string {
	var links []ctIncorporateLink
	if json.Unmarshal(ct.Fields["issuelinks"], &links) != nil {
		return nil
	}
	keys := make([]string, 0, len(links))
	for _, l := range links {
		if l.Type.Name != "Incorporates" {
			continue
		}
		if l.OutwardIssue != nil {
			keys = append(keys, l.OutwardIssue.Key)
		}
		if l.InwardIssue != nil {
			keys = append(keys, l.InwardIssue.Key)
		}
	}
	return keys
}

// shipReadyStatuses are the Jira statuses (lowercased) that count as
// "ship-ready" (Verified or later). Only ship-ready IVTs contribute their CID
// labels when a Component Upgrade's CIDs are unioned across its members, matching
// the remediation-dashboard's inheritance population.
var shipReadyStatuses = map[string]struct{}{
	"verified":        {},
	"release pending": {},
	"released":        {},
	"closed":          {},
}

// isShipReady reports whether a Jira status name is Verified-or-later.
func isShipReady(status string) bool {
	_, ok := shipReadyStatuses[strings.ToLower(strings.TrimSpace(status))]
	return ok
}

// ctFinished reports whether a Component Upgrade has reached a terminal state.
// Closed, Done, and Resolved all count. An open CT does not.
func ctFinished(issue jira_client.JiraIssue) bool {
	switch strings.ToLower(statusName(issue.Fields["status"])) {
	case "closed", "done", "resolved":
		return true
	default:
		return false
	}
}

// finishedComponentUpgrades returns CTs in a terminal status. CID inheritance
// uses only these, so an open Component Upgrade does not grant customer credit.
func finishedComponentUpgrades(cts []jira_client.JiraIssue) []jira_client.JiraIssue {
	finished := make([]jira_client.JiraIssue, 0, len(cts))
	for _, ct := range cts {
		if ctFinished(ct) {
			finished = append(finished, ct)
		}
	}
	return finished
}

// ivtsReadyToShip returns IVT keys whose every linked Component Upgrade is
// finished. An IVT with no CT, or with any CT still open, is absent.
func ivtsReadyToShip(cts []jira_client.JiraIssue) map[string]struct{} {
	type counts struct {
		total    int
		finished int
	}
	byIVT := make(map[string]*counts)
	for _, ct := range cts {
		done := ctFinished(ct)
		for _, key := range ctIncorporatedKeys(ct) {
			if key == "" {
				continue
			}
			c := byIVT[key]
			if c == nil {
				c = &counts{}
				byIVT[key] = c
			}
			c.total++
			if done {
				c.finished++
			}
		}
	}
	ready := make(map[string]struct{})
	for key, c := range byIVT {
		if c.total > 0 && c.finished == c.total {
			ready[key] = struct{}{}
		}
	}
	return ready
}

// ctMemberKeys returns the set of IVT keys incorporated by any of the given
// Component Upgrades.
func ctMemberKeys(cts []jira_client.JiraIssue) map[string]struct{} {
	members := make(map[string]struct{})
	for _, ct := range cts {
		for _, k := range ctIncorporatedKeys(ct) {
			members[k] = struct{}{}
		}
	}
	return members
}

// mergeInheritedCustomers appends a ticket-less customer link for each inherited
// CID not already present among the IVT's existing customer links, de-duplicating
// by customer id so an IVT never gains a duplicate customer row.
func mergeInheritedCustomers(tickets []TicketLink, cids []string) []TicketLink {
	existing := make(map[string]struct{}, len(tickets))
	for _, t := range tickets {
		existing[t.CustomerID] = struct{}{}
	}
	for _, cid := range cids {
		if _, ok := existing[cid]; ok {
			continue
		}
		existing[cid] = struct{}{}
		tickets = append(tickets, TicketLink{CustomerID: cid})
	}
	return tickets
}

// buildInheritedCIDs maps each ship-ready Component Upgrade member IVT key to the
// sorted, de-duplicated union of CID labels carried by the ship-ready members of
// every CT it belongs to. CID labels and member status are read from the fetched
// IVT set (`issues`); members absent from that set, or not ship-ready, are skipped.
func buildInheritedCIDs(issues []jira_client.JiraIssue, cts []jira_client.JiraIssue) map[string][]string {
	type ivtInfo struct {
		cids      []string
		shipReady bool
	}
	info := make(map[string]ivtInfo, len(issues))
	for _, issue := range issues {
		info[issue.Key] = ivtInfo{
			cids:      customerIDsFromLabels(issue.Fields["labels"]),
			shipReady: isShipReady(statusName(issue.Fields["status"])),
		}
	}

	out := make(map[string][]string)
	for _, ct := range cts {
		members := ctIncorporatedKeys(ct)
		shipReadyMembers := make([]string, 0, len(members))
		union := make([]string, 0)
		for _, m := range members {
			meta, ok := info[m]
			if !ok || !meta.shipReady {
				continue
			}
			shipReadyMembers = append(shipReadyMembers, m)
			union = append(union, meta.cids...)
		}
		if len(union) == 0 {
			continue
		}
		for _, m := range shipReadyMembers {
			out[m] = append(out[m], union...)
		}
	}

	for key, cids := range out {
		slices.Sort(cids)
		out[key] = slices.Compact(cids)
	}
	return out
}
