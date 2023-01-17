package config

import (
	"fmt"
	"io"
	"os"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs"
	"github.com/labstack/echo/v4"
	cww "github.com/lzap/cloudwatchwriter2"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

func ConfigureLogging() {
	level, err := zerolog.ParseLevel(Get().Logging.Level)
	conf := Get()
	if err != nil {
		log.Error().Err(err).Msg("")
	}

	if conf.Logging.Console {
		log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr})
	}
	if conf.Cloudwatch.Key != "" {
		cloudWatchLogger, err := newCloudWatchLogger(conf.Cloudwatch)
		if err != nil {
			log.Fatal().Err(err).Msg("ERROR setting up cloudwatch")
		}
		log.Logger = zerolog.New(zerolog.MultiLevelWriter(log.Logger, cloudWatchLogger)).With().Timestamp().Logger()
		log.Logger = log.Logger.Level(level)
	}

	zerolog.SetGlobalLevel(zerolog.InfoLevel)
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
