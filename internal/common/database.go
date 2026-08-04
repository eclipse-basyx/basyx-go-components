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

//nolint:all
package common

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/doug-martin/goqu/v9"
	_ "github.com/doug-martin/goqu/v9/dialect/postgres"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/telemetry"
	_ "github.com/jackc/pgx/v5/stdlib"
)

const (
	CURRENT_DATABASE_VERSION = "v1.1.12"
	cleanSchemaState         = "clean"
)

// PostgresPoolSettings contains the effective database/sql pool limits.
type PostgresPoolSettings struct {
	MaxOpenConnections     int
	MaxIdleConnections     int
	ConnMaxLifetimeMinutes int
	ConnMaxIdleTimeMinutes int
}

// PostgresPools contains the writer pool and the pool used for eligible,
// eventually consistent reads. Reader is the same handle as Writer when no
// separate reader connection is configured.
type PostgresPools struct {
	Writer *sql.DB
	Reader *sql.DB
}

type writerPostgresReadsContextKey struct{}

type postgresPoolOpener func(
	context.Context,
	PostgresConfig,
	string,
	telemetry.DatabasePoolRole,
) (*sql.DB, error)

// WithWriterPostgresReads marks database reads that decide, guard, or validate
// a mutation and therefore require writer consistency.
func WithWriterPostgresReads(ctx context.Context) context.Context {
	return context.WithValue(ctx, writerPostgresReadsContextKey{}, true)
}

// PostgresReadPool selects the configured reader unless the context requires
// writer consistency. A nil reader falls back to the writer.
func PostgresReadPool(ctx context.Context, writer *sql.DB, reader *sql.DB) *sql.DB {
	if reader == nil || ctx != nil && ctx.Value(writerPostgresReadsContextKey{}) == true {
		return writer
	}
	return reader
}

// ResolvePostgresPoolSettings validates and normalizes PostgreSQL pool settings.
// Zero uses the common default, except connMaxIdleTimeMinutes where zero disables
// idle-time recycling.
func ResolvePostgresPoolSettings(cfg PostgresConfig) (PostgresPoolSettings, error) {
	if cfg.MaxOpenConnections < 0 {
		return PostgresPoolSettings{}, fmt.Errorf("CONFIG-POSTGRES-MAXOPEN postgres.maxOpenConnections must not be negative")
	}
	if cfg.MaxIdleConnections < 0 {
		return PostgresPoolSettings{}, fmt.Errorf("CONFIG-POSTGRES-MAXIDLE postgres.maxIdleConnections must not be negative")
	}
	if cfg.ConnMaxLifetimeMinutes < 0 {
		return PostgresPoolSettings{}, fmt.Errorf("CONFIG-POSTGRES-CONNMAXLIFETIME postgres.connMaxLifetimeMinutes must not be negative")
	}
	if cfg.ConnMaxIdleTimeMinutes < 0 {
		return PostgresPoolSettings{}, fmt.Errorf("CONFIG-POSTGRES-CONNMAXIDLETIME postgres.connMaxIdleTimeMinutes must not be negative")
	}

	settings := PostgresPoolSettings{
		MaxOpenConnections:     defaultWhenZero(cfg.MaxOpenConnections, DefaultConfig.PgMaxOpen),
		MaxIdleConnections:     defaultWhenZero(cfg.MaxIdleConnections, DefaultConfig.PgMaxIdle),
		ConnMaxLifetimeMinutes: defaultWhenZero(cfg.ConnMaxLifetimeMinutes, DefaultConfig.PgConnLifetime),
		ConnMaxIdleTimeMinutes: cfg.ConnMaxIdleTimeMinutes,
	}
	if cfg.MaxIdleConnections == 0 && settings.MaxIdleConnections > settings.MaxOpenConnections {
		settings.MaxIdleConnections = settings.MaxOpenConnections
	}
	if settings.MaxIdleConnections > settings.MaxOpenConnections {
		return PostgresPoolSettings{}, fmt.Errorf(
			"CONFIG-POSTGRES-IDLEEXCEEDSOPEN postgres.maxIdleConnections (%d) must not exceed postgres.maxOpenConnections (%d)",
			settings.MaxIdleConnections,
			settings.MaxOpenConnections,
		)
	}

	return settings, nil
}

