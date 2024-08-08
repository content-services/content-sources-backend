## Content Templates

## Background

Content Templates enable users to set a baseline of repository snapshots that help them control patching in predictable ways. In the future,
users will be able to build images with a Content Template.

### What is a Content Template

Content Templates are entities that can include snapshots from different Red Hat and custom repositories. 

The content included in Content Templates is served at different distribution paths in pulp for Red Hat and custom repositories since they are in different domains.

For Red Hat content, the distribution path would be `/pulp/content/<red_hat_domain>/templates/<template_uuid>/`. For example, for this [Red Hat repository](https://cdn.redhat.com/content/dist/layered/rhel8/x86_64/ansible/2/os/), 
the content would be at `/pulp/content/<red_hat_domain>/templates/<template_uuid>/content/dist/layered/rhel8/x86_64/ansible/2/os/`.

For custom content, the distribution path would be `/pulp/content/<custom_domain>/templates/<template_uuid>/<repository_uuid>/`.

### What happens when a Content Template is created, updated, and deleted

After a repository has been snapshotted, a Content Template can be created with that snapshot. 

On Content Template creation and update:

1. The Content Template is created or updated and stored in the content sources database with associations to the repositories that are included in the Content Template
2. A task is started that first creates or updates the pulp distributions, then makes the necessary changes to the related entities in Candlepin

On Content Template deletion:

1. The Content Template is soft-deleted, and therefore no longer visible to the user
2. A task is started to permanently delete the Content Template in our database, its distributions in pulp, and its related entities in Candlepin

### What happens in Candlepin when a Content Template is created, updated, and deleted

The task started on Content Template create, update, and delete modifies any entities in Candlepin needed to represent the Content Template. 
These entities are:

* **Product**: contains the custom repositories included in the Content Template. Can be subscribed to by the owner. Currently we only create one custom product per org.
* **Pool**: acts as the subscription for the custom product. Currently we only create one custom pool per org.
* **Content**: represents the set of Red Hat and custom repository content. Content objects are created only for custom content, the Red Hat content objects must already exist.
* **Environment**: represents the Content Template and contains the content associated with the Content Template. 

On Content Template creation:

1. A product is created if it doesn't already exist
2. A pool is created if it doesn't already exist
3. The content is created and added to the product 
4. An environment is created  
5. The content is promoted to the environment

On Content Template update, any new content is created and either promoted to or demoted from the environment.

On Content Template deletion, the environment is deleted.

### How are Content Templates used

Content Templates can be assigned to systems to configure those systems to pull updates from the snapshots instead of the upstream repositories. Systems
are associated with Candlepin environments within Candlepin, which is done with the Patch application. This will eventually help build “historical” 
images containing past content and software versions that are known to be compatible with customers' workflows.

To test this out on a RHEL client, see these [steps](../register_client.md).
