## Snapshotting

## Background

Yum repositories provided by Red Hat and third parties could be updated daily or weekly.  If a user wants to build an image and iterate on that image, or control what patches are applied to systems, they cannot use these external repositories directly.  Instead users can create point-in-time snapshots to create a stable baseline of content to build images upon or patch systems with.

### What is snapshotting?

While introspection only stores information about the repositories, snapshots store entire copies of the repositories.  This includes copies of all RPMs and metadata such as advisories.  Taking a snapshot takes much longer than introspection as all metadata is parsed and stored, and all RPMs are downloaded.

The snapshots are served at a path `/pulp/content/<domain>/<distribution_path>`.  For example: `/pulp/content/96d6d190/804288c1-13c3-40bc-9abf-ec2ed4ef4a12/8d6336b5-0569-4c3e-9341-1b3a049c67d2/`.

### How often does snapshotting occur?

A cron job is run every hour that introspects at least 1 out every 24 repositories with the goal of snapshotting every repository at least once a day.

* this job can be run manually with `go run cmd/external-repos/main.go nightly-jobs 24`
* or to snapshot only those repositories with a URL: `go run cmd/external-repos/main.go snapshot https://myrepo.example.com/path/`

### How are snapshots used?

1. Snapshots can be used for patching a RHEL client by configuring a client to use a snapshot URL.  The easiest way to do this is by downloading the configuration file from  `/api/content-sources/v1/snapshots/{snapshot_uuid}/config.repo` and placing the file inside the /etc/yum.repos.d/ directory on the client.
2. Image Builder can build using snapshots for a given date.  During the image build workflow, the user can select a date when setting up the image.  If selected, the date will be used to select snapshots for Red Hat and Custom repositories to use during the image build process.   
3. [Content Templates](/TODO/ADD/Link) provide a way to combine the snapshots of different Red Hat and custom repositories into a single entity.  These templates can then be assigned to systems to configure those systems to pull updates from the snapshots instead of the upstream repositories. 

### What does the snapshotting process look like?

1. A task is started as part of the hourly job, or using an API call.
2. A pulp server is needed to snapshot the repository.  We communicate to the pulp server using an openapi client [Zest](https://github.com/content-services/zest/)  
3. This job creates the required entities within Pulp.  These include:
   * Domain: An "organization" in pulp.  Red Hat Content is stored in one domain, each org's content is stored in its own domain.
   * Remote: Stores information about where to pull the content from
   * Repository:  Holds all the generated repository versions
4. A repository sync is started in pulp, creating a repository version
5. A publication is created for the repository version, this generates new yum metadata
6. A distribution is created that exposes the publication at a given path on the web server
7. Depending on the configuration, a content guard can be created which restricts access to the distribution