func defaultWhenZero(value int, defaultValue int) int {
	if value == 0 {
		return defaultValue
	}
	return value
}

// ConfigurePostgresPool validates and applies all database/sql pool settings.
func ConfigurePostgresPool(db *sql.DB, cfg PostgresConfig) (PostgresPoolSettings, error) {
	if db == nil {
		return PostgresPoolSettings{}, fmt.Errorf("COMMON-POSTGRESPOOL-NILDB database handle is nil")
	}

	settings, err := ResolvePostgresPoolSettings(cfg)
	if err != nil {
		return PostgresPoolSettings{}, err
	}
	db.SetMaxOpenConns(settings.MaxOpenConnections)
	db.SetMaxIdleConns(settings.MaxIdleConnections)
	db.SetConnMaxLifetime(time.Duration(settings.ConnMaxLifetimeMinutes) * time.Minute)
	db.SetConnMaxIdleTime(time.Duration(settings.ConnMaxIdleTimeMinutes) * time.Minute)
	return settings, nil
}

// OpenPostgres opens, configures, and verifies the shared PostgreSQL connection pool.
func OpenPostgres(ctx context.Context, cfg PostgresConfig, serviceName string) (*sql.DB, error) {
	return openPostgresForRole(ctx, cfg, serviceName, telemetry.DatabasePoolRoleWriter)
}

func openPostgresForRole(
	ctx context.Context,
	cfg PostgresConfig,
	serviceName string,
	role telemetry.DatabasePoolRole,
) (*sql.DB, error) {
	if ctx == nil {
		return nil, fmt.Errorf("COMMON-OPENPOSTGRES-NOCONTEXT context is required")
	}
	if strings.TrimSpace(serviceName) == "" {
		return nil, fmt.Errorf("COMMON-OPENPOSTGRES-NOSERVICE service name is required")
	}

	dsn, applicationName, err := BuildPostgresDSNForService(cfg, serviceName)
	if err != nil {
		return nil, err
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, fmt.Errorf("COMMON-OPENPOSTGRES-OPEN failed to open PostgreSQL connection pool: %w", err)
	}

	settings, err := ConfigurePostgresPool(db, cfg)
	if err != nil {
		_ = db.Close()
		return nil, err
	}
	if err = db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("COMMON-OPENPOSTGRES-PING failed to connect to PostgreSQL: %w", err)
	}
	if err = telemetry.RegisterDatabasePool(db, role); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("COMMON-OPENPOSTGRES-METRICS failed to register PostgreSQL connection pool: %w", err)
	}

	slog.InfoContext(
		ctx,
		"PostgreSQL connection pool configured",
		"service.name", serviceName,
		"pool.role", role,
		"application_name", applicationName,
		"max_open_connections", settings.MaxOpenConnections,
		"max_idle_connections", settings.MaxIdleConnections,
		"conn_max_lifetime_minutes", settings.ConnMaxLifetimeMinutes,
		"conn_max_idle_time_minutes", settings.ConnMaxIdleTimeMinutes,
	)
	return db, nil
}

// OpenPostgresPoolsWithSchemaValidation opens the writer and optional reader
// pools. Schema validation always runs on the writer. The reader is opened only
// when postgres.reader is configured.
func OpenPostgresPoolsWithSchemaValidation(
	ctx context.Context,
	cfg PostgresConfig,
	serviceName string,
	expectedVersion string,
) (*PostgresPools, error) {
	writer, err := OpenPostgresWithSchemaValidation(ctx, cfg, serviceName, expectedVersion)
	if err != nil {
		return nil, fmt.Errorf("COMMON-OPENPOSTGRESPOOLS-WRITER writer connection failed: %w", err)
	}
	return attachPostgresReader(ctx, cfg, serviceName, writer, openPostgresForRole)
}

