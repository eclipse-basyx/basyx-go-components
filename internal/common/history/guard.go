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

package history

import (
	"context"
	"database/sql"

	"github.com/doug-martin/goqu/v9"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
)

// ApplyPostgresGuardConfig synchronizes database-side history mutation guards.
//
// A service configured for postgres_guarded or external_anchor immutability can
// enable the shared database guard at startup. A service without guarded
// immutability can keep an already-disabled guard disabled, but it cannot
// downgrade a database where another service has enabled the guard.
//
// Parameters:
//   - ctx: Startup context used for the guard configuration write.
//   - db: Database handle connected to the shared BaSyx database.
//
// Returns:
//   - error: Error when db is nil, the guard row cannot be written, or startup
//     attempts to run an unguarded service against a guarded database.
//
// Example:
//
//	Configure(Config{Mode: ModeAudit, Immutability: ImmutabilityPostgresGuarded})
//	if err := ApplyPostgresGuardConfig(ctx, db); err != nil {
//		return err
//	}
func ApplyPostgresGuardConfig(ctx context.Context, db *sql.DB) error {
	if db == nil {
		return common.NewInternalServerError("HISTORY-GUARD-NILDB database handle must not be nil")
	}

	enabled := postgresGuardEnabled(ActiveConfig())
	query, args, err := goqu.Insert(TableGuardConfig).
		Rows(goqu.Record{
			"id":         true,
			"enabled":    enabled,
			"updated_at": goqu.L("NOW()"),
		}).
		OnConflict(goqu.DoUpdate("id", goqu.Record{
			"enabled":    goqu.L("history_guard_config.enabled OR EXCLUDED.enabled"),
			"updated_at": goqu.L("NOW()"),
		})).
		Returning(goqu.C("enabled")).
		ToSQL()
	if err != nil {
		return common.NewInternalServerError("HISTORY-GUARD-BUILDUPSERT " + err.Error())
	}
	var databaseGuardEnabled bool
	if err = db.QueryRowContext(ctx, query, args...).Scan(&databaseGuardEnabled); err != nil {
		return common.NewInternalServerError("HISTORY-GUARD-EXECUPSERT " + err.Error())
	}
	if !enabled && databaseGuardEnabled {
		return common.NewInternalServerError("HISTORY-GUARD-CONFLICT database history guard is enabled but this service is configured without postgres_guarded immutability")
	}
	return nil
}

func postgresGuardEnabled(cfg Config) bool {
	if cfg.Mode == ModeOff {
		return false
	}
	return cfg.Immutability == ImmutabilityPostgresGuarded || cfg.Immutability == ImmutabilityExternalAnchor
}
