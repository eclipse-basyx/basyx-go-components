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
// Author: Jannik Fried (Fraunhofer IESE), Aaron Zielstorff (Fraunhofer IESE)

// Package persistence contains the implementation of the SubmodelRepositoryDatabase interface using PostgreSQL as the underlying database.
package persistence

import (
	"context"
	"crypto/rsa"
	"database/sql"

	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/jws"
	gen "github.com/eclipse-basyx/basyx-go-components/internal/common/model"
)

// SubmodelDatabase is the implementation of the SubmodelRepositoryDatabase interface using PostgreSQL as the underlying database.
type SubmodelDatabase struct {
	db               *sql.DB
	readerDB         *sql.DB
	privateKey       *rsa.PrivateKey
	signingOptions   jws.SigningOptions
	verificationMode gen.VerificationMode
}

// SetJWSCertificateChain configures the optional certificate chain embedded in
// signed Submodel responses.
//
// The provided slice must already be formatted for the JWS "x5c" protected
// header: each entry is a base64 encoded DER certificate, ordered from signer
// certificate to issuer certificates. Passing nil or an empty slice disables the
// "x5c" header while still allowing signed responses when a private key is
// configured.
//
// Parameters:
//   - certificateChain: Base64 encoded DER certificate chain for the JWS "x5c"
//     protected header.
//
// Returns:
//   - None.
func (s *SubmodelDatabase) SetJWSCertificateChain(certificateChain []string) {
	s.signingOptions.CertificateChain = certificateChain
}

// NewSubmodelDatabase creates a new instance of SubmodelDatabase with the provided database connection.
func NewSubmodelDatabase(dsn string, maxOpenConnections int, maxIdleConnections int, connMaxLifetimeMinutes int, privateKey *rsa.PrivateKey, strictVerification string) (*SubmodelDatabase, error) {
	db, err := common.NewDatabaseConnectionWithConfig(dsn, common.PostgresConfig{
		MaxOpenConnections:     maxOpenConnections,
		MaxIdleConnections:     maxIdleConnections,
		ConnMaxLifetimeMinutes: connMaxLifetimeMinutes,
	})
	if err != nil {
		return nil, err
	}

	return NewSubmodelDatabaseFromDB(db, privateKey, strictVerification)
}

// NewSubmodelDatabaseFromDB creates a new repository backend from an existing DB pool.
func NewSubmodelDatabaseFromDB(db *sql.DB, privateKey *rsa.PrivateKey, strictVerification string) (*SubmodelDatabase, error) {
	if db == nil {
		return nil, common.NewErrBadRequest("SMREPO-NEWFROMDB-NILDB database handle must not be nil")
	}
	return NewSubmodelDatabaseFromPools(db, db, privateKey, strictVerification)
}

// NewSubmodelDatabaseFromPools creates a repository backend from caller-owned
// writer and reader pools.
func NewSubmodelDatabaseFromPools(writer *sql.DB, reader *sql.DB, privateKey *rsa.PrivateKey, strictVerification string) (*SubmodelDatabase, error) {
	if writer == nil {
		return nil, common.NewErrBadRequest("SMREPO-NEWFROMPOOLS-NILWRITER writer database handle must not be nil")
	}
	if reader == nil {
		return nil, common.NewErrBadRequest("SMREPO-NEWFROMPOOLS-NILREADER reader database handle must not be nil")
	}

	verificationMode, err := gen.ParseVerificationMode(strictVerification)
	if err != nil {
		return nil, common.NewErrBadRequest("SMREPO-NEWFROMDB-INVALIDMODE " + err.Error())
	}

	return &SubmodelDatabase{
		db:               writer,
		readerDB:         reader,
		privateKey:       privateKey,
		verificationMode: verificationMode,
	}, nil
}

func (s *SubmodelDatabase) readDB(ctx context.Context) *sql.DB {
	return common.PostgresReadPool(ctx, s.db, s.readerDB)
}
