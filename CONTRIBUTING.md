# Contributing

 * Pull requests welcome!
 * Pull requests should come with good tests
 * All PRs should be backed by a JIRA ticket and included in the subject using the format:
   * `Fixes 23: Some great feature` - if a PR fully resolves an issue or feature, or is the last PR in a series of PRs.
   * `Refs 23: Partial work for issue` - if a PR partially resolves an issue or feature or is not the last PR in a series of PRs.
   * If you do not have access to the Jira (we are working on opening it up), open a PR without this
     and we will add it for you.

## A Note on SQL migrations

When undertaking a migration consideration of the previous interation of database structure must be made.
This means, that when desctructive actions are made, as when changing variable names or deleting columns, we need to ensure that there will be no down time during deployment.

To this end, should such actions that could potentially cause destructive changes be made, we should role out changes in a two-stage approach. 

As an example: 

If we wanted to change a column name from "metadata_verification" to simply "metadata". 

1. We would first have a PR that would migrate all fields/values to the new column, while keeping the existing column.
2. Merge/deploy/test said PR.
3. Merge/deploy/test a follow up PR to drop the <i>now</i> unused "metadata_verification" column .


