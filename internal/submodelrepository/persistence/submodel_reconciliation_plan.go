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
// Author: Jannik Fried (Fraunhofer IESE)

package persistence

import (
	"encoding/json"

	"github.com/FriedJannik/aas-go-sdk/types"
	submodelelements "github.com/eclipse-basyx/basyx-go-components/internal/submodelrepository/persistence/submodelElements"
)

type submodelReconciliationMetadata struct {
	CoreChanged         bool                                       `json:"coreChanged"`
	PayloadChanged      bool                                       `json:"payloadChanged"`
	SemanticIDChanged   bool                                       `json:"semanticIdChanged"`
	SupplementalChanged bool                                       `json:"supplementalChanged"`
	IDShort             *string                                    `json:"idShort"`
	Category            *string                                    `json:"category"`
	Kind                *int                                       `json:"kind"`
	Description         json.RawMessage                            `json:"description"`
	DisplayName         json.RawMessage                            `json:"displayName"`
	Administration      json.RawMessage                            `json:"administration"`
	EmbeddedDataSpecs   json.RawMessage                            `json:"embeddedDataSpecifications"`
	SupplementalIDs     json.RawMessage                            `json:"supplementalSemanticIds"`
	Extensions          json.RawMessage                            `json:"extensions"`
	Qualifiers          json.RawMessage                            `json:"qualifiers"`
	SemanticID          *submodelelements.ReconciliationReference  `json:"semanticId"`
	SupplementalRefs    []submodelelements.ReconciliationReference `json:"supplementalReferences"`
}

func buildSubmodelReconciliationTargetMetadata(newSubmodel types.ISubmodel) (submodelReconciliationMetadata, error) {
	payload, err := jsonizeSubmodelPayload(newSubmodel)
	if err != nil {
		return submodelReconciliationMetadata{}, err
	}
	semanticID, err := submodelelements.BuildReconciliationReference(newSubmodel.SemanticID(), true)
	if err != nil {
		return submodelReconciliationMetadata{}, err
	}
	supplemental, err := submodelelements.BuildReconciliationReferences(newSubmodel.SupplementalSemanticIDs())
	if err != nil {
		return submodelReconciliationMetadata{}, err
	}
	var kind *int
	if newSubmodel.Kind() != nil {
		kindValue := int(*newSubmodel.Kind())
		kind = &kindValue
	}
	return submodelReconciliationMetadata{
		IDShort:           newSubmodel.IDShort(),
		Category:          newSubmodel.Category(),
		Kind:              kind,
		Description:       nullableRawJSON(payload.description),
		DisplayName:       nullableRawJSON(payload.displayName),
		Administration:    nullableRawJSON(payload.administrativeInformation),
		EmbeddedDataSpecs: nullableRawJSON(payload.embeddedDataSpecification),
		SupplementalIDs:   nullableRawJSON(payload.supplementalSemanticIDs),
		Extensions:        nullableRawJSON(payload.extensions),
		Qualifiers:        nullableRawJSON(payload.qualifiers),
		SemanticID:        semanticID,
		SupplementalRefs:  supplemental,
	}, nil
}

func nullableRawJSON(value *string) json.RawMessage {
	if value == nil || !json.Valid([]byte(*value)) {
		return json.RawMessage("null")
	}
	return json.RawMessage(*value)
}
