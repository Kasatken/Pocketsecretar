package config

import (
	"encoding/hex"
	"log"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Port             string
	DatabaseURL      string
	AllowedOrigin    string
	JWTAccessSecret  string
	JWTRefreshSecret string
	AccessTTL        time.Duration
	RefreshTTL       time.Duration
	CookieSecure     bool

	GoogleClientID     string
	GoogleClientSecret string
	GoogleRedirectURI  string

	OpenRouterAPIKey           string
	OpenRouterModel            string
	OpenRouterModelAdvice      string
	OpenRouterReferer          string
	OpenRouterProxy            string
	SmartTaskMaxTokens    int
	AdviceMaxTokens       int
	FreeSmartTaskLimit int
	ProSmartTaskLimit  int
	// FreeAdviceRefreshPerHour — максимум принудительных refresh советника в час для Free-пользователей.
	FreeAdviceRefreshPerHour int
	// ProAdviceRefreshPerHour — то же для Pro.
	ProAdviceRefreshPerHour int
	// ProPromoCode — секретная строка; при совпадении POST /api/auth/me/promo выставляет пользователю plan=pro.
	ProPromoCode string

	UploadDir      string
	AvatarMaxBytes int64

	// TokenEncryptionKey — 32-байтовый ключ (64 hex-символа) для AES-256-GCM шифрования
	// OAuth refresh-токенов в БД. Если не задан, шифрование отключено (dev-режим).
	TokenEncryptionKey []byte
}

func Load() Config {
	return Config{
		Port:             getEnv("PORT", "8080"),
		DatabaseURL:      os.Getenv("DATABASE_URL"),
		AllowedOrigin:    getEnv("CORS_ALLOWED_ORIGIN", "http://localhost:5173"),
		JWTAccessSecret:  getEnv("JWT_ACCESS_SECRET", "dev-access-secret"),
		JWTRefreshSecret: getEnv("JWT_REFRESH_SECRET", "dev-refresh-secret"),
		AccessTTL:        time.Duration(getEnvInt("ACCESS_TTL_MIN", 15)) * time.Minute,
		RefreshTTL:       time.Duration(getEnvInt("REFRESH_TTL_DAYS", 14)) * 24 * time.Hour,
		CookieSecure:     getEnv("COOKIE_SECURE", "false") == "true",

		GoogleClientID:     os.Getenv("GOOGLE_CLIENT_ID"),
		GoogleClientSecret: os.Getenv("GOOGLE_CLIENT_SECRET"),
		GoogleRedirectURI:  os.Getenv("GOOGLE_REDIRECT_URI"),

		OpenRouterAPIKey:           os.Getenv("OPENROUTER_API_KEY"),
		OpenRouterModel:            getEnv("OPENROUTER_MODEL", "openai/gpt-4o-mini"),
		OpenRouterModelAdvice:      strings.TrimSpace(os.Getenv("OPENROUTER_MODEL_ADVICE")),
		OpenRouterReferer:          getEnv("OPENROUTER_REFERER", "http://localhost:5173"),
		OpenRouterProxy:            os.Getenv("OPENROUTER_PROXY"),
		SmartTaskMaxTokens:    getEnvInt("SMART_TASK_MAX_TOKENS", 2048),
		AdviceMaxTokens:       getEnvInt("ADVICE_MAX_TOKENS", 1100),
		FreeSmartTaskLimit:       getEnvInt("FREE_SMART_TASK_LIMIT", 3),
		ProSmartTaskLimit:        getEnvInt("PRO_SMART_TASK_LIMIT", 15),
		FreeAdviceRefreshPerHour: getEnvInt("FREE_ADVICE_REFRESH_PER_HOUR", 5),
		ProAdviceRefreshPerHour:  getEnvInt("PRO_ADVICE_REFRESH_PER_HOUR", 20),
		ProPromoCode:       strings.TrimSpace(os.Getenv("PRO_PROMO_CODE")),

		UploadDir:          getEnv("UPLOAD_DIR", "./uploads"),
		AvatarMaxBytes:     int64(getEnvInt("AVATAR_MAX_BYTES", 2_000_000)),
		TokenEncryptionKey: parseHexKey(os.Getenv("TOKEN_ENCRYPTION_KEY")),
	}
}

// parseHexKey декодирует 64-символьный hex-ключ в 32 байта.
// Если строка пустая — возвращает nil (шифрование отключено).
func parseHexKey(s string) []byte {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	b, err := hex.DecodeString(s)
	if err != nil || len(b) != 32 {
		log.Printf("WARNING: TOKEN_ENCRYPTION_KEY невалиден (нужно 64 hex-символа) — шифрование токенов отключено")
		return nil
	}
	return b
}

func getEnv(key, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	return value
}

func getEnvInt(key string, fallback int) int {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return parsed
}
