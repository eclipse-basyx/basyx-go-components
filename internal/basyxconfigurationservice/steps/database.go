// Package steps hold all Steps to initialize BaSyx
package steps

import (
	"fmt"
	"time"

	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	_ "github.com/lib/pq" // PostgreSQL driver registration for database/sql
)

// DatabaseConnection Step for connecting to the PGSQL DB.
type DatabaseConnection struct {
	ctx        *ExecutionContext
	configPath string
}

// NewDatabaseConnection initializes the Database Connection step.
func NewDatabaseConnection(ctx *ExecutionContext, configPath string) *DatabaseConnection {
	return &DatabaseConnection{ctx: ctx, configPath: configPath}
}

// Execute performs the action the step is assigned to.
func (dbc *DatabaseConnection) Execute(stepIndex int) (int, error) {
	cfg, err := common.LoadConfig(dbc.configPath, common.QUIET)
	if err != nil {
		return 1, fmt.Errorf("BASYXCFG-DB-LOADCONFIG: %w", err)
	}

	dsn := common.BuildPostgresDSN(cfg.Postgres)
	db, err := common.NewDatabaseConnection(dsn)
	if err != nil {
		return 1, fmt.Errorf("BASYXCFG-DB-CONNECT: %w", err)
	}

	if cfg.Postgres.MaxOpenConnections > 0 {
		db.SetMaxOpenConns(cfg.Postgres.MaxOpenConnections)
	}
	if cfg.Postgres.MaxIdleConnections > 0 {
		db.SetMaxIdleConns(cfg.Postgres.MaxIdleConnections)
	}
	if cfg.Postgres.ConnMaxLifetimeMinutes > 0 {
		db.SetConnMaxLifetime(time.Duration(cfg.Postgres.ConnMaxLifetimeMinutes) * time.Minute)
	}

	dbc.ctx.Config = cfg
	dbc.ctx.DB = db
	_, _ = fmt.Printf("[Step %d] Database connection established\n", stepIndex)

	return 0, nil
}

// GetDescription returns the step description for console output.
func (dbc *DatabaseConnection) GetDescription(stepIndex int) string {
	return fmt.Sprintf("[Step %d] Connecting to Database", stepIndex)
}
