package config

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	cloudevents "github.com/cloudevents/sdk-go/v2"
	ce "github.com/content-services/content-sources-backend/pkg/errors"
	"github.com/content-services/content-sources-backend/pkg/kafka"
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
	Kafka               kafka.KafkaConfig
	Cloudwatch          Cloudwatch
	Metrics             Metrics
	Clients             Clients            `mapstructure:"clients"`
	Mocks               Mocks              `mapstructure:"mocks"`
	Sentry              Sentry             `mapstructure:"sentry"`
	NotificationsClient cloudevents.Client `mapstructure:"notification_client"`
	TemplateEventClient cloudevents.Client `mapstructure:"template_event_client"`
	Tasking             Tasking            `mapstructure:"tasking"`
	Features            FeatureSet         `mapstructure:"features"`
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

type FeatureSet struct {
	Snapshots              Feature
	AdminTasks             Feature `mapstructure:"admin_tasks"`
	NewRepositoryFiltering Feature `mapstructure:"new_repo_filtering"`
}

type Feature struct {
	Enabled       bool
	Accounts      *[]string // Only allow access if in the accounts list
	Organizations *[]string // Or org id is in the list
	Users         *[]string // or username in the users list
}

const STORAGE_TYPE_LOCAL = "local"
const STORAGE_TYPE_OBJECT = "object"

type Pulp struct {
	Server                  string
	Username                string
	Password                string
	StorageType             string       `mapstructure:"storage_type"` // s3 or local
	CustomRepoObjects       *ObjectStore `mapstructure:"custom_repo_objects"`
	DownloadPolicy          string       `mapstructure:"download_policy"`            // on_demand or immediate
	GuardSubjectDn          string       `mapstructure:"guard_subject_dn"`           // DN to allow access to via x509 identity subject_dn
	CustomRepoContentGuards bool         `mapstructure:"custom_repo_content_guards"` // To turn on or off the creation of content guards for custom repos
	Database                Database     `mapstructure:"database"`                   // for use with tangy
}

const RepoClowderBucketName = "content-sources-central-pulp-s3"

type ObjectStore struct {
	URL       string
	AccessKey string `mapstructure:"access_key"`
	SecretKey string `mapstructure:"secret_key"`
	Name      string
	Region    string
}

type Tasking struct {
	PGXLogging          bool `mapstructure:"pgx_logging"`
	Heartbeat           time.Duration
	WorkerCount         int           `mapstructure:"worker_count"`
	RetryWaitUpperBound time.Duration `mapstructure:"retry_wait_upper_bound"`
}

type Database struct {
	Host       string
	Port       int
	User       string
	Password   string
	Name       string
	CACertPath string `mapstructure:"ca_cert_path"`
	PoolLimit  int    `mapstructure:"pool_limit"`
}

type Logging struct {
	Level   string
	Console bool
}

type Certs struct {
	CertPath           string `mapstructure:"cert_path"`
	DaysTillExpiration int
	CdnCertPair        *tls.Certificate
	CdnCertPairString  *string
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
	Expiration Expiration `mapstructure:"pulp_content_path"`
}

type Expiration struct {
	Rbac            time.Duration
	PulpContentPath time.Duration
}

type Sentry struct {
	Dsn string
}

// https://stackoverflow.com/questions/54844546/how-to-unmarshal-golang-viper-snake-case-values
type Options struct {
	PagedRpmInsertsLimit      int `mapstructure:"paged_rpm_inserts_limit"`
	IntrospectApiTimeLimitSec int `mapstructure:"introspect_api_time_limit_sec"`
	// If true, introspection and snapshotting always runs for nightly job invocation, regardless of how soon they happened previously.  Used for testing.
	AlwaysRunCronTasks     bool   `mapstructure:"always_run_cron_tasks"`
	EnableNotifications    bool   `mapstructure:"enable_notifications"`
	TemplateEventTopic     string `mapstructure:"template_event_topic"`
	RepositoryImportFilter string `mapstructure:"repository_import_filter"` // Used by qe to control which repos are imported
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
	v.SetDefault("database.pool_limit", 20)
	v.SetDefault("certs.cert_path", "")
	v.SetDefault("options.paged_rpm_inserts_limit", DefaultPagedRpmInsertsLimit)
	v.SetDefault("options.introspect_api_time_limit_sec", DefaultIntrospectApiTimeLimitSec)
	v.SetDefault("options.always_run_cron_tasks", false)
	v.SetDefault("options.enable_notifications", false)
	v.SetDefault("options.template_event_topic", "platform.content-sources.template")
	v.SetDefault("options.repository_import_filter", "")
	v.SetDefault("logging.level", "info")
	v.SetDefault("logging.console", true)
	v.SetDefault("metrics.path", "/metrics")
	v.SetDefault("metrics.port", 9000)
	v.SetDefault("clients.rbac_enabled", true)
	v.SetDefault("clients.rbac_base_url", "http://rbac-service:8000/api/rbac/v1")
	v.SetDefault("clients.rbac_timeout", 30)
	v.SetDefault("clients.pulp.server", "")
	v.SetDefault("clients.pulp.download_policy", "immediate")
	v.SetDefault("clients.pulp.username", "")
	v.SetDefault("clients.pulp.password", "")
	v.SetDefault("clients.pulp.guard_subject_dn", "default-content-sources-dn") // Use a default, so we always create one
	v.SetDefault("clients.pulp.custom_repo_content_guards", false)
	v.SetDefault("clients.pulp.database.host", "")
	v.SetDefault("clients.pulp.database.port", 0)
	v.SetDefault("clients.pulp.database.user", "")
	v.SetDefault("clients.pulp.database.password", "")
	v.SetDefault("clients.pulp.database.name", "")
	v.SetDefault("sentry.dsn", "")

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
	v.SetDefault("clients.redis.expiration.rbac", 1*time.Minute)
	v.SetDefault("clients.redis.expiration.pulp_content_path", 1*time.Hour)

	v.SetDefault("tasking.heartbeat", 1*time.Minute)
	v.SetDefault("tasking.worker_count", 3)
	v.SetDefault("tasking.pgx_logging", true)
	v.SetDefault("tasking.retry_wait_upper_bound", time.Hour*12)

	v.SetDefault("features.snapshots.enabled", false)
	v.SetDefault("features.snapshots.accounts", nil)
	v.SetDefault("features.snapshots.organizations", nil)
	v.SetDefault("features.snapshots.users", nil)
	v.SetDefault("features.admin_tasks.enabled", false)
	v.SetDefault("features.admin_tasks.accounts", nil)
	v.SetDefault("features.admin_tasks.organizations", nil)
	v.SetDefault("features.admin_tasks.users", nil)
	v.SetDefault("features.new_repo_filtering.enabled", false)
	addEventConfigDefaults(v)
	addStorageDefaults(v)
}