func attachPostgresReader(
	ctx context.Context,
	cfg PostgresConfig,
	serviceName string,
	writer *sql.DB,
	opener postgresPoolOpener,
) (*PostgresPools, error) {
	if writer == nil {
		return nil, fmt.Errorf("COMMON-OPENPOSTGRESPOOLS-NILWRITER writer database handle is nil")
	}
	if cfg.Reader == nil {
		return &PostgresPools{Writer: writer, Reader: writer}, nil
	}
	if opener == nil {
		closeErr := closePostgresPool(writer)
		return nil, errors.Join(
			errors.New("COMMON-OPENPOSTGRESPOOLS-NILOPENER PostgreSQL pool opener is nil"),
			closeErr,
		)
	}

	readerServiceName := serviceName + "-reader"
	reader, err := opener(ctx, *cfg.Reader, readerServiceName, telemetry.DatabasePoolRoleReader)
	if err != nil {
		closeErr := closePostgresPool(writer)
		return nil, fmt.Errorf(
			"COMMON-OPENPOSTGRESPOOLS-READER reader connection failed: %w",
			errors.Join(err, closeErr),
		)
	}
	if reader == nil {
		closeErr := closePostgresPool(writer)
		return nil, errors.Join(
			errors.New("COMMON-OPENPOSTGRESPOOLS-NILREADER reader database handle is nil"),
			closeErr,
		)
	}
	return &PostgresPools{Writer: writer, Reader: reader}, nil
}

// Close unregisters and closes each distinct database pool.
func (p *PostgresPools) Close() error {
	if p == nil {
		return nil
	}
	var closeErrors []error
	if p.Reader != nil && p.Reader != p.Writer {
		if err := closePostgresPool(p.Reader); err != nil {
			closeErrors = append(closeErrors, fmt.Errorf("COMMON-POSTGRESPOOLS-CLOSEREADER failed to close reader pool: %w", err))
		}
	}
	if p.Writer != nil {
		if err := closePostgresPool(p.Writer); err != nil {
			closeErrors = append(closeErrors, fmt.Errorf("COMMON-POSTGRESPOOLS-CLOSEWRITER failed to close writer pool: %w", err))
		}
	}
	return errors.Join(closeErrors...)
}

func closePostgresPool(db *sql.DB) error {
	if db == nil {
		return nil
	}
	return errors.Join(telemetry.UnregisterDatabasePool(db), db.Close())
}

// NewDatabaseConnection establishes a PostgreSQL database connection.
//
// This function creates a database connection pool with optimized settings for high-concurrency
// applications. Database schema initialization is handled by the BaSyx configuration service.
//
// Connection pool settings:
//   - MaxOpenConns: 50 (maximum concurrent connections)
//   - MaxIdleConns: 25 (maximum idle connections in pool)
//   - ConnMaxLifetime: 5 minutes (connection recycling interval)
//
// Parameters:
//   - dsn: PostgreSQL Data Source Name (connection string)
//     Format: "postgres://user:password@host:port/dbname?sslmode=disable"
//
// Returns:
//   - *sql.DB: Configured database connection pool
//   - error: Error if connection fails
//
// Example:
//
//	dsn := "postgres://admin:password@localhost:5432/basyx_db?sslmode=disable"
//	db, err := NewDatabaseConnection(dsn)
//	if err != nil {
//	    return err
//	}
//	defer db.Close()
func NewDatabaseConnection(dsn string) (*sql.DB, error) {
	return NewDatabaseConnectionWithConfig(dsn, PostgresConfig{})
}

// NewDatabaseConnectionWithConfig opens a PostgreSQL pool for callers that do
// not have a request or process context available.
func NewDatabaseConnectionWithConfig(dsn string, cfg PostgresConfig) (*sql.DB, error) {
	encodedDSN := NormalizePostgresDSN(dsn)
	db, err := sql.Open("pgx", encodedDSN)
	if err != nil {
		return nil, err
	}

	if _, err = ConfigurePostgresPool(db, cfg); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, err
	}

	return db, nil
}

// ValidateSchemaVersion checks whether basyxsystem is clean and matches the expected schema version.
// Returns an error if the state/version is missing, unreadable, dirty, or does not match.
func ValidateSchemaVersion(db *sql.DB, expectedVersion string) error {
	query, trimmedExpected, err := prepareSchemaVersionCheck(db, expectedVersion)
	if err != nil {
		return err
	}
	return validateSchemaVersionRow(db.QueryRow(query), trimmedExpected)
}

