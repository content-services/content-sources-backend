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


Create a `.env` file within your git checkout with:
~~~shell
DATABASE_HOST=localhost
DATABASE_PORT=5432
DATABASE_USER=content
DATABASE_PASSWORD=content
DATABASE_NAME=content
~~~
pointed to your postgresql db.


####Running the server
Load the db variables:
```shell  
export `cat .env`
```

Run the server:
```shell
go run ./cmd/content_sources/main.go
```

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
