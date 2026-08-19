package config

import "os"

type Config struct {
	Port        string
	DatabaseURL string
	Version     string

	// AnthropicAPIKey is required by cmd/triage; getEnv can't distinguish
	// "unset" from "", so cmd/triage checks for "" explicitly and refuses
	// to start rather than run every classification call to a guaranteed
	// failure.
	AnthropicAPIKey string
	// TriagePort is the port cmd/triage listens on.
	TriagePort string
	// TriageServiceURL is where cmd/api reaches the triage service
	// (fire-and-forget POST /triage from tickets.Service.Create).
	TriageServiceURL string
	// MainAppURL is where cmd/triage reaches the main app to write a
	// classification result back (POST /tickets/{id}/triage).
	MainAppURL string
	// ReplyGuardPort is the port cmd/replyguard listens on.
	ReplyGuardPort string
	// ReplyGuardServiceURL is where cmd/api reaches the reply-guard
	// service: a synchronous POST /guard call from
	// tickets.Service.AddComment's reply-guard gate (see the
	// httpReplyGuardClient adapter in cmd/api/main.go), which blocks
	// comment creation on the response.
	ReplyGuardServiceURL string
}

func Load() Config {
	return Config{
		Port:             getEnv("PORT", "3000"),
		DatabaseURL:      getEnv("DATABASE_URL", "postgres://resolve:resolve@localhost:5432/resolve"),
		Version:          getEnv("VERSION", "dev"),
		AnthropicAPIKey:  getEnv("ANTHROPIC_API_KEY", ""),
		TriagePort:       getEnv("TRIAGE_PORT", "3001"),
		TriageServiceURL: getEnv("TRIAGE_SERVICE_URL", "http://localhost:3001"),
		MainAppURL:       getEnv("MAIN_APP_URL", "http://localhost:3000"),

		ReplyGuardPort:       getEnv("REPLYGUARD_PORT", "3002"),
		ReplyGuardServiceURL: getEnv("REPLYGUARD_SERVICE_URL", "http://localhost:3002"),
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
