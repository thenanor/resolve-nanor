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
	"resolve/internal/replyguard"
)

func main() {
	cfg := config.Load()

	// A reply-guard service that can never succeed should refuse to start
	// rather than accept traffic and silently no-op on every request.
	if cfg.AnthropicAPIKey == "" {
		log.Fatal("ANTHROPIC_API_KEY must be set to run the reply-guard service")
	}

	anthropicClient := anthropic.NewClient(option.WithAPIKey(cfg.AnthropicAPIKey))
	classifier := replyguard.NewAnthropicClassifier(anthropicClient)
	service := replyguard.NewService(classifier)

	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	replyguard.NewHandler(service).Mount(r)
	health.NewHandler(cfg.Version).Mount(r)

	log.Printf("reply-guard listening on :%s", cfg.ReplyGuardPort)
	if err := http.ListenAndServe(":"+cfg.ReplyGuardPort, r); err != nil {
		log.Fatal(err)
	}
}
