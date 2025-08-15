## A Broad Context for the Content Sources Application

The focus of this document is to explain in plain, down-to-earth language the context and purpose of the Content Sources application. It gives answers to questions like:

- What is the Content Sources application?
- Where does it fit into other Red Hat Products?
- What capabilities does the app provide to users?
- What use-cases does it deal with?

This document should give any newcomers a basic intro so they can have an easier time to jump into other documents in this folder and dive into the code. The appropriate documents that go more into details are linked throughout this document.

### Where can I find the code for the Content app?

The Content application is hosted in 4 repositories:

- [content-sources-frontend](https://github.com/content-services/content-sources-frontend)
- [content-sources-backend](https://github.com/content-services/content-sources-backend)
- [patchman-ui](https://github.com/RedHatInsights/patchman-ui)
- [patchman-engine](https://github.com/RedHatInsights/patchman-engine)

Currently (2025) all the repositories are mantained by one team.

### Where does the Content app fit in? Insights and Console Dot apps

The Content app is just a small part of a bigger application called Insights. The Insights application itself is a part of an even bigger application called Console Dot (or officially Hybrid Cloud Console).

All Red Hat users who purchase a RHEL operating system subscription get access to the Insights app. A usual Red Hat's RHEL user has the role of a system administrator. Insights app gives system administrators additional maintenance capabilities through a GUI for managing their operating systems: they can configure all their systems, build installation images, view vulnerability reports, keep systems up-to-date, apply advisory recommendations and more.

### What is the raison d'être of the Content app?

System administrators need a reliable source of repositories and packages which can be safely installed on a system without compromising that system's integrity. Repositories and packages are updated daily and it makes it hard for system administrators to install a particular historic version of a repository.

The Content Sources application provides such a functionality. It provides snapshots of a targeted repository in time, allowing system administrators to choose a historical version of a repo frozen in time.

Most of the time, system administrators use the Content app as part of the bigger image building workflow when they need to create an image with particular version of repositories in it. Those images they then use to provison servers in bulk.

The future goal is to allow users to define only templates as a use-case, removing the Content app action of adding a repository from the navigation bar. Adding a repository action will then be a part of a template definition. More on Content app actions follows below.

### What can users do with the Content app?

The Content Sources app enables users to choose and define which versions of [packages](https://en.wikipedia.org/wiki/Package_format) contained in a **repository** (collection of packages) they want to keep track of.  The app is effectively a version control system specifically for .rpm packages and repositories. In addition, there is a regular service that checks for updates of repositories and saves any new versions into a database. A repository can come from Red Hat or from a third-party (a custom repository).

Users can also define which repositories they want to use in a **template**. Template works like a blueprint that puts 2 or more repositories together.

So there are three layers: first packages, which are contained in repositories (second), which can be contained in templates (third).

Content app currently works only with the YUM repository format type.

The versioning capability of the Content app is backed by the Pulp Server Service, which is maintained by the Pulp team.

[Learn more about the architecture and dependencies](./architecture.md)

### Content app Capabilities and Actions

1. Adding a Repository

   There are 3 ways to add a repository:

   A: introspect only  
   First you need to provide a URL to a repository. The Introspection only option introspects the repository at the provided URL and creates metadata about the repo, which are then saved into a DB. Plus there is a service that continuously checks for updates of the tracked repositories. No packages are downloaded in this case.

   [Read more about introspection](./workflows/introspection.md)

   B: snapshot  
   This is the default option. It introspects the repository at the provided URL and creates metadata, plus it copies the whole repository into a DB. There is also a service that continuously keeps checking the repo for any changes and creates history of snapshotted versions for the repo. These snapshots can then be used to build an image.

   Snapshot making depends on the [tasking system](./tasking_system/tasking_system.md).

   [Read more about snapshotting](./workflows/snapshotting.md)

   C: upload  
   This option behaves like the snapshot one but for local private repositories. So instead of providing a public URL, users upload a repository, which is then saved as a snaphot into a DB.

2. Creating a Template

   A template is a blueprint that combines two or more snapshots of a repository into one group. Users can add Red Hat and custom repositories to a template, optionally select a snapshot, and then have an image built from that template.

   [Read more about templates](./workflows/templates.md)  
   [Or see this article](https://developers.redhat.com/articles/2025/04/23/how-use-content-templates-red-hat-insights)

### Use-Cases

    1. I want to build a new image with precisely defined versions of repos, which I can then install on all my servers. I want the repo versions to be frozen, so users on those systems cannot update to newer versions.
    - create a new template, then build a new blueprint, then build a new image

    2. I have a test server where I want to use only specific versions of repos.
    - attach to a template from an already running system

    3. I want to use a newer version of a repository on my servers.
    - update template and refresh the systems

If you want to test that a template works for a built system, [see this document](./register_client.md).
