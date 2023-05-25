package config

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	tlsutils "github.com/RedHatInsights/insights-operator-utils/tls"
	"github.com/Shopify/sarama"
	"github.com/cloudevents/sdk-go/protocol/kafka_sarama/v2"
	cloudevents "github.com/cloudevents/sdk-go/v2"
	ce "github.com/content-services/content-sources-backend/pkg/errors"
	"github.com/content-services/content-sources-backend/pkg/event"
	"github.com/labstack/echo/v4"
	clowder "github.com/redhatinsights/app-common-go/pkg/api/v1"
	"github.com/rs/zerolog/log"
	"github.com/spf13/viper"
)

const DefaultAppName = "content-sources"

type Configuration struct {
	Database            Database
	Logging             Logging
	Loaded              bool
	Certs               Certs
	Options             Options
	Kafka               event.KafkaConfig
	Cloudwatch          Cloudwatch
	Metrics             Metrics
	Clients             Clients            `mapstructure:"clients"`
	Mocks               Mocks              `mapstructure:"mocks"`
	Sentry              Sentry             `mapstructure:"sentry"`
	NewTaskingSystem    bool               `mapstructure:"new_tasking_system"`
	NotificationsClient cloudevents.Client `mapstructure:"notification_client"`
}

type Clients struct {
	RbacEnabled bool   `mapstructure:"rbac_enabled"`
	RbacBaseUrl string `mapstructure:"rbac_base_url"`
	RbacTimeout int    `mapstructure:"rbac_timeout"`
	Pulp        Pulp   `mapstructure:"pulp"`
	Redis       Redis  `mapstructure:"redis"`
}

type Mocks struct {
	Namespace string `mapstructure:"namespace"`
	Rbac      struct {
		UserReadWrite     []string `mapstructure:"user_read_write"`
		UserRead          []string `mapstructure:"user_read"`
		UserNoPermissions []string `mapstructure:"user_no_permissions"`
		// set the predefined response path for the indicated application
		// Applications map[string]string
	} `mapstructure:"rbac"`
}

type Pulp struct {
	Server        string
	Username      string
	Password      string
	EntitledUsers []string `mapstructure:"entitled_users"`
}

type Database struct {
	Host       string
	Port       int
	User       string
	Password   string
	Name       string
	CACertPath string `mapstructure:"ca_cert_path"`
}

type Logging struct {
	Level   string
	Console bool
}

type Certs struct {
	CertPath           string `mapstructure:"cert_path"`
	DaysTillExpiration int
	CdnCertPair        *tls.Certificate
}

type Cloudwatch struct {
	Region  string
	Key     string
	Secret  string
	Session string
	Group   string
	Stream  string
}

type Redis struct {
	Host       string
	Port       int
	Username   string
	Password   string
	DB         int
	Expiration time.Duration
}

type Sentry struct {
	Dsn string
}

// https://stackoverflow.com/questions/54844546/how-to-unmarshal-golang-viper-snake-case-values
type Options struct {
	PagedRpmInsertsLimit      int `mapstructure:"paged_rpm_inserts_limit"`
	IntrospectApiTimeLimitSec int `mapstructure:"introspect_api_time_limit_sec"`
}

type Metrics struct {
	// Defines the path to the metrics server that the app should be configured to
	// listen on for metric traffic.
	Path string `mapstructure:"path"`

	// Defines the metrics port that the app should be configured to listen on for
	// metric traffic.
	Port int `mapstructure:"port"`
}

const (
	DefaultPagedRpmInsertsLimit      = 500
	DefaultIntrospectApiTimeLimitSec = 30
)

var LoadedConfig Configuration

func Get() *Configuration {
	if !LoadedConfig.Loaded {
		Load()
	}
	return &LoadedConfig
}

func RedisUrl() string {
	return fmt.Sprintf("%s:%d", Get().Clients.Redis.Host, Get().Clients.Redis.Port)
}

func readConfigFile(v *viper.Viper) {
	v.SetConfigName("config.yaml")
	v.SetConfigType("yaml")
	v.AddConfigPath("./configs/")
	v.AddConfigPath("../../configs/")
	v.AddConfigPath("../../../configs")

	if path, ok := os.LookupEnv("CONFIG_PATH"); ok {
		v.AddConfigPath(path)
	}
	err := v.ReadInConfig()
	if err != nil {
		log.Logger.Warn().Msgf("config.yaml file not loaded: %s", err.Error())
	}
}

