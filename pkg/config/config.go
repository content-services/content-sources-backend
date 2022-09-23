package config

import (
	"crypto/tls"
	"net/http"
	"os"
	"strings"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	clowder "github.com/redhatinsights/app-common-go/pkg/api/v1"
	identity "github.com/redhatinsights/platform-go-middlewares/identity"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/spf13/viper"
	"github.com/ziflex/lecho/v3"
)

const DefaultAppName = "content-sources"

type Configuration struct {
	Database Database
	Logging  Logging
	Loaded   bool
	Certs    Certs
	Options  Options
	Kafka KafkaConfig
}

// TODO ADD KafkaConfig
type KafkaConfig struct {
	Timeout int
	Group   struct {
		Id string
	}
	Auto struct {
		Offset struct {
			Reset string
		}
		Commit struct {
			Interval struct {
				Ms int
			}
		}
	}
	Bootstrap struct {
		Servers string
	}
	Topics []string
	Sasl   struct {
		Username string
		Password string
		Mechnism string
		Protocol string
	}
	Request struct {
		Timeout struct {
			Ms int
		}
		Required struct {
			Acks int
		}
	}
	Capath  string
	Message struct {
		Send struct {
			Max struct {
				Retries int
			}
		}
	}
	Retry struct {
		Backoff struct {
			Ms int
		}
	}
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

// https://stackoverflow.com/questions/54844546/how-to-unmarshal-golang-viper-snake-case-values
type Options struct {
	PagedRpmInsertsLimit int `mapstructure:"paged_rpm_inserts_limit"`
}

const (
	DefaultPagedRpmInsertsLimit = 200
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

	path, set := os.LookupEnv("CONFIG_PATH")
	if set {
		v.AddConfigPath(path)
	}
	err := v.ReadInConfig()
	if err != nil {
		log.Logger.Err(err).Msg("")
	}
}

func setDefaults(v *viper.Viper) {
	v.SetDefault("Loaded", true)
	//In viper you have to set defaults, otherwise loading from ENV doesn't work
	//   without a config file present
	v.SetDefault("database.host", "")
	v.SetDefault("database.port", "")
	v.SetDefault("database.user", "")
	v.SetDefault("database.password", "")
	v.SetDefault("database.name", "")
	v.SetDefault("certs.cert_path", "")
	v.SetDefault("options.paged_rpm_inserts_limit", DefaultPagedRpmInsertsLimit)

	AddEventConfigDefaults(v)
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
		if clowder.LoadedConfig != nil {
			path, err := clowder.LoadedConfig.RdsCa()
			if err == nil {
				v.Set("database.ca_cert_path", path)
			} else {
				log.Error().Err(err).Msg("Cannot read RDS CA cert")
			}
		}
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
// 	environment variable if specified, or a file path
//  if no certificate is specified, we return no error
//  however if a certificate is specified but cannot be loaded
//  an error is returned.
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

func ConfigureLogging() {
	level, err := zerolog.ParseLevel(Get().Logging.Level)
	if err != nil {
		log.Error().Err(err).Msg("")
	}

	if Get().Logging.Console {
		log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr})
	}
	log.Logger = log.Logger.Level(level)
	zerolog.SetGlobalLevel(level)
	zerolog.DefaultContextLogger = &log.Logger
}

// WrapMiddleware wraps `func(http.Handler) http.Handler` into `echo.MiddlewareFunc`
func WrapMiddlewareWithSkipper(m func(http.Handler) http.Handler, skip middleware.Skipper) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) (err error) {
			if skip != nil && skip(c) {
				err = next(c)
				return
			}
			m(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				c.SetRequest(r)
				c.SetResponse(echo.NewResponse(w, c.Echo()))
				identityHeader := c.Request().Header.Get("X-Rh-Identity")
				if identityHeader != "" {
					c.Response().Header().Set("X-Rh-Identity", identityHeader)
				}
				err = next(c)
			})).ServeHTTP(c.Response(), c.Request())
			return
		}
	}
}

func SkipLiveness(c echo.Context) bool {
	p := c.Request().URL.Path
	if p == "/ping" {
		return true
	}
	if strings.HasPrefix(p, "/api/"+DefaultAppName+"/") &&
		len(strings.Split(p, "/")) == 5 &&
		strings.Split(p, "/")[4] == "ping" {
		return true
	}
	return false
}

func ConfigureEcho() *echo.Echo {
	e := echo.New()
	echoLogger := lecho.From(log.Logger,
		lecho.WithTimestamp(),
		lecho.WithCaller(),
	)
	e.Use(middleware.RequestIDWithConfig(middleware.RequestIDConfig{
		TargetHeader: "x-rh-insights-request-id",
	}))
	e.Use(WrapMiddlewareWithSkipper(identity.EnforceIdentity, SkipLiveness))
	e.Use(lecho.Middleware(lecho.Config{
		Logger:       echoLogger,
		RequestIDKey: "x-rh-insights-request-id",
	}))

	return e
}
