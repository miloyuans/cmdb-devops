package config

import (
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	AppEnv               string
	HTTPAddr             string
	MongoURI             string
	MongoAdminDB         string
	JWTSecret            string
	EncryptionKey        string
	DefaultAdminUser     string
	DefaultAdminPassword string
	PublicBaseURL        string
	ProviderMode         string
	InventoryInterval    time.Duration
	RegionCheckInterval  time.Duration
	IdentityInterval     time.Duration
	ConfigReloadEvery    time.Duration
}

func Load() Config {
	return Config{
		AppEnv:               env("APP_ENV", "dev"),
		HTTPAddr:             env("HTTP_ADDR", ":8080"),
		MongoURI:             env("MONGO_URI", "mongodb://localhost:27017"),
		MongoAdminDB:         env("MONGO_ADMIN_DB", "cmdb_admin"),
		JWTSecret:            env("JWT_SECRET", "change-this-secret"),
		EncryptionKey:        env("ENC_KEY", "0123456789abcdef0123456789abcdef"),
		DefaultAdminUser:     env("DEFAULT_ADMIN_USER", "admin"),
		DefaultAdminPassword: env("DEFAULT_ADMIN_PASSWORD", "admin123456"),
		PublicBaseURL:        strings.TrimRight(env("PUBLIC_BASE_URL", "http://localhost:8080"), "/"),
		ProviderMode:         env("PROVIDER_MODE", "mock"),
		InventoryInterval:    secondsEnv("INVENTORY_INTERVAL_SECONDS", 1800),
		RegionCheckInterval:  secondsEnv("REGION_CHECK_INTERVAL_SECONDS", 86400),
		IdentityInterval:     secondsEnv("IDENTITY_INTERVAL_SECONDS", 21600),
		ConfigReloadEvery:    secondsEnv("CONFIG_RELOAD_SECONDS", 10),
	}
}

func env(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}

func secondsEnv(key string, fallback int) time.Duration {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return time.Duration(fallback) * time.Second
	}
	n, err := strconv.Atoi(v)
	if err != nil || n <= 0 {
		return time.Duration(fallback) * time.Second
	}
	return time.Duration(n) * time.Second
}
