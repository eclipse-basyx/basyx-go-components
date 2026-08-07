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
	"reflect"
	"sort"
	"strings"

	"github.com/FriedJannik/aas-go-sdk/types"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/history"
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

type submodelReconciliationPlan struct {
	Metadata                submodelReconciliationMetadata              `json:"metadata"`
	Updates                 []submodelelements.ReconciliationElementRow `json:"updates"`
	Inserts                 []submodelelements.ReconciliationElementRow `json:"inserts"`
	Deletes                 []string                                    `json:"deletes"`
	ExpectedDeletedElements int                                         `json:"expectedDeletedElements"`
}

type submodelReconciliationPlanJSON struct {
	Metadata                submodelReconciliationMetadata `json:"metadata"`
	Updates                 []reconciliationElementJSONRow `json:"updates"`
	Inserts                 []reconciliationElementJSONRow `json:"inserts"`
	Deletes                 []reconciliationDeleteJSONRow  `json:"deletes"`
	ExpectedDeletedElements int                            `json:"expectedDeletedElements"`
}

type reconciliationElementJSONRow struct {
	Row submodelelements.ReconciliationElementRow `json:"row"`
}

type reconciliationDeleteJSONRow struct {
	Path string `json:"path"`
}

func (p submodelReconciliationPlan) hasLiveMutation() bool {
	return p.Metadata.CoreChanged || p.Metadata.PayloadChanged || p.Metadata.SemanticIDChanged ||
		p.Metadata.SupplementalChanged || len(p.Updates) > 0 || len(p.Inserts) > 0 || len(p.Deletes) > 0
}

func (p submodelReconciliationPlan) marshal() ([]byte, error) {
	encoded, err := json.Marshal(submodelReconciliationPlanJSON{
		Metadata:                p.Metadata,
		Updates:                 wrapReconciliationElementRows(p.Updates),
		Inserts:                 wrapReconciliationElementRows(p.Inserts),
		Deletes:                 wrapReconciliationDeleteRows(p.Deletes),
		ExpectedDeletedElements: p.ExpectedDeletedElements,
	})
	if err != nil {
		return nil, common.NewInternalServerError("SMREPO-RECON-MARSHALPLAN " + err.Error())
	}
	return encoded, nil
}

func wrapReconciliationElementRows(rows []submodelelements.ReconciliationElementRow) []reconciliationElementJSONRow {
	result := make([]reconciliationElementJSONRow, len(rows))
	for index, row := range rows {
		result[index] = reconciliationElementJSONRow{Row: row}
	}
	return result
}

func wrapReconciliationDeleteRows(paths []string) []reconciliationDeleteJSONRow {
	result := make([]reconciliationDeleteJSONRow, len(paths))
	for index, path := range paths {
		result[index] = reconciliationDeleteJSONRow{Path: path}
	}
	return result
}

func (s *SubmodelDatabase) buildSubmodelReconciliationPlan(
	oldSubmodel types.ISubmodel,
	newSubmodel types.ISubmodel,
	oldSnapshot map[string]any,
	newSnapshot map[string]any,
) (submodelReconciliationPlan, error) {
	metadata, err := buildSubmodelReconciliationMetadata(newSubmodel, oldSnapshot, newSnapshot)
	if err != nil {
		return submodelReconciliationPlan{}, err
	}
	oldRows, err := submodelelements.BuildReconciliationElementRows(s.db, oldSubmodel.SubmodelElements())
	if err != nil {
		return submodelReconciliationPlan{}, err
	}
	newRows, err := submodelelements.BuildReconciliationElementRows(s.db, newSubmodel.SubmodelElements())
	if err != nil {
		return submodelReconciliationPlan{}, err
	}
	updates, inserts, deletes, err := reconcileSubmodelElementRows(oldRows, newRows)
	if err != nil {
		return submodelReconciliationPlan{}, err
	}
	return submodelReconciliationPlan{
		Metadata:                metadata,
		Updates:                 updates,
		Inserts:                 inserts,
		Deletes:                 deletes,
		ExpectedDeletedElements: countDeletedReconciliationRows(oldRows, deletes),
	}, nil
}

func countDeletedReconciliationRows(rows []submodelelements.ReconciliationElementRow, roots []string) int {
	count := 0
	for _, row := range rows {
		for _, root := range roots {
			if row.Path == root || strings.HasPrefix(row.Path, root+".") || strings.HasPrefix(row.Path, root+"[") {
				count++
				break
			}
		}
	}
	return count
}

