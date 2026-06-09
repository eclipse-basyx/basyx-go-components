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
	"context"
	"database/sql"
	"errors"
	"strings"

	"github.com/FriedJannik/aas-go-sdk/types"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/history"
	gen "github.com/eclipse-basyx/basyx-go-components/internal/common/model"
	auth "github.com/eclipse-basyx/basyx-go-components/internal/common/security"
	submodelpath "github.com/eclipse-basyx/basyx-go-components/internal/submodelrepository/path"
	submodelqueries "github.com/eclipse-basyx/basyx-go-components/internal/submodelrepository/persistence/queries"
	submodelelements "github.com/eclipse-basyx/basyx-go-components/internal/submodelrepository/persistence/submodelElements"
	persistenceutils "github.com/eclipse-basyx/basyx-go-components/internal/submodelrepository/persistence/utils"
)

func (s *SubmodelDatabase) addTopLevelSubmodelElementInTransaction(tx *sql.Tx, submodelID string, submodelElement types.ISubmodelElement) (string, error) {
	submodelDatabaseID, err := persistenceutils.GetSubmodelDatabaseID(tx, submodelID)
	if err != nil {
		if err == sql.ErrNoRows {
			return "", common.NewErrNotFound("SMREPO-ADDSME-SMNOTFOUND Submodel with ID '" + submodelID + "' not found")
		}
		return "", err
	}

	selectQuery, selectArgs, err := submodelqueries.BuildTopLevelSubmodelElementMaxPositionSQL(submodelDatabaseID)
	if err != nil {
		return "", err
	}

	var maxPosition sql.NullInt64
	err = tx.QueryRow(selectQuery, selectArgs...).Scan(&maxPosition)
	if err != nil {
		return "", err
	}

	startPosition := 0
	if maxPosition.Valid {
		startPosition = int(maxPosition.Int64) + 1
	}

	if isSiblingIDShortCollision(tx, submodelDatabaseID, nil, submodelElement) {
		return "", common.NewErrConflict("SMREPO-ADDSME-COLLISION Duplicate submodel element idShort")
	}

	_, err = submodelelements.InsertSubmodelElements(
		s.db,
		submodelID,
		[]types.ISubmodelElement{submodelElement},
		tx,
		&submodelelements.BatchInsertContext{
			StartPosition: startPosition,
		},
	)
	if err != nil {
		return "", err
	}

	idShort := submodelElement.IDShort()
	if idShort == nil {
		return "", nil
	}

	return *idShort, nil
}

func (s *SubmodelDatabase) updateSubmodelElementInTransaction(tx *sql.Tx, submodelID string, idShortOrPath string, submodelElement types.ISubmodelElement, isPut bool) error {
	modelType, err := getSMEModelTypeByPathInTx(tx, submodelID, idShortOrPath)
	if err != nil {
		return err
	}

	if modelType == nil {
		return common.NewErrNotFound("SMREPO-UPDSME-NOTFOUND Submodel-Element ID-Short: " + idShortOrPath)
	}

	handler, err := submodelelements.GetSMEHandlerByModelType(*modelType, s.db)
	if err != nil {
		return err
	}

	return handler.Update(submodelID, idShortOrPath, submodelElement, tx, isPut)
}

// GetSubmodelElement retrieves a submodel element by path and applies optional ABAC formula filters from ctx.
func (s *SubmodelDatabase) GetSubmodelElement(ctx context.Context, submodelID string, idShortOrPath string, _ bool, level string) (types.ISubmodelElement, error) {
	return submodelelements.GetSubmodelElementByIDShortOrPath(ctx, s.db, submodelID, idShortOrPath, level)
}

// GetSubmodelElements retrieves submodel elements and applies optional ABAC formula filters from ctx.
func (s *SubmodelDatabase) GetSubmodelElements(ctx context.Context, submodelID string, limit *int, cursor string, _ bool, level string) ([]types.ISubmodelElement, string, error) {
	return submodelelements.GetSubmodelElementsBySubmodelID(ctx, s.db, submodelID, limit, cursor, level)
}

// GetSubmodelElementPaths retrieves submodel element paths directly from persisted idshort_path values.
func (s *SubmodelDatabase) GetSubmodelElementPaths(ctx context.Context, submodelID string, level string) ([]string, error) {
	return submodelelements.GetSubmodelElementPathsBySubmodelID(ctx, s.db, submodelID, level)
}

