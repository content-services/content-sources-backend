## A Broad Context for the Content Sources Service

The focus of this document is to explain in plain, down-to-earth language the context and purpose of the Content Sources service. It gives answers to questions like:

- What is the Content Sources service?
- Where does it fit into other Red Hat Products?
- What capabilities does the service provide to users?
- What use-cases does it deal with?

This document should give any newcomers a basic intro so they can have an easier time to jump into other documents in this folder and dive into the code. The appropriate documents that go more into details are linked throughout this document.

### Where can I find the code for the Content Sources service?

The Content service is hosted in 2 repositories:

- [content-sources-frontend](https://github.com/content-services/content-sources-frontend)
- [content-sources-backend](https://github.com/content-services/content-sources-backend)

Currently (2025) both repositories are mantained by one team.

### Where does the Content Sources service fit in? Insights application and Console Dot platform

The Content Sources service is just a small part of a bigger application called Insights. The Insights service itself is a part of an even bigger platform called Console Dot (or officially Hybrid Cloud Console). The Insights app was originally based on the Satellite app (which is a self-hosted system management app), but in a lighter format with fewer features and offered in a Saas format.

All Red Hat users who purchase a RHEL operating system subscription get access to the Insights app. A usual Red Hat's RHEL user has the role of a system administrator. Insights app gives system administrators additional maintenance capabilities through a GUI for managing their operating systems: they can configure all their systems, build installation images, view vulnerability reports, keep systems up-to-date, apply advisory recommendations and more.

### Why does the Content Sources service exist?

System administrators need a reliable source of repositories and packages which can be safely installed on a system without compromising that system's integrity. Repositories and packages are updated daily and it makes it hard for system administrators to install a particular historic version of a repository.

The Content Sources service provides such a functionality. It provides snapshots, which are frozen in time versions of a repository, allowing system administrators to choose a historical version of a repository.

Most of the time, system administrators use the Content Sources service as part of the bigger image building workflow when they need to create an image with particular version of repositories in it. Those images they then use to provison servers in bulk.

The future goal is to allow users to define only templates as a use-case, removing the Content Sources service action of adding a repository from the navigation bar. Adding a repository action will then be a part of a template definition. More on Content Sources service actions follows below.

### What is the Content Sources service and what can users do with it?

The Content Sources service enables users to choose and define which versions of [packages](https://en.wikipedia.org/wiki/Package_format) contained in a **repository** (collection of packages) they want to keep track of. A repository can come from Red Hat or from a third-party (a custom repository - which can be either public - provided through a URL - or private - uploaded). The service is effectively a version control system specifically for .rpm packages and repositories. There is a regular service that checks for updates of repositories, takes their snapshots and saves those snapshots as a new version into a database. So in the end there are stacks of those snapshots for each repository, which serve as a version history for a particular repository. This provides users with the ability to use older versions of repositories.

Users can also define which snapshots of repositories they want to use in a **template**. [Read about templates below](#creating-a-template)

So there are three layers: first packages, which are contained in repositories (second), which can be contained in templates (third).

Content Sources service currently works only with the YUM repository format type.

The versioning capability of the Content Sources service is backed by the Pulp Server Service, which is maintained by the Pulp team.

[Learn more about the architecture and dependencies](./architecture.md)

### Content Sources service Capabilities and Actions

#### Adding a Repository

There are 3 ways to add a repository (which can be either a Red Hat repository or a custom one):

A: Introspect only  
 First you need to provide a URL to a repository. The Introspection only option introspects the repository at the provided URL and creates metadata about the repo, which are then saved into a DB. Plus there is a service that continuously checks for updates of the tracked repositories. No packages are downloaded in this case.

[Read more about introspection](./workflows/introspection.md)

B: Snapshot  
 This is the default option. It introspects the repository at the provided URL and creates metadata, plus it copies the whole repository into a DB. There is also a service that continuously keeps checking the repo for any changes and creates history of snapshotted versions for the repo. These snapshots can be used directly (users add the repository config file for a single snapshot to their system and then they are able to `dnf install` the packages in that snapshot), within a template, or to build an image.

Snapshot making depends on the [tasking system](./tasking_system/tasking_system.md).

[Read more about snapshotting](./workflows/snapshotting.md)

C: Upload  
 This option behaves like the snapshot one but for local private repositories. So instead of providing a public URL, users specify one or more rpms to upload to a repository created for uploaded content, which is then saved as a snaphot into a DB. A snapshot of this type of repository is created each time a user uploads content.

#### Creating a Template

A template is a collection of repository snapshots, including snapshots of Red Hat repositories and optionally custom ones.
There are multiple use-cases for templates. These include:

1.  A template can be assigned to a system and, once assigned, that system will pull updates from the snapshots.
2.  Templates can also be used within Image Builder to build an image that will include the repository snapshots.
3.  By using a template, users can define their [Standard Operating Environment](https://en.wikipedia.org/wiki/Standard_Operating_Environment).

If you want to test that a template is assigned to a system, [see this document](./register_client.md).

[Read more about templates](./workflows/templates.md)  
 [Or see this article introducing templates](https://developers.redhat.com/articles/2025/04/23/how-use-content-templates-red-hat-insights)

### Use-Cases

    1. I want to build a new image with precisely defined versions of repos, which I can then install on all my servers. I want the repo versions to be frozen, so users on those systems cannot update to newer versions.
    - create a new template, then build a new blueprint, then build a new image

    2. I have a test server where I want to use only specific versions of repos.
    - attach to a template from an already running system

    3. I want to use a newer version of a repository on my servers.
    - update template and refresh the systems

### Related Services

There is a related service called Patch that shares some things with the Content Sources App.  
You can find its code following these links:

- [patchman-ui](https://github.com/RedHatInsights/patchman-ui)
- [patchman-engine](https://github.com/RedHatInsights/patchman-engine)

### Note on Terminology - Service, Application, Platform

#### Service

Smaller part of a bigger whole. It provides specific features and is not able to stand alone. It needs other parts and services to provide meaningful functionality to users. (Content Sources service)

#### Application

Software that is able to stand alone from users perspective. Groups more services together into a bigger whole (Insights application).

#### Platform

Even bigger whole that groups multiple applications (Console Dot Platform).
