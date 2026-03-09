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
// Author: Martin Stemmer ( Fraunhofer IESE )

package descriptors

import (
	"database/sql"
	"encoding/json"

	"github.com/doug-martin/goqu/v9"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model"
)

func marshalProtocolVersions(versions []string) (string, error) {
	if versions == nil {
		versions = []string{}
	}
	encoded, err := json.Marshal(versions)
	if err != nil {
		return "", err
	}
	return string(encoded), nil
}

func marshalSecurityAttributes(attrs []model.ProtocolInformationSecurityAttributes) (string, error) {
	if attrs == nil {
		attrs = []model.ProtocolInformationSecurityAttributes{}
	}
	encoded, err := json.Marshal(attrs)
	if err != nil {
		return "", err
	}
	return string(encoded), nil
}

// CreateEndpoints inserts a list of endpoints for a descriptor into the
// database within the provided transaction. Each endpoint is stored in
// `common.TblAASDescriptorEndpoint` with protocol versions and security attributes
// persisted as JSONB columns.
//
// The function is safe to call with a nil slice (no-op) and returns any
// SQL/DB error encountered. The caller is responsible for managing the
// surrounding transaction (commit/rollback).
//
// Parameters:
//   - tx: active SQL transaction used for all inserts
//   - descriptorID: internal descriptor id to associate endpoints with
//   - endpoints: slice of model.Endpoint to persist; the order in the slice
//     is stored in the `common.ColPosition` column
//
// Returns:
//   - error: non-nil if any insert or subsequent dependent write fails
func CreateEndpoints(tx *sql.Tx, descriptorID int64, endpoints []model.Endpoint) error {
	if endpoints == nil {
		return nil
	}
	if len(endpoints) > 0 {
		d := goqu.Dialect(common.Dialect)
		for i, val := range endpoints {
			versionsJSON, err := marshalProtocolVersions(val.ProtocolInformation.EndpointProtocolVersion)
			if err != nil {
				return err
			}
			securityAttrsJSON, err := marshalSecurityAttributes(val.ProtocolInformation.SecurityAttributes)
			if err != nil {
				return err
			}
			sqlStr, args, err := d.
				Insert(common.TblAASDescriptorEndpoint).
				Rows(goqu.Record{
					common.ColDescriptorID:            descriptorID,
					common.ColPosition:                i,
					common.ColHref:                    val.ProtocolInformation.Href,
					common.ColEndpointProtocol:        val.ProtocolInformation.EndpointProtocol,
					common.ColEndpointProtocolVersion: goqu.L("?::jsonb", versionsJSON),
					common.ColSubProtocol:             val.ProtocolInformation.Subprotocol,
					common.ColSubProtocolBody:         val.ProtocolInformation.SubprotocolBody,
					common.ColSubProtocolBodyEncoding: val.ProtocolInformation.SubprotocolBodyEncoding,
					common.ColSecurityAttributes:      goqu.L("?::jsonb", securityAttrsJSON),
					common.ColInterface:               val.Interface,
				}).
				Returning(common.TAASDescriptorEndpoint.Col(common.ColID)).
				ToSQL()
			if err != nil {
				return err
			}
			var id int64
			if err = tx.QueryRow(sqlStr, args...).Scan(&id); err != nil {
				return err
			}
		}
	}
	return nil
}
