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
		//TODO: add handling for clowder
		return nil
	} else {

		options.SetDefault("DBHost", os.Getenv("DB_HOST"))
		options.SetDefault("DBPort", os.Getenv("DB_PORT"))
		options.SetDefault("DBUser", os.Getenv("DB_USER"))
		options.SetDefault("DBPassword", os.Getenv("DB_PASSWORD"))
		options.SetDefault("DBName", os.Getenv("DB_NAME"))

		return &Config{
			DBHost:     options.GetString("DBHost"),
			DBPort:     options.GetInt("DBPort"),
			DBUser:     options.GetString("DBUser"),
			DBPassword: options.GetString("DBPassword"),
			DBName:     options.GetString("DBName"),
		}
	}

}
