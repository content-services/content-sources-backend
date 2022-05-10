package config

import (
	"os"

	clowder "github.com/redhatinsights/app-common-go/pkg/api/v1"
	"github.com/spf13/viper"
)

type Config struct {
	DBHost     string
	DBPort     int
	DBUser     string
	DBPassword string
	DBName     string
}

func Get() *Config {
	options := viper.New()

	if clowder.IsClowderEnabled() {
		cfg := clowder.LoadedConfig

		options.SetDefault("DBHost", cfg.Database.Hostname)
		options.SetDefault("DBPort", cfg.Database.Port)
		options.SetDefault("DBUser", cfg.Database.Username)
		options.SetDefault("DBPassword", cfg.Database.Password)
		options.SetDefault("DBName", cfg.Database.Name)
	} else {
		options.SetDefault("DBHost", os.Getenv("DATABASE_HOST"))
		options.SetDefault("DBPort", os.Getenv("DATABASE_PORT"))
		options.SetDefault("DBUser", os.Getenv("DATABASE_USER"))
		options.SetDefault("DBPassword", os.Getenv("DATABASE_PASSWORD"))
		options.SetDefault("DBName", os.Getenv("DATABASE_NAME"))
	}

	return &Config{
		DBHost:     options.GetString("DBHost"),
		DBPort:     options.GetInt("DBPort"),
		DBUser:     options.GetString("DBUser"),
		DBPassword: options.GetString("DBPassword"),
		DBName:     options.GetString("DBName"),
	}
}