// GetSubmodelElementPathPage retrieves paged submodel element paths directly from persisted idshort_path values.
func (s *SubmodelDatabase) GetSubmodelElementPathPage(ctx context.Context, submodelID string, limit *int, cursor string, level string) ([]string, string, error) {
	return submodelelements.GetSubmodelElementPathsPageBySubmodelID(ctx, s.db, submodelID, limit, cursor, level)
}

// GetSubmodelElementPathsByPath retrieves path notation for a specific submodel element path.
func (s *SubmodelDatabase) GetSubmodelElementPathsByPath(ctx context.Context, submodelID string, idShortPath string, level string) ([]string, error) {
	return submodelelements.GetSubmodelElementPathsByPath(ctx, s.db, submodelID, idShortPath, level)
}

// GetSubmodelElementReferences retrieves SME references and applies optional ABAC formula filters from ctx.
func (s *SubmodelDatabase) GetSubmodelElementReferences(ctx context.Context, submodelID string, limit *int, cursor string) ([]types.IReference, string, error) {
	return submodelelements.GetSubmodelElementReferencesBySubmodelID(ctx, s.db, submodelID, limit, cursor)
}

// AddSubmodelElement adds a top-level submodel element and performs an ABAC re-check before commit when ABAC is enabled.
func (s *SubmodelDatabase) AddSubmodelElement(ctx context.Context, submodelID string, submodelElement types.ISubmodelElement) (err error) {
	tx, cleanup, err := common.StartTransaction(s.db)
	if err != nil {
		return err
	}
	defer cleanup(&err)

	insertedPath, err := s.addTopLevelSubmodelElementInTransaction(tx, submodelID, submodelElement)
	if err != nil {
		return err
	}

	shouldEnforce, enforceErr := shouldEnforceFormula(ctx, "SMREPO-ADDSME-SHOULDENFORCE")
	if enforceErr != nil {
		return enforceErr
	}
	if shouldEnforce && insertedPath != "" {
		exists, visible, visErr := s.checkSubmodelElementVisibilityInTx(ctx, tx, submodelID, insertedPath)
		if visErr != nil {
			return visErr
		}
		if !exists {
			return common.NewInternalServerError("SMREPO-ADDSME-ABACCHECKMISSING created submodel element not found before commit")
		}
		if !visible {
			return common.NewErrDenied("SMREPO-ADDSME-ABACDENIED Created submodel element is not accessible under ABAC constraints")
		}
	}

	if insertedPath == "" {
		err = s.appendCurrentSubmodelHistoryTx(ctx, tx, submodelID, history.ChangeUpdated)
	} else {
		err = s.appendChangedSubmodelElementHistoryTx(ctx, tx, submodelID, submodelElementRootMutation{
			currentPath: insertedPath,
		})
	}
	if err != nil {
		return err
	}

	return tx.Commit()
}

func (s *SubmodelDatabase) addSubmodelElementWithPathInTransaction(ctx context.Context, tx *sql.Tx, submodelID string, submodelDatabaseID int, parentPath string, submodelElement types.ISubmodelElement) error {
	baseCrudHandler, err := submodelelements.NewPostgreSQLSMECrudHandler(s.db)
	if err != nil {
		return err
	}

	parentElementID, err := baseCrudHandler.GetDatabaseID(submodelDatabaseID, parentPath)
	if err != nil {
		return err
	}

	rootSmeID, err := baseCrudHandler.GetRootSmeIDByElementID(parentElementID)
	if err != nil {
		return err
	}

	parentElement, err := submodelelements.GetSubmodelElementByIDShortOrPath(ctx, s.db, submodelID, parentPath, "")
	if err != nil {
		return err
	}

	isFromList := false
	switch parentElement.ModelType() {
	case types.ModelTypeSubmodelElementCollection, types.ModelTypeEntity, types.ModelTypeAnnotatedRelationshipElement:
		isFromList = false
	case types.ModelTypeSubmodelElementList:
		isFromList = true
	default:
		return common.NewErrBadRequest("SMREPO-ADDSMEBYPATH-BADPARENT Parent element does not support child elements")
	}

	nextPosition, err := baseCrudHandler.GetNextPosition(parentElementID)
	if err != nil {
		return err
	}

	if isSiblingIDShortCollision(tx, submodelDatabaseID, &parentElementID, submodelElement) {
		return common.NewErrConflict("SMREPO-ADDSMEBYPATH-COLLISION Duplicate submodel element idShort")
	}

	_, err = submodelelements.InsertSubmodelElements(
		s.db,
		submodelID,
		[]types.ISubmodelElement{submodelElement},
		tx,
		&submodelelements.BatchInsertContext{
			ParentID:      parentElementID,
			ParentPath:    parentPath,
			RootSmeID:     rootSmeID,
			IsFromList:    isFromList,
			StartPosition: nextPosition,
		},
	)
	if err != nil {
		return err
	}

	return nil
}

