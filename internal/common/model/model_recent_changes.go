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

package model

import (
	"github.com/FriedJannik/aas-go-sdk/jsonization"
	"github.com/FriedJannik/aas-go-sdk/types"
)

// RecentChange contains shared metadata for v3.2 recent-change responses.
type RecentChange struct {
	Type      string `json:"type,omitempty"`
	CreatedAt string `json:"createdAt,omitempty"`
	UpdatedAt string `json:"updatedAt,omitempty"`
}

// AssetAdministrationShellRecentChange describes a changed Asset Administration Shell.
type AssetAdministrationShellRecentChange struct {
	RecentChange
	Id               string           `json:"id,omitempty"`
	GlobalAssetId    string           `json:"globalAssetId,omitempty"`
	SpecificAssetIds []map[string]any `json:"specificAssetIds,omitempty"`
}

// SubmodelRecentChange describes a changed Submodel.
type SubmodelRecentChange struct {
	RecentChange
	Id                      string           `json:"id,omitempty"`
	SemanticId              map[string]any   `json:"semanticId,omitempty"`
	SupplementalSemanticIds []map[string]any `json:"supplementalSemanticIds,omitempty"`
}

// ConceptDescriptionRecentChange describes a changed Concept Description.
type ConceptDescriptionRecentChange struct {
	RecentChange
	Id string `json:"id,omitempty"`
}

// GetAllAssetAdministrationShellsRecentChangesResult is the paged AAS recent-change response.
type GetAllAssetAdministrationShellsRecentChangesResult struct {
	PagingMetadata PagedResultPagingMetadata              `json:"paging_metadata"`
	Result         []AssetAdministrationShellRecentChange `json:"result"`
}

// GetAllSubmodelRecentChangesResult is the paged Submodel recent-change response.
type GetAllSubmodelRecentChangesResult struct {
	PagingMetadata PagedResultPagingMetadata `json:"paging_metadata"`
	Result         []SubmodelRecentChange    `json:"result"`
}

// GetAllConceptDescriptionRecentChangesResult is the paged Concept Description recent-change response.
type GetAllConceptDescriptionRecentChangesResult struct {
	PagingMetadata PagedResultPagingMetadata        `json:"paging_metadata"`
	Result         []ConceptDescriptionRecentChange `json:"result"`
}

// JsonableSpecificAssetIDs converts SDK interfaces into JSON-ready recent-change fields.
func JsonableSpecificAssetIDs(assetIDs []types.ISpecificAssetID) ([]map[string]any, error) {
	result := make([]map[string]any, 0, len(assetIDs))
	for _, assetID := range assetIDs {
		jsonable, err := jsonization.ToJsonable(assetID)
		if err != nil {
			return nil, err
		}
		result = append(result, jsonable)
	}
	return result, nil
}

// JsonableReference converts an SDK interface into a JSON-ready recent-change field.
func JsonableReference(reference types.IReference) (map[string]any, error) {
	if reference == nil {
		return nil, nil
	}
	return jsonization.ToJsonable(reference)
}

// JsonableReferences converts SDK interfaces into JSON-ready recent-change fields.
func JsonableReferences(references []types.IReference) ([]map[string]any, error) {
	result := make([]map[string]any, 0, len(references))
	for _, reference := range references {
		jsonable, err := JsonableReference(reference)
		if err != nil {
			return nil, err
		}
		result = append(result, jsonable)
	}
	return result, nil
}
