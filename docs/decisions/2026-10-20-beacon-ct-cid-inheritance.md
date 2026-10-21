# Beacon CT-based CID Inheritance Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make beacon credit a customer for all done IVTs that shipped in a Closed/Done Component Upgrade when any ship-ready IVT in that CT carries the customer's `CID-<n>` label.

**Architecture:** Add a second Jira fetch pass in `Ingestor.Sync` for Closed/Done Component Upgrade issues, derive a `key → inherited CIDs` map from their "Incorporates" links (unioning CID labels across ship-ready members only), then during per-issue sync merge those inherited CIDs into the customer set **only when the IVT's stage is `Lightwell Network`**. Reuses the existing per-vuln customer reconcile — no schema change.

**Tech Stack:** Go; `pkg/lightwell/sync`; existing `jira_client.JiraClient` interface; sqlc DAO.

**Spec:** `docs/superpowers/specs/2026-10-20-beacon-ct-cid-inheritance-design.md`

## Global Constraints

- CT population JQL (verbatim): `project = LTWL AND issuetype = "Component Upgrade" AND component in ("Backport", "Novel Fix") AND status = Closed AND resolution = Done ORDER BY created ASC`
- CT→IVT membership = the CT's `issuelinks` with link type whose matchable outward verb is the lowercase `"incorporates"` (display name `"Incorporates"`) (member IVT on the outward side); reuse existing `linkedIssueKeys(ct, "incorporates")`.
- Ship-ready statuses (lowercased): `verified`, `release pending`, `released`, `closed`. Only ship-ready members contribute a CID label to a CT's union.
- Member CID labels and member status are read from the **already-fetched IVT set**, not from inline link status. Members not in the fetched IVT set are skipped.
- CID labels parsed with the existing `customerIDsFromLabels` (bare number, e.g. `CID-663171` → `663171`).
- Inherited credit affects **only** the `Lightwell Network` bucket: merge happens after stage finalization, gated on `Stage == "Lightwell Network"`. Inherited customers carry `TicketID = ""`.
- Additive only: existing direct-label associations are unchanged.

## Review Focus

- Nil/empty/malformed CT `issuelinks` JSON → no panic, CT contributes nothing. (Task 1 test.)
- CT member key referencing an IVT absent from the fetched set → that member is skipped, not treated as ship-ready. (Task 1 test.)
- A CID present on both the IVT's own labels and inherited → deduped, single customer row. (Task 2 test.)
- A CID on a non-ship-ready member (e.g. status `New`) → does NOT propagate. (Task 1 test.)
- An inheriting IVT whose stage is not `Lightwell Network` (e.g. `Verified` → `Validation`) → receives NO inherited credit. (Task 2 test.)

---

### Task 1: Pure CID-inheritance computation

**Files:**
- Create: `pkg/lightwell/sync/ct_inheritance.go`
- Test: `pkg/lightwell/sync/ct_inheritance_test.go`

**Interfaces:**
- Consumes: existing `customerIDsFromLabels(json.RawMessage) []string`, `statusName(json.RawMessage) string`, `linkedIssueKeys(jira_client.JiraIssue, string) []string` (all in `mapper.go`).
- Produces:
  - `func isShipReady(status string) bool`
  - `func buildInheritedCIDs(issues []jira_client.JiraIssue, cts []jira_client.JiraIssue) map[string][]string` — maps each ship-ready CT member key to the sorted, de-duplicated union of CID labels carried by the ship-ready members of every CT it belongs to.

- [ ] **Step 1: Write the failing tests**

```go
func TestIsShipReady(t *testing.T) {
    for _, s := range []string{"Verified", "Release Pending", "Released", "Closed", "verified"} {
        assert.True(t, isShipReady(s), s)
    }
    for _, s := range []string{"New", "To Do", "In Progress", "Review", "On Hold", ""} {
        assert.False(t, isShipReady(s), s)
    }
}

func TestBuildInheritedCIDsUnionsShipReadyMembers(t *testing.T) {
    // CT incorporates a labelled Released IVT and an unlabelled Released IVT.
    labelled := ivtWith("LTWL-1", "Released", `["CID-663171"]`)
    unlabelled := ivtWith("LTWL-2", "Released", `[]`)
    ct := ctIncorporating("CT-1", "LTWL-1", "LTWL-2")
    got := buildInheritedCIDs([]jira_client.JiraIssue{labelled, unlabelled}, []jira_client.JiraIssue{ct})
    assert.Equal(t, []string{"663171"}, got["LTWL-1"])
    assert.Equal(t, []string{"663171"}, got["LTWL-2"])
}

func TestBuildInheritedCIDsIgnoresNonShipReadySource(t *testing.T) {
    // The only CID label sits on a non-ship-ready (New) member → no propagation.
    newLabelled := ivtWith("LTWL-1", "New", `["CID-663171"]`)
    doneUnlabelled := ivtWith("LTWL-2", "Released", `[]`)
    ct := ctIncorporating("CT-1", "LTWL-1", "LTWL-2")
    got := buildInheritedCIDs([]jira_client.JiraIssue{newLabelled, doneUnlabelled}, []jira_client.JiraIssue{ct})
    assert.Empty(t, got["LTWL-2"])
}

func TestBuildInheritedCIDsSkipsUnknownAndMalformed(t *testing.T) {
    labelled := ivtWith("LTWL-1", "Released", `["CID-663171"]`)
    ctUnknownMember := ctIncorporating("CT-1", "LTWL-1", "LTWL-404") // LTWL-404 not fetched
    ctBadLinks := jira_client.JiraIssue{Key: "CT-2", Fields: map[string]json.RawMessage{
        "issuelinks": json.RawMessage(`not-json`)}}
    got := buildInheritedCIDs([]jira_client.JiraIssue{labelled},
        []jira_client.JiraIssue{ctUnknownMember, ctBadLinks})
    assert.NotContains(t, got, "LTWL-404")
    // no panic; LTWL-1 only has its own CID in the union (self-member)
    assert.Equal(t, []string{"663171"}, got["LTWL-1"])
}
```

