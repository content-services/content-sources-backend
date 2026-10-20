# Beacon: CT-based CID inheritance for the "done" count

**Date:** 2026-10-20
**Component:** Lightwell Beacon (`pkg/lightwell/`) in `content-sources-backend`
**Status:** Design — pending approval

## Intent and why

Beacon's per-customer "done" count (vulnerabilities at `stage = 'Lightwell Network'`
for a customer) is lower than the remediation-dashboard's "shipped" count for the same
customer (observed: beacon 397 vs dashboard 418 for CID-663171). Three independent
drivers were identified; this change addresses **only the first**:

1. **CT-group CID inheritance (this change).** The dashboard credits a customer for
   *every* IVT in a Component Upgrade (CT) if *any* IVT in that CT carries the
   customer's `CID-<n>` label — because they all shipped together in one CT
   (reconcile.py `_inherit_ct_cids`, "Eric's model", 2026-09-25). Beacon has no CT
   awareness at all: it associates a customer with an IVT only when that *specific*
   IVT carries the `CID-<n>` label. So inherited credit is invisible to beacon.
2. **Sync coverage gap (out of scope).** Live Jira has ~3057 IVTs labelled for
   CID-663171; beacon's DB has ~3015. Some labelled IVTs are simply not in beacon yet.
3. **"Done" definition difference (out of scope).** The dashboard counts a `Verified`
   IVT in a Closed/Done CT as shipped; beacon requires the IVT's *own* Jira status to
   be `Released` or `Closed/Done` (→ `Lightwell Network` stage).

**Goal:** make beacon credit a customer for all IVTs that shipped in a Closed/Done CT
when any IVT in that CT carries the customer's CID label — matching the dashboard's
inheritance semantics — so the per-customer "done" count moves toward parity.

## Goals

- Beacon associates customer `C` with IVT `X` (for the purpose of the "done" count)
  when `X` is a member of a Closed/Done Component Upgrade whose membership includes at
  least one IVT labelled `CID-C`, even if `X` itself is not labelled `CID-C`.