func setDefaults(v *viper.Viper) {
	v.SetDefault("Loaded", true)
	// In viper you have to set defaults, otherwise loading from ENV doesn't work
	//   without a config file present
	v.SetDefault("database.host", "")
	v.SetDefault("database.port", "")
	v.SetDefault("database.user", "")
	v.SetDefault("database.password", "")
	v.SetDefault("database.name", "")
	v.SetDefault("certs.cert_path", "")
	v.SetDefault("options.paged_rpm_inserts_limit", DefaultPagedRpmInsertsLimit)
	v.SetDefault("options.introspect_api_time_limit_sec", DefaultIntrospectApiTimeLimitSec)
	v.SetDefault("logging.level", "info")
	v.SetDefault("metrics.path", "/metrics")
	v.SetDefault("metrics.port", 9000)
	v.SetDefault("clients.rbac_enabled", true)
	v.SetDefault("clients.rbac_base_url", "http://rbac-service:8000/api/rbac/v1")
	v.SetDefault("clients.rbac_timeout", 30)
	v.SetDefault("clients.pulp.server", "")
	v.SetDefault("clients.pulp.username", "")
	v.SetDefault("clients.pulp.password", "")
	v.SetDefault("sentry.dsn", "")
	v.SetDefault("new_tasking_system", false)

	v.SetDefault("cloudwatch.region", "")
	v.SetDefault("cloudwatch.group", "")
	v.SetDefault("cloudwatch.stream", DefaultLogwatchStream())
	v.SetDefault("cloudwatch.session", "")
	v.SetDefault("cloudwatch.secret", "")
	v.SetDefault("cloudwatch.key", "")

	v.SetDefault("clients.redis.host", "")
	v.SetDefault("clients.redis.port", "")
	v.SetDefault("clients.redis.username", "")
	v.SetDefault("clients.redis.password", "")
	v.SetDefault("clients.redis.db", 0)
	v.SetDefault("clients.redis.expiration", 1*time.Minute)

	addEventConfigDefaults(v)
}

func Load() {
	var err error
	v := viper.New()

	readConfigFile(v)
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()
	setDefaults(v)

	if clowder.IsClowderEnabled() {
		cfg := clowder.LoadedConfig

		v.Set("database.host", cfg.Database.Hostname)
		v.Set("database.port", cfg.Database.Port)
		v.Set("database.user", cfg.Database.Username)
		v.Set("database.password", cfg.Database.Password)
		v.Set("database.name", cfg.Database.Name)

		v.Set("cloudwatch.region", cfg.Logging.Cloudwatch.Region)
		v.Set("cloudwatch.group", cfg.Logging.Cloudwatch.LogGroup)
		v.Set("cloudwatch.secret", cfg.Logging.Cloudwatch.SecretAccessKey)
		v.Set("cloudwatch.key", cfg.Logging.Cloudwatch.AccessKeyId)

		v.Set("clients.redis.host", cfg.InMemoryDb.Hostname)
		v.Set("clients.redis.port", cfg.InMemoryDb.Port)
		v.Set("clients.redis.username", cfg.InMemoryDb.Username)
		v.Set("clients.redis.password", cfg.InMemoryDb.Password)

		if clowder.LoadedConfig != nil {
			path, err := clowder.LoadedConfig.RdsCa()
			if err == nil {
				v.Set("database.ca_cert_path", path)
			} else {
				log.Error().Err(err).Msg("Cannot read RDS CA cert")
			}
		}

		// Read configuration for instrumentation
		v.Set("metrics.path", cfg.MetricsPath)
		v.Set("metrics.port", cfg.MetricsPort)
	}

	err = v.Unmarshal(&LoadedConfig)
	if err != nil {
		panic(err)
	}
	cert, err := ConfigureCertificate()
	if err != nil {
		log.Fatal().Err(err).Msg("Could not read or parse cdn certificate.")
	}
	LoadedConfig.Certs.CdnCertPair = cert
	LoadedConfig.Certs.DaysTillExpiration, err = DaysTillExpiration(cert)
	if err != nil {
		log.Error().Err(err).Msg("Could not calculate cert expiration date")
	}

	if LoadedConfig.Clients.Redis.Host == "" {
		log.Warn().Msg("Caching is disabled.")
	}

	if len(clowder.KafkaServers) > 0 {
		log.Warn().Msgf("clowder.KafkaServers has length of %s", strconv.Itoa(len(clowder.KafkaServers)))
		LoadedConfig.NotificationsClient = SetupNotifications(clowder.KafkaServers, LoadedConfig)
	} else {
		log.Warn().Msg("clowder.KafkaServers was empty")
	}
}

const RhCertEnv = "RH_CDN_CERT_PAIR"