// AddSubmodelElementWithPath adds a submodel element under an existing container path
// while preserving ABAC visibility checks from ctx.
func (s *SubmodelDatabase) AddSubmodelElementWithPath(ctx context.Context, submodelID string, parentPath string, submodelElement types.ISubmodelElement) error {
	tx, cleanup, err := common.StartTransaction(s.db)
	if err != nil {
		return err
	}
	defer cleanup(&err)

	submodelDatabaseID, err := persistenceutils.GetSubmodelDatabaseID(tx, submodelID)
	if err != nil {
		if err == sql.ErrNoRows {
			return common.NewErrNotFound("SMREPO-ADDSMEBYPATH-SMNOTFOUND Submodel with ID '" + submodelID + "' not found")
		}
		return err
	}

	err = s.addSubmodelElementWithPathInTransaction(ctx, tx, submodelID, submodelDatabaseID, parentPath, submodelElement)
	if err != nil {
		return err
	}

	if err = s.appendChangedSubmodelElementHistoryTx(ctx, tx, submodelID, submodelElementRootMutation{
		previousPath: parentPath,
		currentPath:  parentPath,
	}); err != nil {
		return err
	}

	return tx.Commit()
}

// PutSubmodelElement creates or replaces a submodel element at the requested path in a single transaction.
// It returns true when an existing element was updated and false when a new one was created.
func (s *SubmodelDatabase) PutSubmodelElement(
	ctx context.Context,
	submodelID string,
	idShortPath string,
	submodelElement types.ISubmodelElement,
) (isUpdate bool, err error) {
	tx, cleanup, err := common.StartTransaction(s.db)
	if err != nil {
		return false, err
	}
	defer cleanup(&err)

	submodelDatabaseID, err := persistenceutils.GetSubmodelDatabaseID(tx, submodelID)
	if err != nil {
		if err == sql.ErrNoRows {
			return false, common.NewErrNotFound("SMREPO-PUTSME-SMNOTFOUND Submodel with ID '" + submodelID + "' not found")
		}
		return false, err
	}

	elementExists := false
	historyMutation := submodelElementRootMutation{}
	shouldEnforce, enforceErr := shouldEnforceFormula(ctx, "SMREPO-PUTSME-SHOULDENFORCE")
	if enforceErr != nil {
		return false, enforceErr
	}

	if shouldEnforce {
		exists, _, visErr := s.checkSubmodelElementVisibilityInTx(ctx, tx, submodelID, idShortPath)
		if visErr != nil {
			return false, visErr
		}
		elementExists = exists
		ctx = auth.SelectPutFormulaByExistence(ctx, elementExists)
		if elementExists {
			_, visible, visErr := s.checkSubmodelElementVisibilityInTx(ctx, tx, submodelID, idShortPath)
			if visErr != nil {
				return false, visErr
			}
			if !visible {
				return false, common.NewErrDenied("SMREPO-PUTSME-ABACDENIED Existing submodel element is not accessible under ABAC constraints")
			}
		}
	} else {
		elementExists, err = submodelElementPathExistsInTx(tx, submodelDatabaseID, idShortPath)
		if err != nil {
			return false, err
		}
	}

	if elementExists {
		if err = s.updateSubmodelElementInTransaction(tx, submodelID, idShortPath, submodelElement, true); err != nil {
			return false, err
		}
		historyMutation.previousPath = idShortPath
		historyMutation.currentPath = submodelelements.ResolveUpdatedPath(idShortPath, submodelElement, true)
	} else {
		parentPath, targetIDShort, resolveErr := resolvePutCreateTargetPathParts(idShortPath)
		if resolveErr != nil {
			return false, resolveErr
		}

		if submodelElement == nil {
			return false, common.NewErrBadRequest("SMREPO-PUTSME-BADREQUEST Missing submodel element payload")
		}

		if submodelElement.IDShort() != nil {
			payloadIDShort := strings.TrimSpace(*submodelElement.IDShort())
			if payloadIDShort != "" && payloadIDShort != targetIDShort {
				return false, common.NewErrBadRequest("SMREPO-PUTSME-BADREQUEST Payload idShort must match path idShort when creating")
			}
		}
		submodelElement.SetIDShort(&targetIDShort)

		if parentPath == "" {
			if _, err = s.addTopLevelSubmodelElementInTransaction(tx, submodelID, submodelElement); err != nil {
				return false, err
			}
			historyMutation.currentPath = idShortPath
		} else {
			if err = s.addSubmodelElementWithPathInTransaction(ctx, tx, submodelID, submodelDatabaseID, parentPath, submodelElement); err != nil {
				return false, err
			}
			historyMutation.previousPath = parentPath
			historyMutation.currentPath = parentPath
		}
	}

	if shouldEnforce {
		exists, visible, visErr := s.checkSubmodelElementVisibilityInTx(ctx, tx, submodelID, idShortPath)
		if visErr != nil {
			return false, visErr
		}
		if !exists {
			return false, common.NewInternalServerError("SMREPO-PUTSME-ABACCHECKMISSING Written submodel element not found before commit")
		}
		if !visible {
			return false, common.NewErrDenied("SMREPO-PUTSME-ABACDENIED Written submodel element is not accessible under ABAC constraints")
		}
	}

	if err = s.appendChangedSubmodelElementHistoryTx(ctx, tx, submodelID, historyMutation); err != nil {
		return false, err
	}

	if err = tx.Commit(); err != nil {
		return false, err
	}

	return elementExists, nil
}

