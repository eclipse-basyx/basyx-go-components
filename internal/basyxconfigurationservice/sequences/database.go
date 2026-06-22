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

// Package sequences holds initialization steps to initialize BaSyx
package sequences

import (
	"fmt"
	"time"

	"github.com/eclipse-basyx/basyx-go-components/internal/common"
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
