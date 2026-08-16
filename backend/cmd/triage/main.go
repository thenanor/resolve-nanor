package main

import (
	"log"
	"net/http"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"resolve/internal/config"
	"resolve/internal/health"
	"resolve/internal/triage"
)

func main() {
	cfg := config.Load()

	// A triage service that can never succeed should refuse to start
	// rather than accept traffic and silently no-op on every request.
	if cfg.AnthropicAPIKey == "" {
		log.Fatal("ANTHROPIC_API_KEY must be set to run the triage service")
	}

	anthropicClient := anthropic.NewClient(option.WithAPIKey(cfg.AnthropicAPIKey))
	classifier := triage.NewAnthropicClassifier(anthropicClient)
	updater := &triage.HTTPTicketUpdater{BaseURL: cfg.MainAppURL, Client: http.DefaultClient}
	service := triage.NewService(classifier, updater)

	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	triage.NewHandler(service).Mount(r)
	health.NewHandler(cfg.Version).Mount(r)

	log.Printf("triage listening on :%s", cfg.TriagePort)
	if err := http.ListenAndServe(":"+cfg.TriagePort, r); err != nil {
		log.Fatal(err)
	}
}
