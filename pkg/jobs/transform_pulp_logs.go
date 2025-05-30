package jobs

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/csv"
	"fmt"
	"os"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsConfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs/types"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/cloudevents/sdk-go/v2/event/datacodec/json"
	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/content-services/content-sources-backend/pkg/dao"
	"github.com/content-services/content-sources-backend/pkg/db"
	"github.com/rs/zerolog/log"
)

var LogFilter = "{($.kubernetes.labels.pod = pulp-content) && ($.message = %pulp-content% )}"

var ignoreDomainList = []string{"default", "rhel-ai"}

type LogMessage struct {
	Message string `json:"message"`
}

type PulpLogEvent struct {
	Timestamp    int64
	Path         string
	FileSize     string
	OrgId        string
	RequestOrgId string
	UserAgent    string
	DomainName   string
}

type TransformPulpLogsJob struct {
	domainMap map[string]string
	ctx       context.Context
	re        *regexp.Regexp
}

// Pulls pulp logs from cloudwatch, parses the log data, pulls out information we care about,
// transforms it into a csv and uploads it to s3
func TransformPulpLogs() {
	var err error
	job := TransformPulpLogsJob{ctx: context.Background()}
	job.domainMap, err = domainMap(job.ctx)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to fetch domain map")
		os.Exit(-1)
	}

	// Set the time window for log retrieval (23:59:59 of yesterday, - 24 hours)
	now := time.Now().UTC()
	midnight := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	midnight = midnight.Add(-1 * time.Second)
	startTime := midnight.Add(-24 * time.Hour).UnixMilli()
	endTime := midnight.UnixMilli()

	// Step 1: Get logs from CloudWatch, transform into PulpLogEvents
	events, err := job.getLogEvents(startTime, endTime)
	if err != nil {
		log.Error().Err(err).Msg("failed to get log events")
	}
	log.Info().Msgf("Parsed %v log events", len(events))

	// Gzip the PulpLogEvents
	gzipFile, err := convertToCsv(events)
	if err != nil {
		log.Err(err).Msgf("failed to compress log events")
		os.Exit(-1)
	}

	// Upload to s3
	err = job.uploadGzipToS3(gzipFile, midnight)
	if err != nil {
		log.Err(err).Msgf("failed to upload log events to S3")
		os.Exit(-1)
	}
}

func checkCloudwatchConfig(cw config.Cloudwatch) {
	if cw.Region == "" {
		log.Error().Msg("Cloudwatch region is empty")
	} else if cw.Group == "" {
		log.Error().Msg("Cloudwatch group is empty")
	} else if cw.Key == "" {
		log.Error().Msg("Cloudwatch key is empty")
	} else if cw.Secret == "" {
		log.Error().Msg("Cloudwatch secret is empty")
	}
}

// Gets logs from Cloudwatch and transform into PulpLogEvents
func (t TransformPulpLogsJob) getLogEvents(startTime, endTime int64) (pulpEvents []PulpLogEvent, err error) {
	cfg := config.Get().Clients.PulpLogParser.Cloudwatch
	checkCloudwatchConfig(cfg)

	clientOptions := cloudwatchlogs.Options{
		Region:      cfg.Region,
		Credentials: credentials.NewStaticCredentialsProvider(cfg.Key, cfg.Secret, ""),
	}
	cwClient := cloudwatchlogs.New(clientOptions)

	// Call CloudWatch Logs to filter log events
	params := &cloudwatchlogs.FilterLogEventsInput{
		LogGroupName:  aws.String(cfg.Group),
		StartTime:     aws.Int64(startTime),
		EndTime:       aws.Int64(endTime),
		FilterPattern: &LogFilter,
	}

	resp, err := cwClient.FilterLogEvents(t.ctx, params)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch logs: %w", err)
	}
	pulpEvents = append(pulpEvents, t.transformLogs(resp.Events)...)
	for resp.NextToken != nil {
		params.NextToken = resp.NextToken
		resp, err = cwClient.FilterLogEvents(t.ctx, params)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch logs: %w", err)
		}
		pulpEvents = append(pulpEvents, t.transformLogs(resp.Events)...)
	}

	return pulpEvents, nil
}

func convertToCsv(logs []PulpLogEvent) (compressedData *bytes.Buffer, err error) {
	compressedData = &bytes.Buffer{}
	// Compress the log data using Gzip
	gzipWriter := gzip.NewWriter(compressedData)
	csvWriter := csv.NewWriter(gzipWriter)

	for i := 0; i < len(logs); i++ {
		event := logs[i]
		err = csvWriter.Write([]string{strconv.FormatInt(event.Timestamp, 10), event.RequestOrgId, event.OrgId, event.DomainName, event.Path, event.UserAgent, event.FileSize})
		if err != nil {
			return compressedData, fmt.Errorf("failed to write log event: %w", err)
		}
	}
	csvWriter.Flush()
	err = gzipWriter.Close()
	if err != nil {
		return compressedData, fmt.Errorf("failed to close gzip writer: %w", err)
	}
	return compressedData, nil
}

