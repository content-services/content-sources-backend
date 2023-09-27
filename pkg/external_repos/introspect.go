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
	"github.com/content-services/content-sources-backend/pkg/notifications"
	"github.com/content-services/yummy/pkg/yum"
	"github.com/openlyinc/pointy"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

const (
	RhCdnHost              = "cdn.redhat.com"
	IntrospectTimeInterval = time.Hour * 23
)

// IntrospectUrl Fetch the metadata of a url and insert RPM data
// Returns the number of new RPMs inserted system-wide, any introspection errors,
// and any fatal errors
func IntrospectUrl(ctx context.Context, url string, force bool) (int64, []error, []error) {
	urls := []string{url}
	return IntrospectAll(ctx, &urls, force)
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
		client   http.Client
		err      error
		total    int64
		repomd   *yum.Repomd
		packages []yum.Package
	)
	logger := zerolog.Ctx(ctx)

	logger.Debug().Msg("Introspecting " + repo.URL)

	if repo.FailedIntrospectionsCount >= config.FailedIntrospectionsLimit && !repo.Public {
		return 0, fmt.Errorf("introspection skipped because this repository has failed more than %v times in a row", config.FailedIntrospectionsLimit), false
	}

	if client, err = httpClient(IsRedHat(repo.URL)); err != nil {
		return 0, err, false
	}
	settings := yum.YummySettings{
		Client: &client,
		URL:    &repo.URL,
	}
	yumRepo, _ := yum.NewRepository(settings)

	if repomd, _, err = yumRepo.Repomd(); err != nil {
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

	if packages, _, err = yumRepo.Packages(); err != nil {
		return 0, err, false
	}

	if total, err = dao.Rpm.InsertForRepository(repo.UUID, packages); err != nil {
		return 0, err, false
	}

	var foundCount int
	if foundCount, err = dao.Repository.FetchRepositoryRPMCount(repo.UUID); err != nil {
		return 0, err, false
	}

	repo.RepomdChecksum = checksumStr
	repo.PackageCount = foundCount
	if err = dao.Repository.Update(RepoToRepoUpdate(*repo)); err != nil {
		return 0, err, false
	}

	return total, nil, true
}

func reposForIntrospection(urls *[]string, force bool) ([]dao.Repository, []error) {
	repoDao := dao.GetRepositoryDao(db.DB)
	ignoredFailed := !force // when forcing introspection, include repositories over FailedIntrospectionsLimit
	if urls != nil {
		var repos []dao.Repository
		var errors []error
		for i := 0; i < len(*urls); i++ {
			repo, err := repoDao.FetchForUrl((*urls)[i])
			if err != nil {
				errors = append(errors, err)
			} else if ignoredFailed && repo.FailedIntrospectionsCount > config.FailedIntrospectionsLimit {
				continue
			} else {
				repos = append(repos, repo)
			}
		}
		return repos, errors
	} else {
		repos, err := repoDao.List(ignoredFailed)
		return repos, []error{err}
	}
}

// IntrospectAll introspects all repositories
// Returns the number of new RPMs inserted system-wide, all non-fatal introspection errors,
// and separately all other fatal errors
func IntrospectAll(ctx context.Context, urls *[]string, force bool) (int64, []error, []error) {
	var (
		total                  int64
		count                  int64
		err                    error
		dao                    = dao.GetDaoRegistry(db.DB)
		introspectionErrors    []error
		introspectFailedUuids  []string
		introspectSuccessUuids []string
		updated                bool
	)
	repos, errors := reposForIntrospection(urls, force)
	for i := 0; i < len(repos); i++ {
		if !force {
			hasToIntrospect, reason := needsIntrospect(&repos[i])
			log.Info().Msg(reason)
			if !hasToIntrospect {
				continue
			}
		} else {
			log.Info().Msgf("Forcing introspection for '%s'", repos[i].URL)
		}
		count, err, updated = Introspect(ctx, &repos[i], dao)
		total += count

		if err != nil {
			introspectionErrors = append(introspectionErrors, fmt.Errorf("Error introspecting %s: %s", repos[i].URL, err.Error()))
			introspectFailedUuids = append(introspectFailedUuids, repos[i].UUID)
		} else if updated {
			introspectSuccessUuids = append(introspectSuccessUuids, repos[i].UUID)
		}

		err = UpdateIntrospectionStatusMetadata(repos[i], dao, count, err)
		if err != nil {
			errors = append(errors, err)
		}
	}

	// Logic to handle notifications
	sendIntrospectionNotifications(introspectSuccessUuids, introspectFailedUuids, dao)

	return total, introspectionErrors, errors
}

