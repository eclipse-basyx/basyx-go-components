/*******************************************************************************
* Copyright (C) 2025 the Eclipse BaSyx Authors and Fraunhofer IESE
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

// Package submodelelements provides persistence handlers for various submodel element types.
package submodelelements

import "database/sql"

// BaseHandler provides common functionality shared by all submodel element handlers.
// It uses the decorator pattern where type-specific handlers embed this base
// and extend it with specialized functionality.
//
// This eliminates code duplication across all handler types by centralizing
// the database connection and decorated handler initialization.
type BaseHandler struct {
	DB        *sql.DB
	Decorated *PostgreSQLSMECrudHandler
}

// NewBaseHandler creates a new BaseHandler instance with the provided database connection.
// It initializes the decorated PostgreSQLSMECrudHandler for base submodel element operations.
//
// Parameters:
//   - db: A PostgreSQL database connection
//
// Returns:
//   - *BaseHandler: A new handler instance
//   - error: An error if the decorated handler initialization fails
func NewBaseHandler(db *sql.DB) (*BaseHandler, error) {
	decorated, err := NewPostgreSQLSMECrudHandler(db)
	if err != nil {
		return nil, err
	}
	return &BaseHandler{DB: db, Decorated: decorated}, nil
}
