# Content Sources

##What is it?
Content Sources is an application for storing information about external content (currently YUM repositories) in a central location.


##Developing

### Configuring Postgresql

As root run: 
```shell
yum install -y postgresql

echo "host    all             all             127.0.0.1/32            trust" >> /var/lib/pgsql/data/pg_hba.conf
echo "host    all             all             ::1/128                 trust" >> /var/lib/pgsql/data/pg_hba.conf
systemctl start postgresql
systemctl enable postgresql
sudo -u postgres createdb content
sudo -u postgres psql -c "CREATE USER content WITH PASSWORD 'content'"
sudo -u postgres psql -c "grant all privileges on database content TO content"
```

### Create your configuration
Create a config file from the example:
```
cp ./configs/config.yaml.example ./configs/config.yaml
```

### Migrate your database (and seed it if desired)
```
go run ./cmd/dbmigrate/main.go up
```
```
go run ./cmd/dbmigrate/main.go seed
```

### Run the server!

```
go run ./cmd/content-sources/main.go
```

###
Hit the api:

```
curl http://localhost:8000/api/content_sources/v1.0/repositories/ ```
```
### Generating new openapi docs:

~~~
go install github.com/swaggo/swag/cmd/swag@latest
make openapi
~~~

### Configuration

The default configuration file in ./configs/config.yaml.example shows all available config options.  Any of these can be overridden with an environment variable.  For example  "database.name" can be passed in via an environment variable named "DATABASE_NAME".

### Contributing
 * Pull requests welcome!
 * Pull requests should come with good tests
 * Generally, feature PRs should be backed by a JIRA ticket and included in the subject using the format:
   * `CONTENT-23: Some great feature`
 
## More info
 * [Architecture](docs/architecture.md)