func buildSubmodelReconciliationMetadata(
	newSubmodel types.ISubmodel,
	oldSnapshot map[string]any,
	newSnapshot map[string]any,
) (submodelReconciliationMetadata, error) {
	metadataPatch, err := history.BuildJSONPatch(withoutSubmodelElements(oldSnapshot), withoutSubmodelElements(newSnapshot))
	if err != nil {
		return submodelReconciliationMetadata{}, err
	}
	changedRoots := changedRootJSONFields(metadataPatch)
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
		CoreChanged:         hasAnyRoot(changedRoots, "idShort", "category", "kind"),
		PayloadChanged:      hasAnyRoot(changedRoots, "description", "displayName", "administration", "embeddedDataSpecifications", "supplementalSemanticIds", "extensions", "qualifiers"),
		SemanticIDChanged:   changedRoots["semanticId"],
		SupplementalChanged: changedRoots["supplementalSemanticIds"],
		IDShort:             newSubmodel.IDShort(),
		Category:            newSubmodel.Category(),
		Kind:                kind,
		Description:         nullableRawJSON(payload.description),
		DisplayName:         nullableRawJSON(payload.displayName),
		Administration:      nullableRawJSON(payload.administrativeInformation),
		EmbeddedDataSpecs:   nullableRawJSON(payload.embeddedDataSpecification),
		SupplementalIDs:     nullableRawJSON(payload.supplementalSemanticIDs),
		Extensions:          nullableRawJSON(payload.extensions),
		Qualifiers:          nullableRawJSON(payload.qualifiers),
		SemanticID:          semanticID,
		SupplementalRefs:    supplemental,
	}, nil
}

func withoutSubmodelElements(snapshot map[string]any) map[string]any {
	result := make(map[string]any, len(snapshot))
	for key, value := range snapshot {
		if key != "submodelElements" {
			result[key] = value
		}
	}
	return result
}

func changedRootJSONFields(patch []map[string]any) map[string]bool {
	result := make(map[string]bool)
	for _, operation := range patch {
		path, ok := operation["path"].(string)
		if !ok || !strings.HasPrefix(path, "/") {
			continue
		}
		token := strings.TrimPrefix(path, "/")
		if separator := strings.IndexByte(token, '/'); separator >= 0 {
			token = token[:separator]
		}
		token = strings.ReplaceAll(strings.ReplaceAll(token, "~1", "/"), "~0", "~")
		result[token] = true
	}
	return result
}

func hasAnyRoot(fields map[string]bool, names ...string) bool {
	for _, name := range names {
		if fields[name] {
			return true
		}
	}
	return false
}

func nullableRawJSON(value *string) json.RawMessage {
	if value == nil || !json.Valid([]byte(*value)) {
		return json.RawMessage("null")
	}
	return json.RawMessage(*value)
}

func reconcileSubmodelElementRows(
	oldRows []submodelelements.ReconciliationElementRow,
	newRows []submodelelements.ReconciliationElementRow,
) ([]submodelelements.ReconciliationElementRow, []submodelelements.ReconciliationElementRow, []string, error) {
	oldByPath := indexReconciliationRows(oldRows)
	retained := make(map[string]bool, len(newRows))
	inserted := make(map[string]bool, len(newRows))
	updates := make([]submodelelements.ReconciliationElementRow, 0)
	inserts := make([]submodelelements.ReconciliationElementRow, 0)

	for _, target := range newRows {
		parentInserted := target.ParentPath != "" && inserted[target.ParentPath]
		previous, exists := oldByPath[target.Path]
		if parentInserted || !exists || previous.ModelType != target.ModelType {
			target.Changes = submodelelements.AllReconciliationElementChanges()
			inserted[target.Path] = true
			inserts = append(inserts, target)
			continue
		}
		retained[target.Path] = true
		patch, err := history.BuildJSONPatch(previous, target)
		if err != nil {
			return nil, nil, nil, err
		}
		if len(patch) > 0 {
			target.Changes = reconciliationElementChanges(previous, target)
			updates = append(updates, target)
		}
	}

	deleteCandidates := make(map[string]bool)
	for _, previous := range oldRows {
		if !retained[previous.Path] {
			deleteCandidates[previous.Path] = true
		}
	}
	deletes := make([]string, 0, len(deleteCandidates))
	for _, previous := range oldRows {
		if !deleteCandidates[previous.Path] {
			continue
		}
		if previous.ParentPath != "" && deleteCandidates[previous.ParentPath] {
			continue
		}
		deletes = append(deletes, previous.Path)
	}
	sort.Strings(deletes)
	return updates, inserts, deletes, nil
}

func reconciliationElementChanges(
	previous submodelelements.ReconciliationElementRow,
	target submodelelements.ReconciliationElementRow,
) submodelelements.ReconciliationElementChanges {
	return submodelelements.ReconciliationElementChanges{
		Core: previous.Position != target.Position || previous.IDShort != target.IDShort ||
			!reflect.DeepEqual(previous.Category, target.Category),
		Payload:        !reflect.DeepEqual(previous.Payload, target.Payload),
		SemanticID:     !reflect.DeepEqual(previous.SemanticID, target.SemanticID),
		SupplementalID: !reflect.DeepEqual(previous.SupplementalSemanticIDs, target.SupplementalSemanticIDs),
		TypeData:       previous.TypeTable != target.TypeTable || !reflect.DeepEqual(previous.TypeData, target.TypeData),
		LanguageValues: !reflect.DeepEqual(previous.LanguageValues, target.LanguageValues),
		ValueID:        !reflect.DeepEqual(previous.ValueID, target.ValueID),
	}
}

func indexReconciliationRows(rows []submodelelements.ReconciliationElementRow) map[string]submodelelements.ReconciliationElementRow {
	result := make(map[string]submodelelements.ReconciliationElementRow, len(rows))
	for _, row := range rows {
		result[row.Path] = row
	}
	return result
}
