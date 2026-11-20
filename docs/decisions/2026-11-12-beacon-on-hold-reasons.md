# Beacon On Hold reasons

Status: accepted. Implemented under LWLP-4895 (feature request LWLP-3849). Extended on 2026-11-19 after review by Eric Hadley (decisions 8 and 9, and the section "Review on 2026-11-19").

## Context

Beacon shows a vulnerability as **On Hold** but not why. Jira now records the reason as a structured signal: `blocked:<reason>` labels on the stuck Build Task or Component Upgrade (never on the vulnerability itself), defined by the blocker taxonomy (`lightwell/architecture`, `docs/design/blocker-taxonomy-label-system.md`, linked from LWLP-3983; not re-checked when this was written). A vulnerability can also be blocked by an ordinary open work ticket, which has no label. Customers should see a readable reason, not a Jira key or a raw label.

## Decision

1. **Derive the reasons during the existing Jira sync**, from the vulnerability's issue links, only for vulnerabilities whose stored stage is On Hold. That is the case when the vulnerability's own Jira status is On Hold or, since decision 8, when a Component Upgrade that incorporates it is On Hold. Only open blockers count (Jira status category not `done`).
2. **Store raw slugs, map to customer text on read.** The column is `on_hold_reasons TEXT[] NOT NULL DEFAULT '{}'`. The DAO owns the wording (`onHoldReasonDisplay`), the same split already used for `resolution_reason`. The frontend renders what the API returns.
3. **Slugs are sorted and de-duplicated** before saving, so the same Jira state always stores the same value and the upsert's change detection does not see link-order changes as data changes.
4. **An unknown slug is shown as "Blocked on a dependency"** instead of being dropped or shown raw.
5. **Blockers are fetched in batches into their own map**, only for the open blockers of On Hold vulnerabilities (and, since decision 8, the On Hold Component Upgrades of other vulnerabilities), requesting only the fields needed: labels and, since decision 9, issue links and issue type. They are not read through the sync's shared issue cache, whose entries are fetched for other fields and carry no labels or links.
6. **If a blocker cannot be fetched, that vulnerability fails the run and is not saved.** Reasons already stored are kept, because saving would overwrite them with an empty list.
7. **No reason is a valid result.** An On Hold vulnerability with no labelled blocker and no open work ticket is saved with an empty list, logged at warning level (`beacon_on_hold_without_reason`), and counted in the sync summary.
8. **A vulnerability is On Hold when any of its open Component Upgrades is On Hold.** Jira embeds each linked ticket's status name in the "Incorporates" link, and the sync already reads every vulnerability on every run, so this needs no extra query and no search from the Component Upgrade side. It applies to vulnerabilities in Submitted, Classified, Fix in Progress or Validation that Jira does not show as Released or Closed/Done. Shipped (Lightwell Network) vulnerabilities and ones closed without a fix (Unremediated) are left alone. Such a vulnerability takes its reasons only from its On Hold Component Upgrades, so a stale label on another, progressing Component Upgrade does not become a reason, and its own blocking tickets are not read. The stage is worked out again on every run, so the vulnerability returns to its own stage, with no reasons, when the Component Upgrade leaves On Hold. The sync summary counts these vulnerabilities (`on_hold_from_component_upgrade`).
9. **A Component Upgrade's own blocking ticket is a reason.** A Component Upgrade can be blocked by a `blocked:` label, by an open "is blocked by" ticket of its own, or both. The blocker lookup now also fetches `issuelinks` and `issuetype`. A Component Upgrade with an open blocking ticket adds `blocked-by-ticket`, worded like an open ticket that blocks the vulnerability directly. A Build Task's own links are not read.

## Review on 2026-11-19

Eric Hadley reviewed the logic and implementation plan. What he said, and what was done:

- **Label list.** Pointed to the reason vocabulary in the taxonomy. Evan compared the label list and the wording with it and reported no wording changes.
- **Component Upgrade On Hold.** He agreed: the vulnerability is "effectively on hold / blocked". He also offered to use "blocked" wording everywhere. Decision 8. The wording was not changed.
- **A Component Upgrade can be blocked by a link.** "CTs could have a label or also it's own 'is blocked by' link". Decision 9.
- **On Hold vulnerabilities with an open Build Task** are "a weird state". No change: they still show plain "On Hold".
- **Vulnerabilities waiting on a Build Task.** "Blocked IVTs should be moved to On Hold, unless they are waiting on a VBT (build task) which will keep the IVT in New." Evan decided that such vulnerabilities stay as they are: stage Submitted, no reason. Eric also wrote that `blocked:` labels "will typically happen while IVT is still in New". That can mean a Build Task may carry a `blocked:` label while its vulnerability is still New, which this design does not show. See "Revisit when".
- **Not answered.** What to show when a vulnerability is in two Component Upgrades and only one is On Hold: Evan chose "any open Component Upgrade", which is decision 8. And whether the Component Upgrade On Hold status is stable (LWLP-5129): assumed so.

