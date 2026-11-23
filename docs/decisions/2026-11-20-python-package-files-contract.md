# LWLP-1226: proposed Python package files contract

Status: **proposed for review**. This document selects defaults for the first
implementation; it does not add an endpoint or mark LWLP-1226 complete.

## Summary for review

Add an endpoint that lists the actual Python files for one exact package release
in a repository the caller can access. Use Pulp's inventory, including every
wheel and source archive present. This first increment returns file metadata;
downloads, security artifacts, and other ecosystems follow separately.

## Approval questions

1. **Approve the Python-only first increment?** Start with wheels and source
   archives, with downloads, OSV/VEX/SBOMs, sidecars, and Maven/npm tracked as
   follow-ups.
2. **Approve the route and response shape?** Extend the existing repository/Python
   route with `/files`, select an exact full version, and use `data/meta/links`.
   Each file has `filename`, `category`, `packagetype`, `size`, and `sha256`.
3. **Approve access checks and failure behavior?** Apply identity/RBAC, terms,
   repository entitlement, and Lightwell-origin checks before querying Pulp.
   Hidden/missing repositories or no exact-release records return `404`;
   mapping, Pulp, or malformed-inventory failures return `500`.

The sections below provide the detailed rules, examples, and implementation
reference for these proposed defaults.

---

## First increment

List the actual Python distribution files for one exact release in a repository
visible to the caller. Start with wheels and source distributions, including all
wheels present for different platforms. Use Pulp's Python content inventory via
the already pinned Zest client. Do not construct filenames from coordinates.

OSV, VEX, SBOMs, signatures/checksum sidecars, Maven/npm, and downloads are
follow-ups. The response describes the Python distribution inventory only; it
does not claim to enumerate every security artifact associated with a release.
No optional security-artifact sources are queried in this increment, so there is
no partial-results mode. Download behavior belongs to LWLP-1227.

## Request

```http
GET /api/lightwell/v0.1/repositories/{uuid}/python_packages/{name}/{version}/files
```

- `uuid` is the Network repository configuration UUID returned by the repository
  API, not a Pulp repository UUID or a repository name.
- `name` is normalized to lowercase, replacing each run of `-`, `_`, or `.` with
  `-`, matching the normalization already used in
  `pkg/lightwell/sync/publication_packages.go`. For example,
  `Lightwell__Fixture` selects `lightwell-fixture`.
- `version` is the exact full stored version, including any Lightwell build
  suffix. `3.0.1+rhlw.2` selects only that build. `3.0.1` selects that literal
  release if present; it never means the latest rebuild or all rebuilds. Unlike
  package detail, this operation does not group builds by upstream version.
- Percent-encode path segments once. Preserve a literal `+` in the version; use
  `%2B` when constructing the Pulp query. Do not normalize or trim the version.
- Reject malformed UUIDs and invalid Python package names with `400`.

Supported query parameters:

- `limit`: integer, default `100`. `0` uses the default; values above `200` are
  capped at `200`, following shared pagination. Negative or non-integer values
  return `400`.
- `offset`: nonnegative integer, default `0`; invalid values return `400`.
- `sort_by`: `filename`, `filename:asc`, or `filename:desc`; default
  `filename:asc`. Other sort values return `400`. Order by the exact filename,
  case-sensitively, without locale-dependent collation.

The validation of malformed pagination is explicit for this new operation;
the shared parser currently logs binding errors rather than rejecting them.

## Successful response

Return `200 application/json` with the shared `data`, `meta`, and `links`
collection shape from `pkg/api/api.go`. Every item contains these five keys:

- `filename`: nonempty string, exactly as supplied by the inventory. It is the
  name of a discovered file, not a generated download URL.
- `category`: `package` or `unknown` in this increment. Both wheels and source
  distributions use `package`. Clients should tolerate future category values.
- `packagetype`: nonempty string or `null`. Preserve Pulp's value, including
  unfamiliar values. If absent, infer `bdist_wheel` from `.whl` or `sdist` from
  `.tar.gz`; otherwise return `null`.
