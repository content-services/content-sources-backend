package config

import (
	"crypto/tls"
	"os"
	"strings"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	clowder "github.com/redhatinsights/app-common-go/pkg/api/v1"
	pgm "github.com/redhatinsights/platform-go-middlewares"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/spf13/viper"
	"github.com/ziflex/lecho/v3"
)

type Configuration struct {
	Database Database
	Logging  Logging
	Loaded   bool
	Certs    Certs
	Options  Options
}

type Database struct {
	Host     string
	Port     int
	User     string
	Password string
	Name     string
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
	v.SetConfigName("config")
	v.SetConfigType("yaml")
	v.AddConfigPath("./configs/")

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
}

func Load() {
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
	}

	err := v.Unmarshal(&LoadedConfig)
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
	log.Logger.Level(level)
	zerolog.SetGlobalLevel(level)
	zerolog.DefaultContextLogger = &log.Logger
}

func ConfigureEcho() *echo.Echo {
	e := echo.New()
	echoLogger := lecho.From(log.Logger,
		lecho.WithTimestamp(),
		lecho.WithCaller(),
	)

	e.Use(middleware.RequestID())
	e.Use(lecho.Middleware(lecho.Config{
		Logger: echoLogger,
	}))
	e.Use(echo.WrapMiddleware(pgm.EnforceIdentity))
	return e
}
