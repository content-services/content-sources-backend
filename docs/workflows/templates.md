## Content templates

## Background

If a user wants to customize their systems with images consisting of different repository versions, content templates
provide the support to do that. This enables users to make and distribute consistent software versions across their managed systems.

### What is a content template?

Content templates are entities that can include snapshots from different Red Hat and custom repositories. 

The content included in templates is served at different distribution paths in pulp for Red Hat and custom since they are in different domains.

For Red Hat content, the distribution path would be `/pulp/content/<red_hat_domain>/templates/<template_uuid>/`. For example, for this [Red Hat repository](https://cdn.redhat.com/content/dist/layered/rhel8/x86_64/ansible/2/os/), 
the content would live at `/pulp/content/<red_hat_domain>/templates/<template_uuid>/content/dist/layered/rhel8/x86_64/ansible/2/os/`.

For custom content, the distribution path would be `/pulp/content/<custom_domain>/templates/<template_uuid>/<repository_uuid>/`.

### What happens on our end when a content template is created/updated/deleted?

After a repository has been snapshotted, a template can be created with that snapshot. 

On template creation/update:

1. The template is created or updated and stored in our database with associations to the repositories that are included in the template
2. A task is started that first creates or updates the pulp distributions, then makes the necessary changes to the related entities in Candlepin

On template deletion:

1. The template is soft-deleted
2. A task is started to permanently delete the template in our database, its distributions in pulp, and its related entities in Candlepin

### What happens in Candlepin when a content template is created/updated/deleted?

The task kicked off on template create/update/delete modifies any entities in Candlepin needed to represent the template. 
These entities are:

* **Product** - contains the custom repositories included in the template. Can be subscribed to by the owner. Currently we only create one custom product per org.
* **Pool** - acts as the subscription for the custom product. Currently we only create one custom pool per org.
* **Content** - represents the set of Red Hat and custom repository content. 
* **Environment** - represents the template and contains the content associated with the template. 

On template creation:

1. A product is created if it doesn't already exist
2. A pool is created if it doesn't already exist
3. The content is created and added to the product 
4. An environment is created  
5. The content is promoted to the environment

On template update, any new content is created and either promoted to or demoted from the environment.

On template deletion, the environment is deleted.

### How are content templates used?

Templates can be assigned to systems to configure those systems to pull updates from the snapshots instead of the upstream repositories.
This will help build “historical” images containing past content and software versions that are known to be compatible with customers' workflows.

To test this out on a RHEL client, see these [steps](../register_client.md).