- `size`: nonnegative integer byte count or `null` when absent. A known empty
  file is `0`; missing metadata is never represented as `0`.
- `sha256`: lowercase 64-character hexadecimal digest of the file bytes, or
  `null` when absent. This is metadata, not evidence of a checksum sidecar.

Classify `bdist_wheel` and `sdist` as `package`. An unfamiliar supplied
`packagetype` is authoritative: retain the item as `unknown` even if its suffix
looks familiar. Use filename inference only when `packagetype` is absent.
Unfamiliar filenames are retained rather than silently filtered out.

Do not expose Pulp URLs, content/artifact hrefs, credentials, or storage paths.
Omit `last_modified`: Pulp record timestamps do not establish file modification
time. Omit `signature_type` until downloadable sidecars are actually supported.

`meta.count` is the total number of files for this release before public
pagination. `meta.limit` is the effective limit after defaulting/capping;
`meta.offset` is the requested offset. `data` is always an array.
`links.first` and `links.last` are relative Network API links;
`links.next` and `links.prev` are omitted when unavailable, matching `api.Links`.
Preserve the encoded coordinates and sorting in every link. Never return Pulp's
pagination links. Offset pagination is deterministic for unchanged inventory;
there is no snapshot guarantee across separate HTTP requests.

Example: the repository UUID below is illustrative. File metadata comes from
the existing local Pulp fixture captured on 19 November 2026.

```http
GET /api/lightwell/v0.1/repositories/11111111-1111-4111-8111-111111111111/python_packages/lightwell-fixture/3.0.1%2Brhlw.2/files?limit=100&offset=0
```

```json
{
  "data": [
    {
      "filename": "lightwell_fixture-3.0.1+rhlw.2-py3-none-any.whl",
      "category": "package",
      "packagetype": "bdist_wheel",
      "size": 1082,
      "sha256": "6e2bba7e461225e2328ad69be15a8c7bed2bb1d96416b1133984bf4fe48ba34d"
    },
    {
      "filename": "lightwell_fixture-3.0.1+rhlw.2.tar.gz",
      "category": "package",
      "packagetype": "sdist",
      "size": 1790,
      "sha256": "580c9aa62e63a828e94a96aa08624763a3eaaeda1519347a9a4d0300506c920d"
    }
  ],
  "meta": {
    "count": 2,
    "limit": 100,
    "offset": 0
  },
  "links": {
    "first": "/api/lightwell/v0.1/repositories/11111111-1111-4111-8111-111111111111/python_packages/lightwell-fixture/3.0.1%2Brhlw.2/files?limit=100&offset=0",
    "last": "/api/lightwell/v0.1/repositories/11111111-1111-4111-8111-111111111111/python_packages/lightwell-fixture/3.0.1%2Brhlw.2/files?limit=100&offset=0"
  }
}
```

## Authorization, release existence, and failures

Reuse Network identity, repositories-read RBAC, and the applicable terms
middleware. Resolve the repository through entitlement-aware
`RepositoryConfig.List` for the caller's org, then explicitly check
`origin == lightwell` and `content_type == python`. The existing
`fetchLightwellRepo` helper supplies visibility but does not itself check origin.
No privileged Pulp request may run before these guards pass. Any shared handler
used from another route family must receive equivalent guards.

Use the scoped Pulp inventory as the authority for exact-release existence in
this operation. Do not require a package-mirror row: the mirror is asynchronously
imported and its detail lookup can group builds by upstream version.

- `400`: invalid request parameters, or a visible Lightwell repository whose
  content type is not Python.
- `404`: repository absent, hidden by org/entitlement, or not Lightwell origin;
  also a successful, fully scoped Pulp query with zero exact-release records.
  Do not distinguish hidden repositories from missing ones.
- `200` with `data: []`: only when the release has records but `offset` is at or
  beyond `meta.count`. Keep the true count. There is no separate
  "existing release with no files" state in this inventory-backed contract.