func addStorageDefaults(v *viper.Viper) {
	v.SetDefault("clients.pulp.storage_type", "local")
	v.SetDefault("clients.pulp.custom_repo_objects.url", "")
	v.SetDefault("clients.pulp.custom_repo_objects.name", "")
	v.SetDefault("clients.pulp.custom_repo_objects.region", "")
	v.SetDefault("clients.pulp.custom_repo_objects.secret_key", "")
	v.SetDefault("clients.pulp.custom_repo_objects.access_key", "")
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

			bucket, ok := clowder.ObjectBuckets[RepoClowderBucketName]
			if !ok {
				log.Logger.Error().Msgf("Expected S3 Bucket named %v but not found", RepoClowderBucketName)
			} else {
				v.Set("clients.pulp.storage_type", "object")
				v.Set("clients.pulp.custom_repo_objects.url", ClowderS3Url(*clowder.LoadedConfig.ObjectStore))
				v.Set("clients.pulp.custom_repo_objects.name", bucket.Name)
				log.Logger.Warn().Msgf("Bucket name: %v", bucket.Name)
				if bucket.Region == nil || *bucket.Region == "" {
					// Minio doesn't use regions, but pulp requires a region name, its generally ignored
					v.Set("clients.pulp.custom_repo_objects.region", "DummyRegion")
				} else {
					v.Set("clients.pulp.custom_repo_objects.region", bucket.Region)
				}
				if bucket.SecretKey == nil || *bucket.SecretKey == "" {
					log.Error().Msg("Object store secret Key is empty or nil!")
				} else {
					v.Set("clients.pulp.custom_repo_objects.secret_key", *bucket.SecretKey)
				}
				if bucket.AccessKey == nil || *bucket.AccessKey == "" {
					log.Error().Msg("Object store Access Key is empty or nil!")
				} else {
					v.Set("clients.pulp.custom_repo_objects.access_key", bucket.AccessKey)
				}
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
	cert, certString, err := ConfigureCertificate()
	if err != nil {
		log.Fatal().Err(err).Msg("Could not read or parse cdn certificate.")
	}
	LoadedConfig.Certs.CdnCertPairString = certString
	LoadedConfig.Certs.CdnCertPair = cert
	LoadedConfig.Certs.DaysTillExpiration, err = DaysTillExpiration(cert)
	if err != nil {
		log.Error().Err(err).Msg("Could not calculate cert expiration date")
	}

	if LoadedConfig.Clients.Redis.Host == "" {
		log.Warn().Msg("Caching is disabled.")
	}
	if LoadedConfig.Clients.Pulp.Server == "" && LoadedConfig.Features.Snapshots.Enabled {
		log.Warn().Msg("Snapshots feature is turned on, but Pulp isn't configured, disabling snapshots.")
		LoadedConfig.Features.Snapshots.Enabled = false
	}
}

func ClowderS3Url(c clowder.ObjectStoreConfig) string {
	host := c.Hostname
	port := c.Port

	url, err := url.Parse(host)
	if err != nil {
		log.Error().Err(err).Msgf("Cannot parse object store hostname as url %v", host)
		return ""
	}

	var proto string
	if c.Tls {
		proto = "https"
	} else {
		proto = "http"
	}
	url.Scheme = proto

	return fmt.Sprintf("%v:%v", url.String(), port)
}

const RhCertEnv = "RH_CDN_CERT_PAIR"

// ConfigureCertificate loads in a cert keypair from either, an
// environment variable if specified, or a file path
// if no certificate is specified, we return no error
// however if a certificate is specified but cannot be loaded
// an error is returned.
func ConfigureCertificate() (*tls.Certificate, *string, error) {
	var (
		err       error
		certBytes []byte
	)

	if certString := os.Getenv(RhCertEnv); certString != "" {
		certBytes = []byte(certString)
	} else if Get().Certs.CertPath != "" {
		certBytes, err = os.ReadFile(Get().Certs.CertPath)
		if err != nil {
			return nil, nil, err
		}
	} else {
		log.Warn().Msg("No Red Hat CDN cert pair configured.")
		return nil, nil, nil
	}
	cert, err := tls.X509KeyPair(certBytes, certBytes)
	if err != nil {
		return nil, nil, err
	}
	certString := string(certBytes)

	return &cert, &certString, nil
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

func PulpConfigured() bool {
	return Get().Clients.Pulp.Server != ""
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
		log.Error().Msg(err.Error())
	}
}