Add test helpers in the test file: `ivtWith(key, status, labelsJSON string) jira_client.JiraIssue` (sets `status` and `labels` raw fields) and `ctIncorporating(key string, memberKeys ...string) jira_client.JiraIssue` (builds an `issuelinks` array of `{"type":{"outward":"Incorporates"},"outwardIssue":{"key":<m>}}`).

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./pkg/lightwell/sync/ -run 'ShipReady|InheritedCIDs' -v`
Expected: FAIL (undefined `isShipReady`, `buildInheritedCIDs`).

- [ ] **Step 3: Implement `isShipReady` and `buildInheritedCIDs` in `ct_inheritance.go`**

`isShipReady`: lowercase/trim, membership test against the four ship-ready statuses. `buildInheritedCIDs`: build `map[key]struct{cids []string; shipReady bool}` from `issues`; for each CT, take `linkedIssueKeys(ct, "incorporates")`, keep members present in the map and ship-ready, union their cids, then append the union to each such member's entry in the output; finally sort+compact each output slice.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./pkg/lightwell/sync/ -run 'ShipReady|InheritedCIDs' -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add pkg/lightwell/sync/ct_inheritance.go pkg/lightwell/sync/ct_inheritance_test.go
git commit -m "feat(beacon): compute CT-based CID inheritance map"
```

---

### Task 2: Fetch Closed/Done CTs and apply done-gated inheritance in Sync

**Files:**
- Modify: `pkg/lightwell/sync/ingestion.go` (add CT fetch + JQL/fields const, generalize paginator, wire into `Sync`, merge in `syncIssue`)
- Modify: `pkg/lightwell/sync/ingestion_test.go` (extend `fakeJira` to serve CT pages; add tests)

**Interfaces:**
- Consumes: `buildInheritedCIDs`, `isShipReady` (Task 1); existing `searchAll` pagination pattern; `jira_client.JiraClient.Search`.
- Produces (unexported, same package):
  - `const ComponentUpgradeJQL = "..."` (verbatim from Global Constraints)
  - `var componentUpgradeFields = []string{"issuelinks"}`
  - `func (i *Ingestor) searchCTs(ctx context.Context) ([]jira_client.JiraIssue, error)`
  - `func mergeInheritedCustomers(tickets []TicketLink, cids []string) []TicketLink`
  - `syncIssue` gains a `inheritedCIDs []string` parameter.

- [ ] **Step 1: Write the failing tests**

