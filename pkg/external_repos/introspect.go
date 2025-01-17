package external_repos

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/RedHatInsights/event-schemas-go/apps/repositories/v1"
	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/content-services/content-sources-backend/pkg/dao"
	"github.com/content-services/content-sources-backend/pkg/db"
	"github.com/content-services/content-sources-backend/pkg/event"
	"github.com/content-services/content-sources-backend/pkg/utils"
	"github.com/content-services/yummy/pkg/yum"
	"github.com/rs/zerolog"
)

const (
	RhCdnHost = "cdn.redhat.com"
)

// IntrospectUrl Fetch the metadata of a url and insert RPM data
// Returns the number of new RPMs inserted system-wide, any introspection errors,
// and any fatal errors
func IntrospectUrl(ctx context.Context, url string) (int64, error, error) {
	var (
		total                  int64
		count                  int64
		err                    error
		dao                    = dao.GetDaoRegistry(db.DB)
		introspectionError     error
		introspectFailedUuids  []string
		introspectSuccessUuids []string
		updated                bool
	)

	repo, err := dao.Repository.FetchForUrl(ctx, url)
	if err != nil {
		return total, introspectionError, err
	}
	count, err, updated = Introspect(ctx, &repo, dao)

	if err != nil {
		introspectionError = fmt.Errorf("error introspecting %s: %s", repo.URL, err.Error())
		introspectFailedUuids = append(introspectFailedUuids, repo.UUID)
	} else if updated {
		introspectSuccessUuids = append(introspectSuccessUuids, repo.UUID)
	}

	err = UpdateIntrospectionStatusMetadata(ctx, repo, dao, count, err)

	// Logic to handle notifications.  This should really be moved to a daily report?
	sendIntrospectionNotifications(ctx, introspectSuccessUuids, introspectFailedUuids, dao)

	return total, introspectionError, err
}

// IsRedHat returns if the url is a 'cdn.redhat.com' url
func IsRedHat(url string) bool {
	return strings.Contains(url, RhCdnHost)
}

// Introspect introspects a dao.Repository with the given Rpm
// inserting any needed RPMs and adding and removing associations to the repository
// Returns the number of new RPMs inserted system-wide and any error encountered
func Introspect(ctx context.Context, repo *dao.Repository, dao *dao.DaoRegistry) (int64, error, bool) {
	var (
		client        http.Client
		err           error
		total         int64
		repomd        *yum.Repomd
		packages      []yum.Package
		modMds        []yum.ModuleMD
		packageGroups []yum.PackageGroup
		environments  []yum.Environment
	)
	logger := zerolog.Ctx(ctx)

	if repo.FailedIntrospectionsCount == config.FailedIntrospectionsLimit {
		logger.Debug().Msgf("introspection count reached for %v", repo.URL)
		return 0, fmt.Errorf("introspection skipped because this repository has failed more than %v times in a row", config.FailedIntrospectionsLimit), false
	}

	logger.Debug().Msg("Introspecting " + repo.URL)

	if client, err = httpClient(IsRedHat(repo.URL)); err != nil {
		return 0, err, false
	}
	settings := yum.YummySettings{
		Client: &client,
		URL:    &repo.URL,
	}
	yumRepo, _ := yum.NewRepository(settings)

	if repomd, _, err = yumRepo.Repomd(ctx); err != nil {
		return 0, err, false
	}

	checksumStr := ""
	if repomd.RepomdString != nil && *repomd.RepomdString != "" {
		sum := sha256.Sum256([]byte(*repomd.RepomdString))
		checksumStr = hex.EncodeToString(sum[:])
	}

	if repo.RepomdChecksum != "" && checksumStr != "" && checksumStr == repo.RepomdChecksum {
		// If repository hasn't changed, no need to update
		return 0, nil, false
	}

	if packages, _, err = yumRepo.Packages(ctx); err != nil {
		return 0, err, false
	}

	if total, err = dao.Rpm.InsertForRepository(ctx, repo.UUID, packages); err != nil {
		return 0, err, false
	}

	var foundCount int
	if foundCount, err = dao.Repository.FetchRepositoryRPMCount(ctx, repo.UUID); err != nil {
		return 0, err, false
	}

	if packageGroups, _, err = yumRepo.PackageGroups(ctx); err != nil {
		return 0, err, false
	}
	if _, err = dao.PackageGroup.InsertForRepository(ctx, repo.UUID, packageGroups); err != nil {
		return 0, err, false
	}

	if environments, _, err = yumRepo.Environments(ctx); err != nil {
		return 0, err, false
	}
	if _, err = dao.Environment.InsertForRepository(ctx, repo.UUID, environments); err != nil {
		return 0, err, false
	}

	if modMds, _, err = yumRepo.ModuleMDs(ctx); err != nil {
		return 0, err, false
	}
	if _, err = dao.ModuleStream.InsertForRepository(ctx, repo.UUID, modMds); err != nil {
		return 0, err, false
	}

	repo.RepomdChecksum = checksumStr
	repo.PackageCount = foundCount
	if err = dao.Repository.Update(ctx, RepoToRepoUpdate(*repo)); err != nil {
		return 0, err, false
	}

	return total, nil, true
}

