# Beacon On Hold reasons

Status: accepted. Implemented under LWLP-4895 (feature request LWLP-3849).

## Context

Beacon shows a vulnerability as **On Hold** but not why. Jira now records the reason as a structured signal: `blocked:<reason>` labels on the stuck Build Task or Component Upgrade (never on the vulnerability itself), defined by the blocker taxonomy (`lightwell/architecture`, `docs/design/blocker-taxonomy-label-system.md`, linked from LWLP-3983; not re-checked when this was written). A vulnerability can also be blocked by an ordinary open work ticket, which has no label. Customers should see a readable reason, not a Jira key or a raw label.

## Decision

1. **Derive the reasons during the existing Jira sync**, from the vulnerability's issue links, only for vulnerabilities whose stored stage is On Hold. Only open blockers count (Jira status category not `done`).
2. **Store raw slugs, map to customer text on read.** The column is `on_hold_reasons TEXT[] NOT NULL DEFAULT '{}'`. The DAO owns the wording (`onHoldReasonDisplay`), the same split already used for `resolution_reason`. The frontend renders what the API returns.
3. **Slugs are sorted and de-duplicated** before saving, so the same Jira state always stores the same value and the upsert's change detection does not see link-order changes as data changes.
4. **An unknown slug is shown as "Blocked on a dependency"** instead of being dropped or shown raw.
5. **Labels are fetched in batches into their own map**, only for the open blockers of On Hold vulnerabilities, requesting only the labels field. They are not read through the sync's shared issue cache, whose entries are fetched for other fields and carry no labels.
6. **If a blocker's labels cannot be fetched, that vulnerability fails the run and is not saved.** Reasons already stored are kept, because saving would overwrite them with an empty list.
7. **No reason is a valid result.** An On Hold vulnerability with no labelled blocker and no open work ticket is saved with an empty list, logged at debug level (`beacon_on_hold_without_reason`), and counted in the sync summary.

## Alternatives considered

- **Store customer text in the database.** Rejected: wording changes would need a data migration, and it breaks the `resolution_reason` precedent.
- **Map slugs to text in the frontend.** Rejected: every client would need its own copy of the wording, and unknown slugs would be handled in several places.
- **Labels on the vulnerability.** Not available: the taxonomy places them on the Build Task or Component Upgrade.
- **On a failed label fetch, save the vulnerability but leave its reasons untouched.** Rejected for now. It needs a third "unknown" state through the upsert SQL, the generated code and the DAO input, and it makes a failure quiet (the vulnerability counts as updated, not failed). Other fields would keep updating, which is its advantage.
- **On a failed label fetch, treat the blocker as having no labels and warn.** Rejected for now: reasons could drop out for one run and come back on the next.
- **A nullable column.** Rejected: it gives "no reasons" two representations, `NULL` and `{}`, which the upsert would treat as different values.

## Consequences

- Customer wording can change with a one-map edit in the DAO, with no schema change.
- Until Jira labelling is widespread (LWLP-3983), most On Hold vulnerabilities will show `blocked-by-ticket` or no reason.
- One unreadable blocker keeps that vulnerability from updating at all while it stays unreadable, including its stage and customer links, until the blocker is readable or removed. It shows up in the sync's failed count and failure list.
- A taxonomy label this code does not know yet shows the generic fallback text until a wording is added.
- The sync makes extra Jira searches (batches of 100 keys) proportional to the number of distinct open blockers of On Hold vulnerabilities.

## Revisit when

- A real sync shows many vulnerabilities failing because their blockers cannot be read. Then reconsider keeping the other fields updating (the two rejected failure options above).
- Jira labelling lands and most On Hold vulnerabilities have a labelled blocker. Then check whether the generic fallback and `blocked-by-ticket` are still useful.
- Product or documentation owners change the customer wording or add taxonomy labels.
