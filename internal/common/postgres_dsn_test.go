/*******************************************************************************
* Copyright (C) 2026 the Eclipse BaSyx Authors and Fraunhofer IESE
*
* Permission is hereby granted, free of charge, to any person obtaining
* a copy of this software and associated documentation files (the
* "Software"), to deal in the Software without restriction, including
* without limitation the rights to use, copy, modify, merge, publish,
* distribute, sublicense, and/or sell copies of the Software, and to
* permit persons to whom the Software is furnished to do so, subject to
* the following conditions:
*
* The above copyright notice and this permission notice shall be
* included in all copies or substantial portions of the Software.
*
* THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND,
* EXPRESS OR IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF
* MERCHANTABILITY, FITNESS FOR A PARTICULAR PURPOSE AND
* NONINFRINGEMENT. IN NO EVENT SHALL THE AUTHORS OR COPYRIGHT HOLDERS BE
* LIABLE FOR ANY CLAIM, DAMAGES OR OTHER LIABILITY, WHETHER IN AN ACTION
* OF CONTRACT, TORT OR OTHERWISE, ARISING FROM, OUT OF OR IN CONNECTION
* WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE SOFTWARE.
*
* SPDX-License-Identifier: MIT
******************************************************************************/

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

func TestBuildPostgresDSNUsesConfiguredDSN(t *testing.T) {
	cfg := PostgresConfig{
		DSN: "  postgres://user:p@ss@localhost:5432/basyx?sslmode=require&application_name=svc  ",
	}

	dsn := BuildPostgresDSN(cfg)
	parsed, err := url.Parse(dsn)
	if err != nil {
		t.Fatalf("parse dsn: %v", err)
	}

	if parsed.Scheme != "postgres" {
		t.Fatalf("unexpected scheme: %s", parsed.Scheme)
	}
	pass, ok := parsed.User.Password()
	if !ok {
		t.Fatal("expected password")
	}
	if pass != "p@ss" {
		t.Fatalf("unexpected password: %s", pass)
	}
	assertQueryValue(t, parsed.Query(), "sslmode", "require")
	assertQueryValue(t, parsed.Query(), "application_name", "svc")
}

func TestBuildPostgresDSNWithConfiguredParameters(t *testing.T) {
	cfg := PostgresConfig{
		Host:                    "localhost",
		Port:                    5432,
		User:                    "user",
		Password:                "password",
		DBName:                  "basyx",
		SSLMode:                 "verify-full",
		SSLCert:                 "/certs/client.crt",
		SSLKey:                  "/certs/client.key",
		SSLRootCert:             "/certs/root.crt",
		ConnectTimeoutSeconds:   15,
		ApplicationName:         "basyx-service",
		FallbackApplicationName: "basyx",
		SearchPath:              "tenant_a,public",
		Options:                 "-c statement_timeout=5000",
		TimeZone:                "Europe/Berlin",
	}

	dsn := BuildPostgresDSN(cfg)
	parsed, err := url.Parse(dsn)
	if err != nil {
		t.Fatalf("parse dsn: %v", err)
	}

	query := parsed.Query()
	assertQueryValue(t, query, "sslmode", "verify-full")
	assertQueryValue(t, query, "sslcert", "/certs/client.crt")
	assertQueryValue(t, query, "sslkey", "/certs/client.key")
	assertQueryValue(t, query, "sslrootcert", "/certs/root.crt")
	assertQueryValue(t, query, "connect_timeout", "15")
	assertQueryValue(t, query, "application_name", "basyx-service")
	assertQueryValue(t, query, "fallback_application_name", "basyx")
	assertQueryValue(t, query, "search_path", "tenant_a,public")
	assertQueryValue(t, query, "options", "-c statement_timeout=5000")
	assertQueryValue(t, query, "TimeZone", "Europe/Berlin")
}

func assertQueryValue(t *testing.T, query url.Values, key string, want string) {
	t.Helper()
	if got := query.Get(key); got != want {
		t.Fatalf("unexpected %s: %q, want %q", key, got, want)
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
