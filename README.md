# Content Sources

##What is it?
Content Sources is an application for storing information about external content (currently YUM repositories) in a central location.


##Developing

Create a `.env` file with:
~~~
DATABASE_HOST=localhost
DATABASE_PORT=5432
DATABASE_USER=content
DATABASE_PASSWORD=content
DATABASE_NAME=content
~~~

pointed to your postgresql db.


### Generating new openapi docs:

~~~
go install github.com/swaggo/swag/cmd/swag@latest
make openapi
~~~

### Contributing
 * Pull requests welcome!
 * Pull requests should come with good tests
 * Generally, feature PRs should be backed by a JIRA ticket and included in the subject using the format:
   * `CONTENT-23: Some great feature`
 
## More info
 * [Architecture](docs/architecture.md)
