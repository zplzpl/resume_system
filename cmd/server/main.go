package main

import (
	"log"

	"github.com/zplzpl/resume_system/internal/config"
	"github.com/zplzpl/resume_system/internal/server"
)

func main() {
	cfg := config.FromEnv()

	r, err := server.NewRouter(cfg)
	if err != nil {
		log.Fatalf("init router: %v", err)
	}

	if err := r.Run(":" + cfg.Port); err != nil {
		log.Fatalf("run server: %v", err)
	}
}
