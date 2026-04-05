package config

import "os"

type Config struct {
	Port              string
	SupabaseURL       string
	SupabaseAnonKey   string
	SupabaseJWTSecret string
}

func FromEnv() Config {
	return Config{
		Port:              getenv("PORT", "8080"),
		SupabaseURL:       os.Getenv("SUPABASE_URL"),
		SupabaseAnonKey:   os.Getenv("SUPABASE_ANON_KEY"),
		SupabaseJWTSecret: os.Getenv("SUPABASE_JWT_SECRET"),
	}
}

func getenv(key, fallback string) string {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	return v
}
