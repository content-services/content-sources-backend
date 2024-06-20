## Introspection

## Background

One of the purposes of the content-sources application is to provide a central registry of repositories used by an organization, alongside information about those repositories.  This information includes what packages, advisories, and package groups included within the repository.  If a service needs to install content from an introspected repository it would need to use the external URL to pull the RPMs directly, no RPMs are actually hosted by our service. 

### What is introspection?
Introspection is the process of learning about a yum repository, including details around packages, advisories/errata, and package groups.

This information is stored within the database and can be fetched or searched via the api.

### How often does introspection occur?
Repositories are attempted to be introspected once per day. A job is run hourly that attempts to introspect any repos that have not been introspected successfully in the last 24 hours.

* this job can be run manually with `go run cmd/external-repos/main.go nightly-jobs`

Introspection can also be triggered directly via the API & UI, or via a command line commands:
* `go run cmd/external-repos/main.go introspect-all`
* `go run cmd/external-repos/main.go introspect https://myrepo.example.com/path/`


### How is repository introspection data used?

Image builder can build images using data from the content-sources api.  When building an image Image Builder will list repositories within the users organization and allow the user to search for packages within their selected repositories.

### What does the introspection process look like?

For most of the heavy lifting, content-sources uses [yummy](https://github.com/content-services/yummy) for downloading and parsing yum metadata.
Yummy downloads the `./repodata/repomd.xml` file within the yum repository.  This file provides an index of other files available.  It will then download the following files:
* primary: contains package information
* updateinfo: contains advisory/errata information
* group or group_gz: contains package group and package environment information

Once the data is downloaded, it is saved within the database.
