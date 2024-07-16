package config

import (
	"github.com/content-services/tang/pkg/tangy"
	"github.com/rs/zerolog/log"
)

var Tang *tangy.Tangy

func ConfigureTang() error {
	log.Info().Msgf("LOGHERE: pulp server: %v, snapshots: %v", Get().Clients.Pulp.Server, Get().Features.Snapshots.Enabled)
	if Get().Clients.Pulp.Server == "" || !Get().Features.Snapshots.Enabled {
		return nil
	}

	pDb := Get().Clients.Pulp.Database
	tDb := tangy.Database{
		Name:       pDb.Name,
		Host:       pDb.Host,
		Port:       pDb.Port,
		User:       pDb.User,
		Password:   pDb.Password,
		CACertPath: pDb.CACertPath,
		PoolLimit:  pDb.PoolLimit,
	}
	tLogger := tangy.Logger{
		Logger:   &log.Logger,
		LogLevel: Get().Logging.Level,
		Enabled:  true,
	}
	t, err := tangy.New(tDb, tLogger)
	log.Info().Msgf("LOGHERE: new tang: %v", t)
	if err != nil {
		return err
	}
	Tang = &t
	log.Info().Msgf("LOGHERE: tang: %v", Tang)
	return nil
}
