package main

import (
	"log"

	"github.com/content-services/content-sources-backend/pkg/db"
	"github.com/gin-gonic/gin"
)

func main() {
	var err error

	err = db.Connect()
	if err != nil {
		log.Fatalf("%v", err)
	}

	r := gin.Default()
	r.GET("/ping", func(c *gin.Context) {
		c.JSON(200, gin.H{
			"message": "pong",
		})
	})

	err = r.Run(":8000")
	if err != nil {
		log.Fatalf("%v", err)
	}
}