// DeleteSubmodelElementByPath deletes a submodel element and checks ABAC access on the current element when ABAC is enabled.
func (s *SubmodelDatabase) DeleteSubmodelElementByPath(ctx context.Context, submodelID string, idShortPath string) (err error) {
	tx, cleanup, err := common.StartTransaction(s.db)
	if err != nil {
		return err
	}
	defer cleanup(&err)

	shouldEnforce, enforceErr := shouldEnforceFormula(ctx, "SMREPO-DELSMEBPATH-SHOULDENFORCE")
	if enforceErr != nil {
		return enforceErr
	}
	if shouldEnforce {
		exists, visible, visErr := s.checkSubmodelElementVisibilityInTx(ctx, tx, submodelID, idShortPath)
		if visErr != nil {
			return visErr
		}
		if !exists {
			return common.NewErrNotFound("SMREPO-DELSMEBPATH-NOTFOUND Submodel-Element ID-Short: " + idShortPath)
		}
		if !visible {
			return common.NewErrDenied("SMREPO-DELSMEBPATH-ABACDENIED Deleting this submodel element is not allowed")
		}
	}

	deletedRootPath, err := submodelElementRootPath(idShortPath)
	if err != nil {
		return err
	}

	err = submodelelements.DeleteSubmodelElementByPath(tx, submodelID, idShortPath)
	if err != nil {
		return err
	}

	currentRootPath := deletedRootPath
	if deletedRootPath == idShortPath {
		currentRootPath = ""
	}
	if err = s.appendChangedSubmodelElementHistoryTx(ctx, tx, submodelID, submodelElementRootMutation{
		previousPath: deletedRootPath,
		currentPath:  currentRootPath,
	}); err != nil {
		return err
	}

	return tx.Commit()
}

