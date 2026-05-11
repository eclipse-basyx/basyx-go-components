//nolint:gosec // Test file intentionally contains DSN fixtures with placeholder credentials.
package common

import (
	"net/url"
	"testing"
)

func TestBuildPostgresDSN(t *testing.T) {
	cfg := PostgresConfig{
		Host:     "localhost",
		Port:     5432,
		User:     "user@name",
		Password: "p@ss:word/with?chars&=",
		DBName:   "basyx",
	}

	dsn := BuildPostgresDSN(cfg)
	parsed, err := url.Parse(dsn)
	if err != nil {
		t.Fatalf("parse dsn: %v", err)
	}

	if parsed.Scheme != "postgres" {
		t.Fatalf("unexpected scheme: %s", parsed.Scheme)
	}
	if parsed.Host != "localhost:5432" {
		t.Fatalf("unexpected host: %s", parsed.Host)
	}
	if parsed.Path != "/basyx" {
		t.Fatalf("unexpected path: %s", parsed.Path)
	}
	if parsed.User == nil {
		t.Fatal("expected user info")
	}
	if got := parsed.User.Username(); got != cfg.User {
		t.Fatalf("unexpected username: %s", got)
	}
	pass, ok := parsed.User.Password()
	if !ok {
		t.Fatal("expected password")
	}
	if pass != cfg.Password {
		t.Fatalf("unexpected password: %s", pass)
	}
	if got := parsed.Query().Get("sslmode"); got != "disable" {
		t.Fatalf("unexpected sslmode: %s", got)
	}
}

func TestNormalizePostgresDSN(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "trims non url dsn",
			in:   "  host=localhost port=5432 user=foo password=bar dbname=test sslmode=disable  ",
			want: "host=localhost port=5432 user=foo password=bar dbname=test sslmode=disable",
		},
		{
			name: "trims on parse error",
			in:   "  postgres://%zz  ",
			want: "postgres://%zz",
		},
		{
			name: "trims url without user info",
			in:   "  postgresql://localhost:5432/testdb?sslmode=disable  ",
			want: "postgresql://localhost:5432/testdb?sslmode=disable",
		},
		{
			name: "keeps already encoded credentials",
			in:   "postgres://user:p%40ss@localhost:5432/mydb?sslmode=disable",
			want: "postgres://user:p%40ss@localhost:5432/mydb?sslmode=disable",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := NormalizePostgresDSN(tt.in); got != tt.want {
				t.Fatalf("NormalizePostgresDSN() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestNormalizePostgresDSN_EncodesUserInfoAndPreservesQuery(t *testing.T) {
	in := "  postgresql://user:p@ss@localhost:5432/mydb?sslmode=disable&application_name=svc  "

	got := NormalizePostgresDSN(in)
	parsed, err := url.Parse(got)
	if err != nil {
		t.Fatalf("parse normalized dsn: %v", err)
	}

	if parsed.Scheme != "postgresql" {
		t.Fatalf("unexpected scheme: %s", parsed.Scheme)
	}
	if parsed.User == nil {
		t.Fatal("expected user info")
	}
	if gotUser := parsed.User.Username(); gotUser != "user" {
		t.Fatalf("unexpected username: %s", gotUser)
	}
	pass, ok := parsed.User.Password()
	if !ok {
		t.Fatal("expected password")
	}
	if pass != "p@ss" {
		t.Fatalf("unexpected password: %s", pass)
	}
	if gotSSLMode := parsed.Query().Get("sslmode"); gotSSLMode != "disable" {
		t.Fatalf("unexpected sslmode: %s", gotSSLMode)
	}
	if appName := parsed.Query().Get("application_name"); appName != "svc" {
		t.Fatalf("unexpected application_name: %s", appName)
	}
}
