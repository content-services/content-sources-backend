# Architecture

Version: 1.0

Note: this document is targeting the desired architecture for the first milestone (November 2022).  It will be updated as needed. 

This application consists of these parts:
* Rest Api
  * Used for our frontend or for other applications to interact with ours
* Postgresql Database
  * Used for storing our data ([database model](https://www.plantuml.com/plantuml/proxy?cache=no&src=https://raw.githubusercontent.com/content-services/content-sources-backend/main/docs/db-model.puml))
* Kafka Platform
  * Used to push messages for the event listener and other applications that want information around content sources.  These messages could include Repository creation, update, deletion, etc.
* Event listener
  * Will listen for messages via kafka to do background processing tasks.  The results of which are stored in the Postgresql database.
  * For example, at Repository creation or update time, the listener will fetch metadata from the yum repository and introspect it to learn about its packages.

![](architecture.png)



## Deployments

When deploying within kubernetes, we recommend:
 * Minimum 3 pods for the API
   * Each pod consumes about 30 MB of memory
 * Minimum 3 pods for the Event Listener (kafka consumer)
   * Each pod consumes about 35 MB of memory