- Inheritance is sourced from Jira by fetching Component Upgrade issues and reading
  their "Incorporates" links (the dashboard's source of truth).
- The change is additive: existing direct-label associations are unchanged.

## Non-goals / anti-goals

- **Not** closing the sync coverage gap (#2) or changing the "done"/stage definition (#3).
- **Not** changing any stage count other than "done". Inherited credit must *only*
  affect the `Lightwell Network` bucket — it must not inflate a customer's `Submitted`,
  `Fix in Progress`, `Validation`, etc. counts. (A sibling IVT pulled in by inheritance
  that is *not* itself done must not appear in that customer's non-done counts.)
- **Not** achieving an exact 418 match — drivers #2 and #3 remain. A failure mode would
  be silently inflating non-done counts, or crediting through non-Closed/Done CTs.
- **Not** a DB schema change if avoidable (the write path already upserts cleanly).

## Decisions made (by the user, 2026-10-20)

1. **Credit scope:** done-only — inherited credit affects only the `Lightwell Network`
   (done) bucket, matching the dashboard (whose inheritance applies only within the
   shipped/Closed-Done-CT population).
2. **CT population:** Closed/Done CTs only — `status = Closed AND resolution = Done`,
   for both `Backport` and `Novel Fix` components (the dashboard's two shipped-credit
   CT populations).
3. **CT membership source:** fetch Component Upgrade issues from Jira and read each
   CT's "Incorporates" issue links to build CT→IVT groups (a second fetch pass),
   rather than deriving from each IVT's own `issuelinks`.
4. **Ship-ready propagation gate:** only IVTs whose own Jira status is ship-ready
   (Verified or later) contribute their CID label to a CT's union — matching the
   dashboard, which excludes non-ship-ready members from the group entirely
   (`jira_client.py:87-91`, `246-268`, `554-575`). A CID on a pre-Verified IVT does not
   propagate to done siblings.

## Current state (as investigated)

- Sync fetches only IVTs: `VulnerabilityJQL = "project = LTWL AND type = Vulnerability
  AND issuekey NOT IN (...) ORDER BY created ASC"` (`sync/ingestion.go:18`), paginated
  via `searchAll` (`ingestion.go:270`).
- Customer extraction: `customerIDsFromLabels` / `customerLabelPattern`
  `(?i)^CID-(\d+)$` (`sync/mapper.go:289-316`), stored as the bare number.
- Per-IVT customer links assembled in `issueRelationships` (`ingestion.go:446-465`)
  from that IVT's own labels; written by `reconcileTickets`
  (`pkg/dao/lightwell_vulnerability.go:199-246`) via `InsertVulnerabilityCustomer`
  (`db/queries/vulnerabilities.sql:172`), with stale rows pruned per-vuln by
  `DeleteVulnerabilityCustomersNotIn`.
- Stage promotion to `Lightwell Network`: `applyPublishedStage` + `releasedOrDone`
  (`ingestion.go:423-436`, `mapper.go:479-488`). Pulp gating currently disabled via
  `skipPublicationVerification=true` / `promoteByJiraState=true`.
- **No CT data exists anywhere** in beacon (ingested, stored, or derivable).

## Proposed approach (left to the builder to refine in the plan)

A new sync stage, run after the IVT fetch and before/within the per-issue write loop:

1. **Fetch Closed/Done CTs.** New JQL, e.g.
   `project = LTWL AND issuetype = "Component Upgrade" AND component in ("Backport",
   "Novel Fix") AND status = Closed AND resolution = Done`, requesting the links/field
   that expresses "Incorporates" membership. (Builder: confirm whether membership is an
   issue link named "Incorporates" and in which direction, mirroring reconcile.py /
   jira_client.py's CT→IVT extraction.)
2. **Build CT groups → inherited-CID map.** For each CT, collect its member IVT keys,
   **keeping only members whose own status is ship-ready (Verified+)** (the dashboard's
   membership gate — see decision #4). Using the already-fetched IVT label data
   (key → own CID set), union the CID sets across each CT's ship-ready members, and
   produce `inheritedCIDs: map[ivtKey]set[CID]` assigning that union to every member.
   (Only CID labels on ship-ready members contribute; a label on a pre-Verified member
   does not propagate.)
3. **Apply done-only.** During the per-IVT customer assignment, add the IVT's
   `inheritedCIDs[key]` to its customer set **only when that IVT's computed stage is
   `Lightwell Network`**. Non-done IVTs get no inherited customers, so non-done stage
   counts are untouched. Direct-label associations are always kept as today.
4. **Write** through the existing `reconcileTickets` path (no schema change): the
   augmented customer set is reconciled per-vuln exactly as direct labels are now.

### Why done-only works with beacon's global customer table

Beacon's `lightwell_vulnerability_customers` is stage-agnostic, so naively adding
inherited rows would inflate *all* stage counts. Gating the *insertion* on the IVT
being at `Lightwell Network` confines the effect to the done bucket while reusing the
existing single-table write path. `CountByStage` groups by stage, so inherited rows
surface only in the `Lightwell Network` group.

### Residual differences (expected, documented)

After this change, beacon's "done" for a customer will include inherited done IVTs but
will still differ from the dashboard by drivers #2 (coverage) and #3 (an IVT that is
`Verified`, i.e. beacon stage `Validation`, but is in a Closed/Done CT — the dashboard
counts it, beacon still won't). Exact parity is explicitly not a goal here.

## Builder-check resolutions (settled, 2026-10-20)

These were raised by a design review and settled as author's calls (no behavioural
ambiguity for a sensible builder):

- **"Incorporates" link direction:** read membership off the **CT's** `fields.issuelinks`;
  the member IVT is `link.outwardIssue or link.inwardIssue` (mirrors
  `remediation-dashboard/jira_client.py:260,567`).
- **`issueRelationships` early-return** when an IVT has no own CID labels
  (`ingestion.go:447-450`): the inherited-CID map must be threaded in *before* that guard
  so an unlabelled-but-inheriting done IVT still reaches customer assignment.
- **Inherited customers have no support ticket:** set `TicketID=""`; `reconcileTickets`
  inserts the customer row and skips the empty-ticket upsert cleanly — no schema change.
- **Singleton CTs (<2 members):** nothing to propagate; replicate the dashboard's skip or
  ignore — no behavioural impact.
- **CT fetch fields:** request `issuelinks` including the inline `outwardIssue.fields.status`
  stub (needed for the ship-ready gate below); no changelog needed (beacon computes no ship
  dates).
- **CTs with members outside the fetched IVT set:** skip (matches `_inherit_ct_cids`, which
  only updates keys already in the population).

## Left to the builder

- Exact Jira link name/direction for CT "Incorporates" membership and the field set to
  request; reconcile how the dashboard (`jira_client.py`, `reconcile.py`) extracts CT
  members and reuse that definition.
- Whether to fetch CTs via the existing `searchAll` paginator or a dedicated helper;
  pagination and field selection.
- How to thread the inherited-CID map into the per-issue loop (a prebuilt map passed
  into `issueRelationships`, per the investigation's suggested insertion point).
- Test strategy (TDD): unit tests for the union/group logic and the done-only gate;
  integration coverage against the local Postgres fixture for the customer reconcile.
- Handling CTs whose member IVTs are outside the fetched IVT set (skip / log).
