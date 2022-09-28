package main

import (
	config "github.com/content-services/content-sources-backend/pkg/config"
	"github.com/content-services/content-sources-backend/pkg/db"
	"github.com/content-services/content-sources-backend/pkg/event"
	"github.com/content-services/content-sources-backend/pkg/event/handler"
)

// func fillTopics(cfg *config.Configuration) {
// 	cfg.Kafka.Topics = []string{
// 		schema.TopicIntrospect,
// 	}
// }

func main() {
	cfg := config.Get()
	// fillTopics(cfg)
	if err := db.Connect(); err != nil {
		panic(err)
	}
	handler := handler.NewIntrospectHandler(db.DB)
	event.Start(cfg, handler)
}