// UpdateSubmodelElement updates a submodel element and checks ABAC access on old and new state when ABAC is enabled.
func (s *SubmodelDatabase) UpdateSubmodelElement(ctx context.Context, submodelID string, idShortOrPath string, submodelElement types.ISubmodelElement, isPut bool) (err error) {
	tx, cleanup, err := common.StartTransaction(s.db)
	if err != nil {
		return err
	}
	defer cleanup(&err)

	shouldEnforce, enforceErr := shouldEnforceFormula(ctx, "SMREPO-UPDSME-SHOULDENFORCE")
	if enforceErr != nil {
		return enforceErr
	}
	if shouldEnforce {
		exists, _, visErr := s.checkSubmodelElementVisibilityInTx(ctx, tx, submodelID, idShortOrPath)
		if visErr != nil {
			return visErr
		}
		if !exists {
			return common.NewErrNotFound("SMREPO-UPDSME-NOTFOUND Submodel-Element ID-Short: " + idShortOrPath)
		}
		ctx = auth.SelectPutFormulaByExistence(ctx, exists)
		_, visible, visErr := s.checkSubmodelElementVisibilityInTx(ctx, tx, submodelID, idShortOrPath)
		if visErr != nil {
			return visErr
		}
		if !visible {
			return common.NewErrDenied("SMREPO-UPDSME-ABACDENIED Existing submodel element is not accessible under ABAC constraints")
		}
	}

	err = s.updateSubmodelElementInTransaction(tx, submodelID, idShortOrPath, submodelElement, isPut)
	if err != nil {
		return err
	}

	if shouldEnforce {
		exists, visible, visErr := s.checkSubmodelElementVisibilityInTx(ctx, tx, submodelID, idShortOrPath)
		if visErr != nil {
			return visErr
		}
		if !exists || !visible {
			return common.NewErrDenied("SMREPO-UPDSME-ABACDENIED Updated submodel element is not accessible under ABAC constraints")
		}
	}

	if err = s.appendChangedSubmodelElementHistoryTx(ctx, tx, submodelID, submodelElementRootMutation{
		previousPath: idShortOrPath,
		currentPath:  submodelelements.ResolveUpdatedPath(idShortOrPath, submodelElement, isPut),
	}); err != nil {
		return err
	}

	return tx.Commit()
}

// UpdateSubmodelElementValueOnly updates a submodel element using value-only representation
// while preserving ABAC visibility checks from ctx.
func (s *SubmodelDatabase) UpdateSubmodelElementValueOnly(ctx context.Context, submodelID string, idShortOrPath string, valueOnly gen.SubmodelElementValue) (err error) {
	tx, cleanup, err := common.StartTransaction(s.db)
	if err != nil {
		return err
	}
	defer cleanup(&err)

	if err = s.updateSubmodelElementValueOnly(tx, submodelID, idShortOrPath, valueOnly); err != nil {
		return err
	}
	if err = s.appendChangedSubmodelElementHistoryTx(ctx, tx, submodelID, submodelElementRootMutation{
		previousPath: idShortOrPath,
		currentPath:  idShortOrPath,
	}); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *SubmodelDatabase) updateSubmodelElementValueOnly(tx *sql.Tx, submodelID string, idShortOrPath string, valueOnly gen.SubmodelElementValue) error {
	modelType, err := submodelelements.GetModelTypeByIdShortPathAndSubmodelIDTx(tx, submodelID, idShortOrPath)
	if err != nil {
		return err
	}

	if modelType == nil {
		return common.NewErrNotFound("SMREPO-UPDSMEVALONLY-NOTFOUND Submodel-Element ID-Short: " + idShortOrPath)
	}

	handler, err := submodelelements.GetSMEHandlerByModelType(*modelType, s.db)
	if err != nil {
		return err
	}

	return handler.UpdateValueOnly(submodelID, idShortOrPath, valueOnly, tx)
}

// UpdateSubmodelValueOnly updates all included top-level submodel elements using value-only representation
// while preserving ABAC visibility checks from ctx.
func (s *SubmodelDatabase) UpdateSubmodelValueOnly(ctx context.Context, submodelID string, valueOnly gen.SubmodelValue) (err error) {
	tx, cleanup, err := common.StartTransaction(s.db)
	if err != nil {
		return err
	}
	defer cleanup(&err)

	mutations := make([]submodelElementRootMutation, 0, len(valueOnly))
	for idShort, elementValue := range valueOnly {
		if err = s.updateSubmodelElementValueOnly(tx, submodelID, idShort, elementValue); err != nil {
			return err
		}
		mutations = append(mutations, submodelElementRootMutation{
			previousPath: idShort,
			currentPath:  idShort,
		})
	}

	if err = s.appendChangedSubmodelElementHistoryTx(ctx, tx, submodelID, mutations...); err != nil {
		return err
	}
	return tx.Commit()
}

