package config

import (
	"crypto/tls"
	"net/http"
	"os"
	"strings"

	ce "github.com/content-services/content-sources-backend/pkg/errors"
	"github.com/content-services/content-sources-backend/pkg/event"
	"github.com/labstack/echo"
	clowder "github.com/redhatinsights/app-common-go/pkg/api/v1"
	"github.com/rs/zerolog/log"
	"github.com/spf13/viper"
)

const DefaultAppName = "content-sources"

type Configuration struct {
	Database   Database
	Logging    Logging
	Loaded     bool
	Certs      Certs
	Options    Options
	Kafka      event.KafkaConfig
	Cloudwatch Cloudwatch
	Metrics    Metrics
	Clients    Clients `mapstructure:"clients"`
	Mocks      Mocks   `mapstructure:"mocks"`
}

type Clients struct {
	RbacEnabled bool   `mapstructure:"rbac_enabled"`
	RbacBaseUrl string `mapstructure:"rbac_base_url"`
	RbacTimeout int    `mapstructure:"rbac_timeout"`
}

type Mocks struct {
	MyOrgId   string `mapstructure:"my_org_id"`
	Namespace string `mapstructure:"namespace"`
	Rbac      struct {
		AccountAdmin  string `mapstructure:"account_admin"`
		AccountViewer string `mapstructure:"account_viewer"`
		// set the predefined response path for the indicated application
		// Applications map[string]string
	} `mapstructure:"rbac"`
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
	CertPath    string `mapstructure:"cert_path"`
	CdnCertPair *tls.Certificate
}

type Cloudwatch struct {
	Region  string
	Key     string
	Secret  string
	Session string
	Group   string
	Stream  string
}

// https://stackoverflow.com/questions/54844546/how-to-unmarshal-golang-viper-snake-case-values
type Options struct {
	PagedRpmInsertsLimit int `mapstructure:"paged_rpm_inserts_limit"`
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
	DefaultPagedRpmInsertsLimit = 500
)

var LoadedConfig Configuration

func Get() *Configuration {
	if !LoadedConfig.Loaded {
		Load()
	}
	return &LoadedConfig
}

func readConfigFile(v *viper.Viper) {
	v.SetConfigName("config.yaml")
	v.SetConfigType("yaml")
	v.AddConfigPath("./configs/")
	v.AddConfigPath("../../configs/")

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
	v.SetDefault("logging.level", "info")
	v.SetDefault("metrics.path", "/metrics")
	v.SetDefault("metrics.port", 9000)
	v.SetDefault("clients.rbac_enabled", true)
	v.SetDefault("clients.rbac_base_url", "http://rbac-service:8000/api/rbac/v1")
	v.SetDefault("clients.rbac_timeout", 30)

	v.SetDefault("cloudwatch.region", "")
	v.SetDefault("cloudwatch.group", "")
	v.SetDefault("cloudwatch.stream", DefaultLogwatchStream())
	v.SetDefault("cloudwatch.session", "")
	v.SetDefault("cloudwatch.secret", "")
	v.SetDefault("cloudwatch.key", "")
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