func sendIntrospectionNotifications(successUuids []string, failedUuids []string, dao *dao.DaoRegistry) {
	count := 0
	wg := sync.WaitGroup{}

	if len(successUuids) != 0 {
		for i := 0; i < len(successUuids); i++ {
			uuid := successUuids[i]
			repos := dao.RepositoryConfig.InternalOnly_FetchRepoConfigsForRepoUUID(uuid)
			for j := 0; j < len(repos); j++ {
				wg.Add(1)
				count = count + 1
				go func(index int) {
					notifications.SendNotification(
						repos[index].OrgID,
						notifications.RepositoryIntrospected,
						[]repositories.Repositories{notifications.MapRepositoryResponse(repos[index])},
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
		repos := dao.RepositoryConfig.InternalOnly_FetchRepoConfigsForRepoUUID(uuid)
		for j := 0; j < len(repos); j++ {
			wg.Add(1)
			count = count + 1
			go func(index int) {
				notifications.SendNotification(
					repos[index].OrgID,
					notifications.RepositoryIntrospectionFailure,
					[]repositories.Repositories{notifications.MapRepositoryResponse(repos[index])},
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

func needsIntrospect(repo *dao.Repository) (bool, string) {
	if repo == nil {
		return false, "Cannot introspect nil Repository"
	}

	if config.Get().Options.AlwaysRunCronTasks {
		return true, "Introspection started: AlwaysRunCronTasks is true"
	}

	if repo.Status != config.StatusValid {
		return true, fmt.Sprintf("Introspection started: the Status field content differs from '%s' for Repository.UUID = %s", config.StatusValid, repo.UUID)
	}

	if repo.LastIntrospectionTime == nil {
		return true, fmt.Sprintf("Introspection started: not expected LastIntrospectionTime = nil for Repository.UUID = %s", repo.UUID)
	}

	threshold := repo.LastIntrospectionTime.Add(IntrospectTimeInterval)
	if threshold.After(time.Now()) {
		return false, fmt.Sprintf("Introspection skipped: Last instrospection happened before the threshold for Repository.UUID = %s", repo.UUID)
	}

	return true, fmt.Sprintf("Introspection started: last introspection happened after the threshold for Repository.UUID = %s", repo.UUID)
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
		}

		transport := &http.Transport{TLSClientConfig: tlsConfig, ResponseHeaderTimeout: timeout}
		return http.Client{Transport: transport, Timeout: timeout}, nil
	} else {
		return http.Client{}, nil
	}
}

// UpdateIntrospectionStatusMetadata updates introspection timestamps, error, and status on repo. Use after calling Introspect().
func UpdateIntrospectionStatusMetadata(repo dao.Repository, dao *dao.DaoRegistry, count int64, err error) error {
	introspectTimeEnd := time.Now()
	repoUpdate := updateIntrospectionStatusMetadata(repo, count, err, &introspectTimeEnd)

	if err := dao.Repository.Update(repoUpdate); err != nil {
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
		output.LastIntrospectionError = pointy.String("")
		output.Status = config.StatusValid
		output.FailedIntrospectionsCount = 0
		return RepoToRepoUpdate(output)
	}

	// If introspection fails
	output.LastIntrospectionError = pointy.String(err.Error())
	output.FailedIntrospectionsCount += 1
	switch input.Status {
	case config.StatusValid:
		output.Status = config.StatusUnavailable // Repository introspected successfully at least once, but now errors
	case config.StatusInvalid:
		output.Status = config.StatusInvalid
	case config.StatusPending:
		if output.LastIntrospectionSuccessTime == nil {
			output.Status = config.StatusInvalid
		} else {
			output.Status = config.StatusUnavailable
		}
	case config.StatusUnavailable:
	default:
		output.Status = config.StatusUnavailable
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
		Status:                       &repo.Status,
		PackageCount:                 &repo.PackageCount,
		FailedIntrospectionsCount:    &repo.FailedIntrospectionsCount,
	}
}
