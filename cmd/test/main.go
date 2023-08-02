package main

import (
	"github.com/content-services/content-sources-backend/pkg/db"
	"github.com/content-services/content-sources-backend/pkg/seeds"
)

func main() {
	db.Connect()
	d := db.DB

	for i := 0; i < 1000; i++ {
		seeds.SeedTasks(d, 100, seeds.TaskSeedOptions{})
	}
}