## Alternatives considered

- **Store customer text in the database.** Rejected: wording changes would need a data migration, and it breaks the `resolution_reason` precedent.
- **Map slugs to text in the frontend.** Rejected: every client would need its own copy of the wording, and unknown slugs would be handled in several places.
- **Labels on the vulnerability.** Not available: the taxonomy places them on the Build Task or Component Upgrade.
- **On a failed label fetch, save the vulnerability but leave its reasons untouched.** Rejected for now. It needs a third "unknown" state through the upsert SQL, the generated code and the DAO input, and it makes a failure quiet (the vulnerability counts as updated, not failed). Other fields would keep updating, which is its advantage.
- **On a failed label fetch, treat the blocker as having no labels and warn.** Rejected for now: reasons could drop out for one run and come back on the next.
- **A nullable column.** Rejected: it gives "no reasons" two representations, `NULL` and `{}`, which the upsert would treat as different values.
- **On Hold only when every open Component Upgrade is On Hold.** Rejected. It avoids calling a vulnerability blocked when another Component Upgrade can still deliver the fix, but it hides blocked vulnerabilities, which is the gap raised in review. It also differs from how a vulnerability's own blockers work, where any open blocker counts. The cost of the chosen rule: a vulnerability in a second, progressing Component Upgrade still reads On Hold, and its reason line says why.
- **Keep the vulnerability's stage and add a note** such as "component upgrade on hold". Rejected: the vulnerability would not count in the On Hold filter or the summary chart.
- **A separate list of On Hold Component Upgrades.** Rejected as more than needed: the "Incorporates" links already reach every affected vulnerability.
- **Also read a held vulnerability's own blocking tickets.** Not done: only the On Hold Component Upgrades' reasons are shown. Revisit if the reasons look incomplete.

## Consequences

- Customer wording can change with a one-map edit in the DAO, with no schema change.
- On the first real sync (13 Nov snapshot, 813 On Hold vulnerabilities), 697 (86%) had at least one reason and 116 (14%) had none. `blocked-by-ticket` was the most common reason (501), and label-based reasons were already widespread. Of the 116 without a reason, 113 had an open Build Task or Component Upgrade with no `blocked:` label.
- One unreadable blocker keeps that vulnerability from updating at all while it stays unreadable, including its stage and customer links, until the blocker is readable or removed. It shows up in the sync's failed count and failure list.
- A taxonomy label this code does not know yet shows the generic fallback text until a wording is added.
- The sync makes extra Jira searches (batches of 100 keys) proportional to the number of distinct open blockers of On Hold vulnerabilities, now including the On Hold Component Upgrades of other vulnerabilities. Each search also returns the blockers' issue links.
- The On Hold count rises by the vulnerabilities held through their Component Upgrades, and a vulnerability's Beacon stage can now differ from its Jira status. How many cannot be known without a real sync: the first run reports it in `on_hold_from_component_upgrade`. On 2026-11-19 Jira had 6 On Hold Component Upgrades (14 the day before), so the number moves.
- The rule depends on the Component Upgrade status name "On Hold" (LWLP-5129). If that status is renamed, these vulnerabilities silently stop being held.
- A vulnerability held by a Component Upgrade with no `blocked:` label and no blocking ticket shows plain "On Hold".
- A vulnerability kept in New by a blocked Build Task gets no reason line. This is accepted for now (see "Review on 2026-11-19").

## Revisit when

- A real sync shows many vulnerabilities failing because their blockers cannot be read. Then reconsider keeping the other fields updating (the two rejected failure options above). The first real sync (13 Nov) had no such failures.
- Jira labelling lands and most On Hold vulnerabilities have a labelled blocker. Then check whether the generic fallback and `blocked-by-ticket` are still useful.
- Product or documentation owners change the customer wording or add taxonomy labels.
- Eric or product says vulnerabilities kept in New by a blocked Build Task should show a reason, or the Component Upgrade statuses change under LWLP-5129.
- The first real sync holds many more vulnerabilities than expected, or the ones it holds look wrong. The sync summary's `on_hold_from_component_upgrade` count is where that shows up.
- Beacon's wording changes from "On Hold" to "blocked" (Eric offered this).
