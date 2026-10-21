package sync

import (
	"slices"
	"strings"

	"github.com/content-services/content-sources-backend/pkg/clients/jira_client"
)

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

// ctMemberKeys returns the set of IVT keys incorporated by any of the given
// (Closed/Done) Component Upgrades. An IVT is "shipped" only if it is a member of
// one of these CTs.
func ctMemberKeys(cts []jira_client.JiraIssue) map[string]struct{} {
	members := make(map[string]struct{})
	for _, ct := range cts {
		for _, k := range linkedIssueKeys(ct, "incorporates") {
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
		// linkedIssueKeys matches the link's inward/outward verb, not its display
		// name: on a CT the member IVT is the outwardIssue of the "Incorporates"
		// link type, whose outward verb is the lowercase "incorporates".
		members := linkedIssueKeys(ct, "incorporates")
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