// ValidateSchemaVersionContext checks the schema version with cancellation support.
func ValidateSchemaVersionContext(ctx context.Context, db *sql.DB, expectedVersion string) error {
	if ctx == nil {
		return fmt.Errorf("DB-CHECKVER-NOCONTEXT context is required")
	}
	query, trimmedExpected, err := prepareSchemaVersionCheck(db, expectedVersion)
	if err != nil {
		return err
	}
	return validateSchemaVersionRow(db.QueryRowContext(ctx, query), trimmedExpected)
}

func prepareSchemaVersionCheck(db *sql.DB, expectedVersion string) (string, string, error) {
	if db == nil {
		return "", "", fmt.Errorf("DB-CHECKVER-NILDB database handle is nil")
	}
	trimmedExpected := strings.TrimSpace(expectedVersion)
	if trimmedExpected == "" {
		return "", "", fmt.Errorf("DB-CHECKVER-NOEXPECTED expected version is empty")
	}

	query, _, err := goqu.Dialect("postgres").
		From(goqu.T("basyxsystem")).
		Select(goqu.C("schema_version"), goqu.C("state")).
		Order(goqu.C("identifier").Asc()).
		Limit(1).
		ToSQL()
	if err != nil {
		return "", "", fmt.Errorf("DB-CHECKVER-BUILDQUERY failed to build version query: %w", err)
	}
	return query, trimmedExpected, nil
}

func validateSchemaVersionRow(row *sql.Row, trimmedExpected string) error {
	var actualVersion string
	var schemaState string
	err := row.Scan(&actualVersion, &schemaState)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("DB-CHECKVER-NOVERSIONROW basyxsystem has no version row")
		}
		slog.Error("BaSyx Configuration Service schema metadata unavailable", "error.code", "COMMON-VALIDATESCHEMAVERSION-READMETADATA")
		slog.Error("verify that the configuration service completed successfully and uses the same database", "error.code", "COMMON-VALIDATESCHEMAVERSION-CHECKCONFIGSERVICE")
		return fmt.Errorf("DB-CHECKVER-READFAIL failed to read schema version: %w", err)
	}

	if strings.TrimSpace(schemaState) != cleanSchemaState {
		return fmt.Errorf(
			"DB-CHECKVER-DIRTYSTATE expected schema state %q but found %q",
			cleanSchemaState,
			strings.TrimSpace(schemaState),
		)
	}

	if strings.TrimSpace(actualVersion) != trimmedExpected {
		return fmt.Errorf(
			"DB-CHECKVER-MISMATCH expected schema version %q but found %q",
			trimmedExpected,
			strings.TrimSpace(actualVersion),
		)
	}

	return nil
}

// OpenPostgresWithSchemaValidation opens a shared pool and validates the schema
// through the same context-aware connection.
func OpenPostgresWithSchemaValidation(
	ctx context.Context,
	cfg PostgresConfig,
	serviceName string,
	expectedVersion string,
) (*sql.DB, error) {
	db, err := OpenPostgres(ctx, cfg, serviceName)
	if err != nil {
		return nil, err
	}
	if err = ValidateSchemaVersionContext(ctx, db, expectedVersion); err != nil {
		return nil, closePostgresAfterSchemaValidationFailure(db, err)
	}
	return db, nil
}

func closePostgresAfterSchemaValidationFailure(db *sql.DB, validationErr error) error {
	closeErr := closePostgresPool(db)
	if closeErr == nil {
		return validationErr
	}
	return fmt.Errorf(
		"COMMON-OPENPOSTGRES-CLOSE failed to close PostgreSQL connection pool after schema validation failure: %w",
		errors.Join(validationErr, closeErr),
	)
}

// ValidateSchemaVersionByDSN opens a temporary database connection and validates the schema version.
func ValidateSchemaVersionByDSN(dsn string, expectedVersion string) error {
	db, err := NewDatabaseConnection(dsn)
	if err != nil {
		return fmt.Errorf("DB-CHECKVER-CONNECTFAIL failed to connect while validating version: %w", err)
	}
	defer func() {
		_ = db.Close()
	}()

	return ValidateSchemaVersion(db, expectedVersion)
}

func StartTransaction(db *sql.DB) (*sql.Tx, func(*error), error) {
	tx, err := db.Begin()
	if err != nil {
		return nil, nil, err
	}
	cleanup := func(txErr *error) {
		if txErr != nil {
			_ = tx.Rollback()
		}
	}
	return tx, cleanup, nil
}
