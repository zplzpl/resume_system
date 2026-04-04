package main

import (
	"log"
	"net/http"
	"os"

	"github.com/zplzpl/resume_system/internal/httpapi"
	"github.com/zplzpl/resume_system/internal/report"
)

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	service := report.NewService(nil)
	server := httpapi.NewServer(service)

	addr := ":" + port
	log.Printf("resume_system server listening on %s", addr)
	if err := http.ListenAndServe(addr, server.Handler()); err != nil {
		log.Fatalf("server exited: %v", err)
	}
}