// FileAttachmentExists reports whether a File submodel element currently has

func isSiblingIDShortCollision(tx *sql.Tx, submodelDatabaseID int, parentElementID *int, submodelElement types.ISubmodelElement) bool {
	idShortPtr := submodelElement.IDShort()
	if idShortPtr == nil || *idShortPtr == "" {
		return false
	}

	sqlQuery, args, err := submodelqueries.BuildSiblingIDShortCollisionSQL(submodelDatabaseID, parentElementID, *idShortPtr)
	if err != nil {
		return false
	}

	var count int
	if err = tx.QueryRow(sqlQuery, args...).Scan(&count); err != nil {
		return false
	}

	return count > 0
}

func getSMEModelTypeByPathInTx(tx *sql.Tx, submodelID string, idShortOrPath string) (*types.ModelType, error) {
	submodelDatabaseID, err := persistenceutils.GetSubmodelDatabaseID(tx, submodelID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, common.NewErrNotFound("SMREPO-GETMODELTYPE-SMNOTFOUND Submodel with ID '" + submodelID + "' not found")
		}
		return nil, err
	}

	query, args, err := submodelqueries.BuildSubmodelElementModelTypeByPathSQL(submodelDatabaseID, idShortOrPath)
	if err != nil {
		return nil, err
	}

	var modelType types.ModelType
	err = tx.QueryRow(query, args...).Scan(&modelType)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, common.NewErrNotFound("SMREPO-GETMODELTYPE-NOTFOUND Submodel-Element ID-Short: " + idShortOrPath)
		}
		return nil, err
	}

	return &modelType, nil
}

func submodelElementPathExistsInTx(tx *sql.Tx, submodelDatabaseID int, idShortPath string) (bool, error) {
	query, args, err := submodelqueries.BuildSubmodelElementPathExistsSQL(submodelDatabaseID, idShortPath)
	if err != nil {
		return false, err
	}

	var elementID int64
	err = tx.QueryRow(query, args...).Scan(&elementID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		return false, err
	}

	return true, nil
}

func parsePutIDShortPathSegments(idShortPath string) ([]submodelpath.Segment, error) {
	segments, err := submodelpath.ParseIDShortPathSegments(idShortPath)
	if err != nil {
		if errors.Is(err, submodelpath.ErrEmptyPath) {
			return nil, common.NewErrBadRequest("SMREPO-PUTSME-BADREQUEST Invalid idShortPath")
		}
		if errors.Is(err, submodelpath.ErrEmptyListIndex) {
			return nil, common.NewErrBadRequest("SMREPO-PUTSME-BADREQUEST Empty list index in idShortPath")
		}
		return nil, common.NewErrBadRequest("SMREPO-PUTSME-BADREQUEST Invalid idShortPath syntax")
	}
	return segments, nil
}

func buildPutIDShortPathFromSegments(segments []submodelpath.Segment) string {
	return submodelpath.BuildIDShortPathFromSegments(segments)
}

func resolvePutCreateTargetPathParts(idShortPath string) (string, string, error) {
	segments, parseErr := parsePutIDShortPathSegments(idShortPath)
	if parseErr != nil {
		return "", "", parseErr
	}

	lastSegment := segments[len(segments)-1]
	if lastSegment.IsIndex {
		return "", "", common.NewErrBadRequest("SMREPO-PUTSME-BADREQUEST Creating by list index path is not supported")
	}

	targetIDShort := strings.TrimSpace(lastSegment.Value)
	if targetIDShort == "" {
		return "", "", common.NewErrBadRequest("SMREPO-PUTSME-BADREQUEST Empty idShort segment in path")
	}

	if len(segments) == 1 {
		return "", targetIDShort, nil
	}

	parentPath := buildPutIDShortPathFromSegments(segments[:len(segments)-1])
	if parentPath == "" {
		return "", "", common.NewErrBadRequest("SMREPO-PUTSME-BADREQUEST Invalid parent path")
	}

	return parentPath, targetIDShort, nil
}