func (t TransformPulpLogsJob) uploadGzipToS3(compressedData *bytes.Buffer, date time.Time) (err error) {
	cfg := config.Get().Clients.PulpLogParser.S3
	if cfg.Name == "" {
		log.Warn().Msg("Not configured to upload to S3")
		return nil
	}

	// Define S3 object key (file name), date of the logs with current unix time for uniqness
	s3Key := fmt.Sprintf("%s/%s-%v.csv.gz", cfg.FilePrefix, date.Format("2006-01-02"), time.Now().Unix())

	awsCfg, err := awsConfig.LoadDefaultConfig(context.Background(), awsConfig.WithRegion(cfg.Region),
		awsConfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(cfg.AccessKey, cfg.SecretKey, "")))
	if err != nil {
		return fmt.Errorf("unable to load SDK config, %v", err)
	}
	s3Client := s3.NewFromConfig(awsCfg)

	_, err = s3Client.PutObject(t.ctx, &s3.PutObjectInput{
		Bucket: &cfg.Name,
		Key:    &s3Key,
		Body:   compressedData,
	})
	if err != nil {
		return fmt.Errorf("failed to upload logs to S3: %w", err)
	}
	log.Info().Msgf("Uploaded %v to s3", s3Key)
	return nil
}

// Returns a mapping of domain name to orgId
func domainMap(ctx context.Context) (domainMap map[string]string, err error) {
	err = db.Connect()
	domainMap = make(map[string]string)
	if err != nil {
		return domainMap, err
	}
	daoReg := dao.GetDaoRegistry(db.DB)
	domainList, err := daoReg.Domain.List(ctx)
	if err != nil {
		return domainMap, err
	}
	for _, domain := range domainList {
		domainMap[domain.DomainName] = domain.OrgId
	}
	return domainMap, nil
}

// Parses a pulp log string into a PulpLogEvent
// Example
// 192.168.1.1 [27/Jan/2025:20:44:09 +0000] "GET /api/pulp-content/rhel-ai/gaudi-rhel-9.4/repodata/path/primary.xml.gz HTTP/1.0" 302 791 "-" "libdnf (Red Hat Enterprise Linux 9.4; generic; Linux.x86_64)" "MISS" "21547" "939458934"
// IP [TIMESTAMP] "METHOD PATH HTTPVER" STATUS RESP_SIZE "-" "USER_AGENT" "CACHE_STATUS" "RPM_SIZE" "REQUEST_ORG_ID"
// Uses ideas from https://clavinjune.dev/en/blogs/create-log-parser-using-go/
func (t TransformPulpLogsJob) parsePulpLogMessage(logMsg string) *PulpLogEvent {
	event := PulpLogEvent{}
	if t.re == nil {
		logsFormat := `$_ \[$timestamp\] \"$http_method $request_path $_\" $response_code $_ \"$_\" \"$user_agent\" \"$_\" \"$rpm_size\" \"$request_org_id\"`
		regexFormat := regexp.MustCompile(`\$([\w_]*)`).ReplaceAllString(logsFormat, `(?P<$1>.*)`)
		t.re = regexp.MustCompile(regexFormat)
	}

	matches := t.re.FindStringSubmatch(logMsg)
	if matches == nil {
		log.Error().Str("log_event", logMsg).Msgf("Log event does not match expected regular expression, has the pulp log format changed?")
		return nil
	}
	for i, k := range t.re.SubexpNames() {
		// ignore the first and the $_
		if i == 0 || k == "_" {
			continue
		}
		switch item := k; item {
		case "request_path":
			event.Path = matches[i]
		case "rpm_size":
			event.FileSize = matches[i]
		case "user_agent":
			event.UserAgent = matches[i]
		case "request_org_id":
			event.RequestOrgId = matches[i]
		case "timestamp":
			event.Timestamp = parseTimestamp(matches[i])
		}
	}

	domainName := domainNameFromPath(event.Path)
	if domainName == nil {
		return nil
	} else {
		event.DomainName = *domainName
	}

	if slices.Contains(ignoreDomainList, *domainName) {
		return nil
	}

	if event.OrgId = t.domainMap[*domainName]; event.OrgId == "" {
		log.Warn().Msgf("Unknown domain %v", event.DomainName)
		return nil
	}

	return &event
}

// 27/Jan/2025:20:44:09 +0000
func parseTimestamp(ts string) int64 {
	layout := "02/Jan/2006:15:04:05 +0000"
	t, err := time.Parse(layout, ts)
	if err != nil {
		log.Error().Err(err).Msgf("Failed to parse timestamp %v", ts)
		return 0
	}
	return t.Unix()
}

// Extracts the domain name form a url
// For example /api/pulp-content/abcde/gaudi-rhel-9.4/repodata/path/primary.xml.gz
// would return abcde
func domainNameFromPath(path string) *string {
	splitPath := strings.Split(path, "/")
	if len(splitPath) < 4 || splitPath[1] != "api" || splitPath[2] != "pulp-content" {
		log.Warn().Msgf("Unexpected pulp-content path: %v", path)
		return nil
	} else {
		return &splitPath[3]
	}
}

// Converts FilteredLogEvents to PulpLogEvents
func (t TransformPulpLogsJob) transformLogs(events []types.FilteredLogEvent) (pulpEvents []PulpLogEvent) {
	for _, logEvent := range events {
		pulpEvent := t.transformLogToEvent(logEvent)
		if pulpEvent != nil {
			pulpEvents = append(pulpEvents, *pulpEvent)
		}
	}
	return pulpEvents
}

// Converts a single FilteredLogEvent to PulpLogEvent
func (t TransformPulpLogsJob) transformLogToEvent(event types.FilteredLogEvent) *PulpLogEvent {
	logMsg := LogMessage{}
	outerMsg := event.Message
	if outerMsg == nil {
		return nil
	}
	err := json.Decode(t.ctx, []byte(*outerMsg), &logMsg)
	if err != nil {
		log.Error().Err(err).Msgf("failed to decode event %v", event.Timestamp)
		return nil
	}
	return t.parsePulpLogMessage(logMsg.Message)
}
