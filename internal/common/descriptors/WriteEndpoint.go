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
// Author: Martin Stemmer ( Fraunhofer IESE )

package descriptors

import (
	"database/sql"

	"github.com/doug-martin/goqu/v9"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model"
)

func createEndpointAttributes(tx *sql.Tx, endpointID int64, securityAttributes []model.ProtocolInformationSecurityAttributes) error {
	if len(securityAttributes) == 0 {
		return nil
	}
	d := goqu.Dialect(dialect)
	rows := make([]goqu.Record, 0, len(securityAttributes))
	for _, val := range securityAttributes {
		rows = append(rows, goqu.Record{
			colEndpointID:    endpointID,
			colSecurityType:  val.Type,
			colSecurityKey:   val.Key,
			colSecurityValue: val.Value,
		})
	}
	sqlStr, args, err := d.Insert(tblSecurityAttributes).Rows(rows).ToSQL()
	if err != nil {
		return err
	}
	_, err = tx.Exec(sqlStr, args...)
	return err
}

func createEndpointProtocolVersion(tx *sql.Tx, endpointID int64, endpointProtocolVersion []string) error {
	if len(endpointProtocolVersion) == 0 {
		return nil
	}
	d := goqu.Dialect(dialect)
	rows := make([]goqu.Record, 0, len(endpointProtocolVersion))
	for _, val := range endpointProtocolVersion {
		rows = append(rows, goqu.Record{
			colEndpointID:              endpointID,
			colEndpointProtocolVersion: val,
		})
	}
	sqlStr, args, err := d.Insert(tblEndpointProtocolVersion).Rows(rows).ToSQL()
	if err != nil {
		return err
	}
	_, err = tx.Exec(sqlStr, args...)
	return err
}

// CreateEndpoints inserts a list of endpoints for a descriptor into the
// database within the provided transaction. For each endpoint the base
// endpoint row is inserted into the `tblAASDescriptorEndpoint` table and the
// generated row id is used to insert related rows:
//   - protocol version(s) via `createEndpointProtocolVersion`
//   - security attributes via `createEndpointAttributes`
//
// The function is safe to call with a nil slice (no-op) and returns any
// SQL/DB error encountered. The caller is responsible for managing the
// surrounding transaction (commit/rollback).
//
// Parameters:
//   - tx: active SQL transaction used for all inserts
//   - descriptorID: internal descriptor id to associate endpoints with
//   - endpoints: slice of model.Endpoint to persist; the order in the slice
//     is stored in the `colPosition` column
//
// Returns:
//   - error: non-nil if any insert or subsequent dependent write fails
func CreateEndpoints(tx *sql.Tx, descriptorID int64, endpoints []model.Endpoint) error {
	if endpoints == nil {
		return nil
	}
	if len(endpoints) > 0 {
		d := goqu.Dialect(dialect)
		for i, val := range endpoints {
			sqlStr, args, err := d.
				Insert(tblAASDescriptorEndpoint).
				Rows(goqu.Record{
					colDescriptorID:            descriptorID,
					colPosition:                i,
					colHref:                    val.ProtocolInformation.Href,
					colEndpointProtocol:        val.ProtocolInformation.EndpointProtocol,
					colSubProtocol:             val.ProtocolInformation.Subprotocol,
					colSubProtocolBody:         val.ProtocolInformation.SubprotocolBody,
					colSubProtocolBodyEncoding: val.ProtocolInformation.SubprotocolBodyEncoding,
					colInterface:               val.Interface,
				}).
				Returning(tAASDescriptorEndpoint.Col(colID)).
				ToSQL()
			if err != nil {
				return err
			}
			var id int64
			if err = tx.QueryRow(sqlStr, args...).Scan(&id); err != nil {
				return err
			}
			if err = createEndpointProtocolVersion(tx, id, val.ProtocolInformation.EndpointProtocolVersion); err != nil {
				return err
			}
			if err = createEndpointAttributes(tx, id, val.ProtocolInformation.SecurityAttributes); err != nil {
				return err
			}
		}
	}
	return nil
}
