package config

import (
	"fmt"
	"io"
	"os"
	"time"

	sentrywriter "github.com/archdx/zerolog-sentry"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs"
	"github.com/getsentry/sentry-go"
	"github.com/labstack/echo/v4"
	cww "github.com/lzap/cloudwatchwriter2"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

const HeaderRequestId = "x-rh-insights-request-id" // the header that contains the request ID
const RequestIdLoggingKey = "request_id"           // the key that represents the request ID when logged

// Used in the context as the Key to store the Request ID
type ContextRequestIDKey struct{}

func ConfigureLogging() {
	var writers []io.Writer

	level, err := zerolog.ParseLevel(Get().Logging.Level)
	conf := Get()
	if err != nil {
		log.Error().Err(err).Msg("")
	}

	if conf.Logging.Console {
		writers = append(writers, zerolog.NewConsoleWriter())
	}

	if conf.Cloudwatch.Key != "" {
		cloudWatchLogger, err := newCloudWatchLogger(conf.Cloudwatch)
		if err != nil {
			log.Fatal().Err(err).Msg("ERROR setting up cloudwatch")
		}
		writers = append(writers, cloudWatchLogger)
	}
	if conf.Sentry.Dsn != "" {
		log.Info().Msg("Configuring Sentry")
		sWriter, err := sentryWriter(conf.Sentry.Dsn)
		if err != nil {
			log.Fatal().Err(err).Msg("Failed to initialize sentry, disabling sentry monitoring")
		} else {
			writers = append(writers, sWriter)
		}
	}
	log.Logger = zerolog.New(io.MultiWriter(writers...)).With().Timestamp().Logger()
	log.Logger = log.Logger.Level(level)
	zerolog.SetGlobalLevel(level)
	zerolog.DefaultContextLogger = &log.Logger
}

func newCloudWatchLogger(cwConfig Cloudwatch) (io.Writer, error) {
	log.Info().Msgf("Configuring Cloudwatch for group %s, stream %s", cwConfig.Group, cwConfig.Stream)
	cloudWatchWriter, err := cww.NewWithClient(newCloudWatchClient(cwConfig), 2000*time.Millisecond, cwConfig.Group, cwConfig.Stream)
	if err != nil {
		return log.Logger, fmt.Errorf("cloudwatchwriter.NewWithClient: %w", err)
	}

	return cloudWatchWriter, nil
}

func newCloudWatchClient(cwConfig Cloudwatch) *cloudwatchlogs.Client {
	cache := aws.NewCredentialsCache(credentials.NewStaticCredentialsProvider(
		Get().Cloudwatch.Key, cwConfig.Secret, cwConfig.Session))

	return cloudwatchlogs.New(cloudwatchlogs.Options{
		Region:      cwConfig.Region,
		Credentials: cache,
	})
}

func DefaultLogwatchStream() string {
	hostname, err := os.Hostname()
	if err != nil {
		log.Error().Err(err).Msg("Could not read hostname")
		return "content-sources-default"
	}
	return hostname
}

func SkipLogging(c echo.Context) bool {
	p := c.Request().URL.Path
	if p == "/ping" || p == "/ping/" || p == "/metrics" {
		return true
	}
	return false
}

// sentryWriter creates a zerolog writer for sentry.
// Uses github.com/archdx/zerolog-sentry which is very simple wrapper.
func sentryWriter(dsn string) (io.Writer, error) {
	wr, err := sentrywriter.New(dsn)
	if err != nil {
		return nil, fmt.Errorf("cannot initialize sentry: %w", err)
	}
	sentry.ConfigureScope(func(scope *sentry.Scope) {
		scope.SetTag("stream", ProgramString())
	})
	return wr, nil
}

type RequestIdHook struct{}

func (h RequestIdHook) Run(e *zerolog.Event, level zerolog.Level, msg string) {
	requestId, ok := e.GetCtx().Value(ContextRequestIDKey{}).(string)
	if ok {
		e.Str("requestId", requestId)
	}
}
