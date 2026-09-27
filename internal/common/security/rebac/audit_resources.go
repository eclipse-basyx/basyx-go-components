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
// Author: Aaron Zielstorff ( Fraunhofer IESE )

package rebac

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/doug-martin/goqu/v9"
)

// resolveAuditResources adds the public identity of each event's object
// while the object still exists, with one query per object type.
func resolveAuditResources(ctx context.Context, q Queryer, events []AuditEvent) error {
	uuidsByType := map[string][]any{}
	for _, event := range events {
		if objectType, authUUID, ok := auditObjectUUID(event.Object); ok {
			uuidsByType[objectType] = append(uuidsByType[objectType], goqu.L("?::uuid", authUUID))
		}
	}
	identifiers := map[string]string{}
	for objectType, uuids := range uuidsByType {
		if err := lookupAuditIdentifiers(ctx, q, objectType, uuids, identifiers); err != nil {
			return err
		}
	}
	for i := range events {
		events[i].Resource = auditResource(events[i], identifiers)
	}
	return nil
}

// auditObjectUUID returns the resource type and authorization UUID an
// object key refers to. Element keys refer to their Submodel.
func auditObjectUUID(objectKey string) (string, string, bool) {
	objectType, rest, found := strings.Cut(objectKey, ":")
	if !found {
		return "", "", false
	}
	if objectType == TypeElement {
		submodelUUID, _, _ := strings.Cut(rest, ".")
		return TypeSubmodel, submodelUUID, true
	}
	if _, covered := KindForObjectType(objectType); !covered {
		return "", "", false
	}
	return objectType, rest, true
}

// lookupAuditIdentifiers stores the identifiers of the given resources of
// one type in identifiers, keyed by their resource key.
func lookupAuditIdentifiers(ctx context.Context, q Queryer, objectType string, uuids []any, identifiers map[string]string) error {
	kind, _ := KindForObjectType(objectType)
	ds := dialect.From(kind.Rows().As("resource")).
		Select(goqu.L("resource.object_uuid::text"), goqu.I("resource.identifier")).
		Where(goqu.I("resource.object_uuid").In(uuids...)).
		Prepared(true)
	rows, err := queryDataset(ctx, q, "REBAC-AUDITRESOURCES", ds)
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var authUUID, identifier string
		if err = rows.Scan(&authUUID, &identifier); err != nil {
			return fmt.Errorf("REBAC-AUDITRESOURCES-SCAN: %w", err)
		}
		identifiers[ResourceKey(objectType, authUUID)] = identifier
	}
	return rows.Err()
}

// auditResource returns the public identity of an event's object, or nil
// when the object no longer exists or concerns all of ReBAC.
func auditResource(event AuditEvent, identifiers map[string]string) *AuditResource {
	objectType, rest, _ := strings.Cut(event.Object, ":")
	if objectType == TypeRepository {
		return &AuditResource{Type: TypeRepository, ID: rest}
	}
	resourceType, authUUID, ok := auditObjectUUID(event.Object)
	if !ok {
		return nil
	}
	identifier, found := identifiers[ResourceKey(resourceType, authUUID)]
	if !found {
		return nil
	}
	if objectType != TypeElement {
		return &AuditResource{Type: objectType, ID: identifier}
	}
	var details struct {
		IDShortPath string `json:"idShortPath"`
	}
	_ = json.Unmarshal(event.Details, &details)
	return &AuditResource{Type: TypeElement, ID: identifier, IDShortPath: details.IDShortPath}
}