// ConfigureCertificate loads in a cert keypair from either, an
// environment variable if specified, or a file path
// if no certificate is specified, we return no error
// however if a certificate is specified but cannot be loaded
// an error is returned.
func ConfigureCertificate() (*tls.Certificate, error) {
	var (
		err       error
		certBytes []byte
	)

	if certString := os.Getenv(RhCertEnv); certString != "" {
		certBytes = []byte(certString)
	} else if Get().Certs.CertPath != "" {
		certBytes, err = os.ReadFile(Get().Certs.CertPath)
		if err != nil {
			return nil, err
		}
	} else {
		log.Warn().Msg("No Red Hat CDN cert pair configured.")
		return nil, nil
	}
	cert, err := tls.X509KeyPair(certBytes, certBytes)
	if err != nil {
		return nil, err
	}
	return &cert, nil
}

// DaysTillExpiration Finds the number of days until the specified certificate expired
// tls.Certificate allows for multiple certs to be combined, so this takes the expiration date
// that is coming the soonest
func DaysTillExpiration(certs *tls.Certificate) (int, error) {
	expires := time.Time{}
	found := false
	if certs == nil {
		return 0, nil
	}
	for _, tlsCert := range certs.Certificate {
		fonCert, err := x509.ParseCertificate(tlsCert)
		if err != nil {
			continue
		}
		if !found || fonCert.NotAfter.Before(expires) {
			expires = fonCert.NotAfter
			found = true
		}
	}
	if !found {
		return 0, nil
	}
	diff := time.Until(expires)
	return int(diff.Hours() / 24), nil
}

func ProgramString() string {
	return strings.Join(os.Args, " ")
}

func CustomHTTPErrorHandler(err error, c echo.Context) {
	var code int
	var message ce.ErrorResponse

	if c.Response().Committed {
		c.Logger().Error(err)
		return
	}

	if errResp, ok := err.(ce.ErrorResponse); ok {
		code = ce.GetGeneralResponseCode(errResp)
		message = errResp
	} else if he, ok := err.(*echo.HTTPError); ok {
		errResp := ce.NewErrorResponseFromEchoError(he)
		code = errResp.Errors[0].Status
		message = errResp
	} else {
		code = http.StatusInternalServerError
		message = ce.NewErrorResponse(code, "", http.StatusText(http.StatusInternalServerError))
	}

	// Send response
	if c.Request().Method == http.MethodHead {
		err = c.NoContent(code)
	} else {
		err = c.JSON(code, message)
	}
	if err != nil {
		log.Logger.Error().Err(err)
	}
}

func SetupNotifications(kafkaServers []string, cfg Configuration) cloudevents.Client {
	saramaConfig := sarama.NewConfig()

	saramaConfig.Version = sarama.V2_0_0_0
	saramaConfig.Consumer.Offsets.Initial = sarama.OffsetOldest

	if strings.Contains(cfg.Kafka.Sasl.Protocol, "SSL") {
		log.Warn().Msgf("Configuring SSL authentication: %s", cfg.Kafka.Sasl.Protocol)
		saramaConfig.Net.TLS.Enable = true
	}

	if cfg.Kafka.Capath != "" {
		log.Warn().Msgf("kafkaCaPath is: %s", cfg.Kafka.Capath)
		tlsConfig, err := tlsutils.NewTLSConfig(cfg.Kafka.Capath)
		if err != nil {
			log.Error().Err(err).Msgf("Unable to load TLS config for %s cert", cfg.Kafka.Capath)
			return nil
		}
		saramaConfig.Net.TLS.Config = tlsConfig
	}

	if strings.HasPrefix(cfg.Kafka.Sasl.Protocol, "SASL_") {
		log.Warn().Msgf("Configuring SASL authentication: %s", cfg.Kafka.Sasl.Protocol)
		saramaConfig.Net.SASL.Enable = true
		saramaConfig.Net.SASL.User = cfg.Kafka.Sasl.Username
		saramaConfig.Net.SASL.Password = cfg.Kafka.Sasl.Password
		saramaConfig.Net.SASL.Mechanism = sarama.SASLMechanism(cfg.Kafka.Sasl.Mechanism)
	}

	protocol, err := kafka_sarama.NewSender(kafkaServers, saramaConfig, "platform.notifications.ingress")
	if err != nil {
		log.Error().Err(err).Msg("failed to create kafka_sarama protocol")
		return nil
	}

	c, err := cloudevents.NewClient(protocol, cloudevents.WithTimeNow(), cloudevents.WithUUIDs())
	if err != nil {
		log.Error().Err(err).Msg("failed to create cloudevents client")
		return nil
	}
	return c
}
