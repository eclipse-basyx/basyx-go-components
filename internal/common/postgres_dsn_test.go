package common

import "testing"

func TestBuildPostgresDSNEncodesCredentials(t *testing.T) {
	cfg := PostgresConfig{
		Host:     "localhost",
		Port:     5432,
		User:     "service:user",
		Password: "p@ss:w/rd?#",
		DBName:   "basyx_db",
	}

	dsn := BuildPostgresDSN(cfg)
	expected := "postgres://service%3Auser:p%40ss%3Aw%2Frd%3F%23@localhost:5432/basyx_db?sslmode=disable"
	if dsn != expected {
		t.Fatalf("expected %q, got %q", expected, dsn)
	}
}

func TestNormalizePostgresDSNKeepsEncodedURLFormat(t *testing.T) {
	raw := "postgres://service%3Auser:p%40ss@localhost:5432/basyx_db?sslmode=disable"

	normalized := NormalizePostgresDSN(raw)
	if normalized != raw {
		t.Fatalf("expected %q, got %q", raw, normalized)
	}
}

func TestNormalizePostgresDSNKeepsNonURLFormat(t *testing.T) {
	raw := "host=localhost user=admin password=admin123 dbname=basyx sslmode=disable"

	normalized := NormalizePostgresDSN(raw)
	if normalized != raw {
		t.Fatalf("expected %q, got %q", raw, normalized)
	}
}