- `428`: terms acceptance required, using the existing middleware response.
- Identity/RBAC denials retain the existing middleware status and response.
- `500`: repository lookup failure, missing/broken Pulp mapping for a visible
  repository, upstream timeout/authentication/transport/server error, malformed
  inventory, or failure on any inventory page. Reuse the current service's
  server-error convention. An upstream failure is never a successful empty or
  partially populated collection and never proves that a release is missing.

Use the existing `errors` envelope (`pkg/errors/handler_error.go`) and redact
upstream URLs, credentials, and raw response bodies from public error details.
For example, an exact release not present in the scoped inventory returns:

```json
{
  "errors": [
    {
      "status": 404,
      "title": "Package release not found",
      "detail": "No files were found for the requested package release."
    }
  ]
}
```

## Execution decisions

1. Add a narrow Python inventory adapter in `pkg/clients/pulp_client`, calling
   Zest's `ContentPackagesAPI.ContentPythonPackagesList`; no new client dependency
   or generic multi-ecosystem framework. Resolve the authorized repository's
   distribution and pin its repository-version href once per request. Scope all
   inventory pages to that snapshot, normalized name, and exact full version.
2. Follow upstream pagination to completion before sorting and applying public
   pagination. Use the request context and configured client timeout; fetch no
   file bodies and make no per-file artifact request for the demonstrated
   Python metadata. Do not follow arbitrary upstream pagination URLs: advance
   offsets through the same configured client and filters. Detect non-progress
   or incomplete pagination and fail the request.
3. Preserve filename and content/artifact identity internally for the later
   download lookup. Collapse repeated occurrences of the same content record;
   conflicting records for one filename fail with `500` rather than choosing
   arbitrary bytes. Missing filename, negative size, or malformed nonempty
   digest also fail as malformed inventory. Absent size/hash remain nullable.
4. Wire the guarded Network route, response types, and focused tests together.
   Add handler annotations and generate the Network spec with
   `make openapi-lightwell` during implementation; follow repository generation
   requirements for any shared API types. The served spec remains an account
   of implemented endpoints, so this proposal does not edit generated API files.

## Evidence and implementation acceptance

The local development capture demonstrated two exact builds,
`3.0.1+rhlw.1` and `3.0.1+rhlw.2`, each with a wheel/source pair, direct filename,
size and SHA-256 metadata, repository scoping, literal `+` filtering, filename
ordering, and two one-item pages matching the full response. A Pulp CLI query
matched the HTTP inventory. These are synthetic development fixtures, not proof
of stage/production parity or real producer naming.

The implementation must cover:

- Exact build selection, no implicit latest-build fallback, Python name
  normalization, and encoded `+` through routing, Pulp filtering, and links.
- Multiple platform wheels plus source archives, upstream pagination, both sort
  directions, public page boundaries, total count, and invalid pagination.
- Unknown types/names, filename inference, missing size/hash versus zero size,
  duplicate content, conflicting filenames, and malformed records.
- Org/entitlement denial, non-Lightwell origin, wrong content type, RBAC and
  terms guards, with no content lookup after a failed guard.
- Missing release versus a failed upstream page, including timeout and broken
  repository mapping; never return an apparently complete partial inventory.

Before rollout, compare the endpoint against a real producer release in a target
environment, including multiple wheels where available. This is a verification
task, not a prerequisite for proposing the contract or writing fixture tests.

Follow with LWLP-1227's Python download workflow. Add security artifacts only
after verifying inventory locations and authoritative exact-release associations;
do not infer them from filename substrings. Add Maven/npm in separate increments.
Those increments remain part of ticket-scope review; the Python slice alone does
not claim to close the broader list-files requirement.

## References

- [Existing Python routes and repository resolution](../../pkg/handler/packages.go)
- [Network registration and guards](../../pkg/handler/lightwell/api.go)
- [Shared collection types](../../pkg/api/api.go)
- [Shared pagination helpers](../../pkg/handler/api.go)
- [Existing Python name normalization](../../pkg/lightwell/sync/publication_packages.go)
- [Current generated Network specification](../../api/lightwell/openapi.json)
- [Local fixture/import workflow](../lightwell_packages_dev.md)
