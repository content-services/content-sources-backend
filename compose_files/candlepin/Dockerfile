FROM quay.io/candlepin/candlepin:dev-latest

USER root
RUN echo "jpa.config.hibernate.dialect=org.hibernate.dialect.PostgreSQL92Dialect" > /etc/candlepin/candlepin.conf; \
    echo "jpa.config.hibernate.connection.driver_class=org.postgresql.Driver" >> /etc/candlepin/candlepin.conf; \
    echo "jpa.config.hibernate.connection.url=jdbc:postgresql://cs_postgres-content_1/candlepin" >> /etc/candlepin/candlepin.conf; \
    echo "jpa.config.hibernate.connection.username=candlepin" >> /etc/candlepin/candlepin.conf; \
    echo "jpa.config.hibernate.connection.password=candlepin" >> /etc/candlepin/candlepin.conf; \
    echo "candlepin.auth.trusted.enable=true" >> /etc/candlepin/candlepin.conf; \
    echo "candlepin.auth.oauth.enable=true" >> /etc/candlepin/candlepin.conf; \
    echo "candlepin.auth.oauth.consumer.rspec.secret=rspec-oauth-secret" >> /etc/candlepin/candlepin.conf; \
    echo "candlepin.db.database_manage_on_startup=Manage" >> /etc/candlepin/candlepin.conf; \
    echo "candlepin.standalone=true" >> /etc/candlepin/candlepin.conf;

USER tomcat


ENTRYPOINT ["/opt/tomcat/bin/catalina.sh", "run"]
