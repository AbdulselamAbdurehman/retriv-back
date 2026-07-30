package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

// Config holds all runtime settings, loaded from the environment.
type Config struct {
	HTTPAddr   string
	AppBaseURL string // public URL, used to build magic-link and masked-channel links

	DatabaseURL string

	OllamaBaseURL    string
	OllamaEmbedModel string
	OllamaChatModel  string

	// SMTP (real relay, e.g. Gmail). Username/password enable authenticated send.
	MailHost     string
	MailPort     string
	MailUsername string
	MailPassword string
	MailFrom     string

	// EncKey decrypts finder verification details (32 bytes, base64-encoded in env).
	EncKeyB64 string
	// SessionSecret signs the session cookie.
	SessionSecret string

	// Matching thresholds (all tunable without redeploy).
	MatchRadiusMeters      float64
	MatchTimeWindow        time.Duration
	MatchMaxCosineDistance float64 // candidate kept only if cosine distance <= this
	MatchCandidateLimit    int

	// Verification.
	MinConfidence     float64
	MaxVerifyAttempts int

	// Abuse / retention.
	LostReportsPerDay int
	ReportTTL         time.Duration
}

// Load reads configuration from the environment, applying defaults where sensible.
// Secrets (DB URL, encryption key, session secret) are required and error if unset.
func Load() (Config, error) {
	c := Config{
		HTTPAddr:               getenv("HTTP_ADDR", ":8080"),
		AppBaseURL:             getenv("APP_BASE_URL", "http://localhost:5173"),
		OllamaBaseURL:          getenv("OLLAMA_URL", "http://localhost:11434"),
		OllamaEmbedModel:       getenv("OLLAMA_EMBED_MODEL", "nomic-embed-text"),
		OllamaChatModel:        getenv("OLLAMA_CHAT_MODEL", "llama3.2:1b"),
		MailHost:               getenv("MAIL_HOST", "smtp.gmail.com"),
		MailPort:               getenv("MAIL_PORT", "587"),
		MailUsername:           os.Getenv("MAIL_USERNAME"),
		MailPassword:           os.Getenv("MAIL_PASSWORD"),
		MailFrom:               getenv("MAIL_FROM", ""),
		MatchRadiusMeters:      getfloat("MATCH_RADIUS_METERS", 5000),
		MatchTimeWindow:        getdur("MATCH_TIME_WINDOW", 14*24*time.Hour),
		MatchMaxCosineDistance: getfloat("MATCH_MAX_COSINE_DISTANCE", 0.35),
		MatchCandidateLimit:    getint("MATCH_CANDIDATE_LIMIT", 5),
		MinConfidence:          getfloat("MIN_CONFIDENCE", 0.7),
		MaxVerifyAttempts:      getint("MAX_VERIFY_ATTEMPTS", 3),
		LostReportsPerDay:      getint("LOST_REPORTS_PER_DAY", 2),
		ReportTTL:              getdur("REPORT_TTL", 30*24*time.Hour),
	}

	var err error
	if c.DatabaseURL, err = required("DATABASE_URL"); err != nil {
		return Config{}, err
	}
	if c.EncKeyB64, err = required("ENC_KEY"); err != nil {
		return Config{}, err
	}
	if c.SessionSecret, err = required("SESSION_SECRET"); err != nil {
		return Config{}, err
	}
	return c, nil
}

func required(key string) (string, error) {
	v := os.Getenv(key)
	if v == "" {
		return "", fmt.Errorf("config: %s is required", key)
	}
	return v, nil
}

func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func getint(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

func getfloat(key string, def float64) float64 {
	if v := os.Getenv(key); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			return f
		}
	}
	return def
}

func getdur(key string, def time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return def
}
