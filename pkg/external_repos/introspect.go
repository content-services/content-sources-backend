package external_repos

import (
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/content-services/content-sources-backend/pkg/dao"
	"github.com/content-services/content-sources-backend/pkg/db"
	"github.com/content-services/yummy/pkg/yum"
	"github.com/openlyinc/pointy"
	"github.com/rs/zerolog/log"
)

const (
	RhCdnHost              = "cdn.redhat.com"
	IntrospectTimeInterval = time.Hour * 23
)

// IntrospectUrl Fetch the metadata of a url and insert RPM data
// Returns the number of new RPMs inserted system-wide and any error encountered
func IntrospectUrl(url string, force bool) (int64, []error) {
	urls := []string{url}
	return IntrospectAll(&urls, force)
}

// IsRedHat returns if the url is a 'cdn.redhat.com' url
func IsRedHat(url string) bool {
	return strings.Contains(url, RhCdnHost)
}

// Introspect introspects a dao.Repository with the given RpmDao
// inserting any needed RPMs and adding and removing associations to the repository
// Returns the number of new RPMs inserted system-wide and any error encountered
func Introspect(repo *dao.Repository, repoDao dao.RepositoryDao, rpm dao.RpmDao) (int64, error) {
	var (
		client   http.Client
		err      error
		total    int64
		repomd   *yum.Repomd
		packages []yum.Package
	)
	log.Debug().Msg("Introspecting " + repo.URL)

	if repo.FailedIntrospectionsCount >= config.FailedIntrospectionsLimit && !repo.Public {
		return 0, fmt.Errorf("introspection skipped because this repository has failed more than %v times in a row", config.FailedIntrospectionsLimit)
	}

	if client, err = httpClient(IsRedHat(repo.URL)); err != nil {
		return 0, err
	}
	settings := yum.YummySettings{
		Client: &client,
		URL:    &repo.URL,
	}
	yumRepo, _ := yum.NewRepository(settings)

	if repomd, _, err = yumRepo.Repomd(); err != nil {
		return 0, err
	}

	checksumStr := ""
	if repomd.RepomdString != nil && *repomd.RepomdString != "" {
		sum := sha256.Sum256([]byte(*repomd.RepomdString))
		checksumStr = hex.EncodeToString(sum[:])
	}

	if repo.RepomdChecksum != "" && checksumStr != "" && checksumStr == repo.RepomdChecksum {
		// If repository hasn't changed, no need to update
		return 0, nil
	}

	if packages, _, err = yumRepo.Packages(); err != nil {
		return 0, err
	}

	if total, err = rpm.InsertForRepository(repo.UUID, packages); err != nil {
		return 0, err
	}

	var foundCount int
	if foundCount, err = repoDao.FetchRepositoryRPMCount(repo.UUID); err != nil {
		return 0, err
	}

	repo.RepomdChecksum = checksumStr
	repo.PackageCount = foundCount
	if err = repoDao.Update(RepoToRepoUpdate(*repo)); err != nil {
		return 0, err
	}

	return total, nil
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
// Returns the number of new RPMs inserted system-wide and all errors encountered
func IntrospectAll(urls *[]string, force bool) (int64, []error) {
	var (
		total   int64
		count   int64
		err     error
		rpmDao  = dao.GetRpmDao(db.DB)
		repoDao = dao.GetRepositoryDao(db.DB)
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
		count, err = Introspect(&repos[i], repoDao, rpmDao)
		total += count

		if err != nil {
			errors = append(errors, fmt.Errorf("Error introspecting %s: %s", repos[i].URL, err.Error()))
		}
		err = UpdateIntrospectionStatusMetadata(repos[i], repoDao, count, err)
		if err != nil {
			errors = append(errors, err)
		}
	}
	err = repoDao.OrphanCleanup()
	if err != nil {
		errors = append(errors, err)
	}
	err = rpmDao.OrphanCleanup()
	if err != nil {
		errors = append(errors, err)
	}
	return total, errors
}

func needsIntrospect(repo *dao.Repository) (bool, string) {
	if repo == nil {
		return false, "Cannot introspect nil Repository"
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
func UpdateIntrospectionStatusMetadata(repo dao.Repository, repoDao dao.RepositoryDao, count int64, err error) error {
	introspectTimeEnd := time.Now()
	repoUpdate := updateIntrospectionStatusMetadata(repo, count, err, &introspectTimeEnd)

	if err := repoDao.Update(repoUpdate); err != nil {
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