func sendIntrospectionNotifications(ctx context.Context, successUuids []string, failedUuids []string, dao *dao.DaoRegistry) {
	count := 0
	wg := sync.WaitGroup{}

	if len(successUuids) != 0 {
		for i := 0; i < len(successUuids); i++ {
			uuid := successUuids[i]
			repos := dao.RepositoryConfig.InternalOnly_FetchRepoConfigsForRepoUUID(ctx, uuid)
			for j := 0; j < len(repos); j++ {
				wg.Add(1)
				count = count + 1
				go func(index int) {
					event.SendNotification(
						repos[index].OrgID,
						event.RepositoryIntrospected,
						[]repositories.Repositories{event.MapRepositoryResponse(repos[index])},
					)
					wg.Done()
				}(j)
				if count > 100 { // This limits the thread count
					count = 0
					wg.Wait()
				}
			}
		}
		// Reset count
		count = 0
		wg.Wait()
	}

	for i := 0; i < len(failedUuids); i++ {
		uuid := failedUuids[i]
		repos := dao.RepositoryConfig.InternalOnly_FetchRepoConfigsForRepoUUID(ctx, uuid)
		for j := 0; j < len(repos); j++ {
			wg.Add(1)
			count = count + 1
			go func(index int) {
				event.SendNotification(
					repos[index].OrgID,
					event.RepositoryIntrospectionFailure,
					[]repositories.Repositories{event.MapRepositoryResponse(repos[index])},
				)
				wg.Done()
			}(j)
			if count > 100 { // This limits the thread count
				count = 0
				wg.Wait()
			}
		}
	}

	wg.Wait()
}

func httpClient(useCert bool) (http.Client, error) {
	timeout := 90 * time.Second
	if useCert {
		var (
			cert   *tls.Certificate
			caCert []byte
			err    error
		)

		cert = config.Get().Certs.CdnCertPair
		if cert == nil {
			return http.Client{}, errors.New("no certificate loaded")
		}
		if caCert, err = LoadCA(); err != nil {
			return http.Client{}, err
		}
		caCertPool := x509.NewCertPool()
		caCertPool.AppendCertsFromPEM(caCert)

		tlsConfig := &tls.Config{
			Certificates: []tls.Certificate{*cert},
			RootCAs:      caCertPool,
			MinVersion:   tls.VersionTLS12,
		}

		transport := &http.Transport{TLSClientConfig: tlsConfig, ResponseHeaderTimeout: timeout}
		return http.Client{Transport: transport, Timeout: timeout}, nil
	} else {
		return http.Client{}, nil
	}
}

// UpdateIntrospectionStatusMetadata updates introspection timestamps, error, and status on repo. Use after calling Introspect().
func UpdateIntrospectionStatusMetadata(ctx context.Context, repo dao.Repository, dao *dao.DaoRegistry, count int64, err error) error {
	introspectTimeEnd := time.Now()
	repoUpdate := updateIntrospectionStatusMetadata(repo, count, err, &introspectTimeEnd)

	if err := dao.Repository.Update(ctx, repoUpdate); err != nil {
		return fmt.Errorf("failed to update introspection timestamps: %w", err)
	}

	return nil
}

func updateIntrospectionStatusMetadata(input dao.Repository, count int64, err error, introspectTimeEnd *time.Time) dao.RepositoryUpdate {
	output := input
	output.LastIntrospectionTime = introspectTimeEnd

	// If introspection was successful
	if err == nil {
		if count != 0 {
			output.LastIntrospectionUpdateTime = introspectTimeEnd
		}
		output.LastIntrospectionSuccessTime = introspectTimeEnd
		output.LastIntrospectionError = utils.Ptr("")
		output.LastIntrospectionStatus = config.StatusValid
		output.FailedIntrospectionsCount = 0
		return RepoToRepoUpdate(output)
	}

	// If introspection fails
	output.LastIntrospectionError = utils.Ptr(err.Error())
	output.FailedIntrospectionsCount += 1
	switch input.LastIntrospectionStatus {
	case config.StatusValid:
		output.LastIntrospectionStatus = config.StatusUnavailable // Repository introspected successfully at least once, but now errors
	case config.StatusInvalid:
		output.LastIntrospectionStatus = config.StatusInvalid
	case config.StatusPending:
		if output.LastIntrospectionSuccessTime == nil {
			output.LastIntrospectionStatus = config.StatusInvalid
		} else {
			output.LastIntrospectionStatus = config.StatusUnavailable
		}
	case config.StatusUnavailable:
	default:
		output.LastIntrospectionStatus = config.StatusUnavailable
	}

	return RepoToRepoUpdate(output)
}

func RepoToRepoUpdate(repo dao.Repository) dao.RepositoryUpdate {
	return dao.RepositoryUpdate{
		UUID:                         repo.UUID,
		URL:                          &repo.URL,
		RepomdChecksum:               &repo.RepomdChecksum,
		LastIntrospectionTime:        repo.LastIntrospectionTime,
		LastIntrospectionSuccessTime: repo.LastIntrospectionSuccessTime,
		LastIntrospectionUpdateTime:  repo.LastIntrospectionUpdateTime,
		LastIntrospectionError:       repo.LastIntrospectionError,
		LastIntrospectionStatus:      &repo.LastIntrospectionStatus,
		PackageCount:                 &repo.PackageCount,
		FailedIntrospectionsCount:    &repo.FailedIntrospectionsCount,
	}
}