```go
func TestIngestorInheritsCIDForDoneSibling(t *testing.T) {
    // LTWL-1 labelled+Released; LTWL-2 Released, no label; same Closed/Done CT.
    v1 := validJiraIssue("LTWL-1"); v1.Fields["labels"] = json.RawMessage(`["CID-663171"]`)
    v1.Fields["status"] = json.RawMessage(`{"name":"Released"}`)
    v2 := validJiraIssue("LTWL-2"); v2.Fields["labels"] = json.RawMessage(`[]`)
    v2.Fields["status"] = json.RawMessage(`{"name":"Released"}`)
    ct := ctIncorporating("CT-1", "LTWL-1", "LTWL-2")
    jira := &fakeJira{
        pages:   map[string]jira_client.JiraPage{"": {Issues: []jira_client.JiraIssue{v1, v2}}},
        ctPages: map[string]jira_client.JiraPage{"": {Issues: []jira_client.JiraIssue{ct}}},
    }
    store := &fakeVulnerabilityStore{outcome: dao.LightwellVulnerabilityInserted}
    _, err := NewIngestor(jira, store, nil, nil).Sync(context.Background())
    require.NoError(t, err)
    // LTWL-2 (done, unlabelled) now carries the inherited customer.
    saved2 := savedByKey(store, "LTWL-2")
    assert.Equal(t, []dao.LightwellVulnerabilityTicket{{CustomerID: "663171"}}, saved2.Tickets)
    assert.Contains(t, jira.searchJQLs, ComponentUpgradeJQL)
}

func TestIngestorDoesNotInheritForNonDoneSibling(t *testing.T) {
    // LTWL-2 is Verified (ship-ready but stage Validation, not Lightwell Network).
    v1 := validJiraIssue("LTWL-1"); v1.Fields["labels"] = json.RawMessage(`["CID-663171"]`)
    v1.Fields["status"] = json.RawMessage(`{"name":"Released"}`)
    v2 := validJiraIssue("LTWL-2"); v2.Fields["labels"] = json.RawMessage(`[]`)
    v2.Fields["status"] = json.RawMessage(`{"name":"Verified"}`)
    ct := ctIncorporating("CT-1", "LTWL-1", "LTWL-2")
    jira := &fakeJira{
        pages:   map[string]jira_client.JiraPage{"": {Issues: []jira_client.JiraIssue{v1, v2}}},
        ctPages: map[string]jira_client.JiraPage{"": {Issues: []jira_client.JiraIssue{ct}}},
    }
    store := &fakeVulnerabilityStore{outcome: dao.LightwellVulnerabilityInserted}
    _, err := NewIngestor(jira, store, nil, nil).Sync(context.Background())
    require.NoError(t, err)
    assert.Empty(t, savedByKey(store, "LTWL-2").Tickets)
}

func TestIngestorDedupesOwnAndInheritedCID(t *testing.T) {
    // Both IVTs labelled 663171; LTWL-1 must not get a duplicate customer row.
    v1 := validJiraIssue("LTWL-1"); v1.Fields["labels"] = json.RawMessage(`["CID-663171"]`)
    v1.Fields["status"] = json.RawMessage(`{"name":"Released"}`)
    v2 := validJiraIssue("LTWL-2"); v2.Fields["labels"] = json.RawMessage(`["CID-663171"]`)
    v2.Fields["status"] = json.RawMessage(`{"name":"Released"}`)
    ct := ctIncorporating("CT-1", "LTWL-1", "LTWL-2")
    jira := &fakeJira{
        pages:   map[string]jira_client.JiraPage{"": {Issues: []jira_client.JiraIssue{v1, v2}}},
        ctPages: map[string]jira_client.JiraPage{"": {Issues: []jira_client.JiraIssue{ct}}},
    }
    store := &fakeVulnerabilityStore{outcome: dao.LightwellVulnerabilityInserted}
    _, err := NewIngestor(jira, store, nil, nil).Sync(context.Background())
    require.NoError(t, err)
    assert.Equal(t, []dao.LightwellVulnerabilityTicket{{TicketID: "", CustomerID: "663171"}},
        savedByKey(store, "LTWL-1").Tickets)
}
```

Add a `savedByKey(store *fakeVulnerabilityStore, key string) dao.LightwellVulnerabilityInput` test helper (finds the saved input whose `VulnerabilityKey == key`). Reuse `ctIncorporating` from Task 1's test file (same package).

- [ ] **Step 2: Extend `fakeJira` for CT pages**

In `ingestion_test.go`, add field `ctPages map[string]jira_client.JiraPage`. In `Search`, add a branch: `if jql == ComponentUpgradeJQL { return f.ctPages[token], nil }` placed before the existing `if jql != VulnerabilityJQL` batch branch.

- [ ] **Step 3: Run tests to verify they fail**

Run: `go test ./pkg/lightwell/sync/ -run 'Inherits|NonDoneSibling|DedupesOwnAndInherited' -v`
Expected: FAIL (undefined `ComponentUpgradeJQL`, `searchCTs`; `syncIssue` arity).

- [ ] **Step 4: Implement the fetch, wiring, and merge in `ingestion.go`**

Generalize the paginator: extract `func (i *Ingestor) searchPaged(ctx context.Context, jql string, fields []string) ([]jira_client.JiraIssue, error)` from `searchAll` (keep `searchAll` as a thin caller with `VulnerabilityJQL`/`vulnerabilityFields`). Add `searchCTs` calling `searchPaged(ctx, ComponentUpgradeJQL, componentUpgradeFields)`. In `Sync`, after `searchAll`: fetch CTs, `inherited := buildInheritedCIDs(issues, cts)`, and pass `inherited[issue.Key]` into `syncIssue`. In `syncIssue`, after the stage is finalized (including the `promoteByJiraState` block), add: `if vulnerability.Stage == "Lightwell Network" { tickets = mergeInheritedCustomers(tickets, inheritedCIDs) }`. Implement `mergeInheritedCustomers` to append a `TicketLink{CustomerID: cid}` for each cid not already present among `tickets` CustomerIDs.

- [ ] **Step 5: Run the full package tests**

Run: `go test ./pkg/lightwell/sync/ -v`
Expected: PASS (new tests and all existing tests).

- [ ] **Step 6: Commit**

```bash
git add pkg/lightwell/sync/ingestion.go pkg/lightwell/sync/ingestion_test.go
git commit -m "feat(beacon): credit inherited CIDs onto done IVTs from Closed/Done CTs"
```

---

## Notes for the executor

- A CT fetch failure should fail the sync like `searchAll` does (return the error from `Sync`), consistent with the existing fetch's error handling.
- Do not touch the `promoteByJiraState` / `skipPublicationVerification` toggles — out of scope.
- Residual gap vs the dashboard (coverage #2, done-definition #3) is expected and documented in the spec; do not attempt to close it here.
