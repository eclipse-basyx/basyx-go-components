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
	"github.com/eclipse-basyx/basyx-go-components/internal/common/createprecheck"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/history"
	gen "github.com/eclipse-basyx/basyx-go-components/internal/common/model"
	auth "github.com/eclipse-basyx/basyx-go-components/internal/common/security"
	submodelpath "github.com/eclipse-basyx/basyx-go-components/internal/submodelrepository/path"
	submodelqueries "github.com/eclipse-basyx/basyx-go-components/internal/submodelrepository/persistence/queries"
	submodelelements "github.com/eclipse-basyx/basyx-go-components/internal/submodelrepository/persistence/submodelElements"
	persistenceutils "github.com/eclipse-basyx/basyx-go-components/internal/submodelrepository/persistence/utils"
)

func (s *SubmodelDatabase) addTopLevelSubmodelElementInTransaction(ctx context.Context, tx *sql.Tx, submodelID string, submodelElement types.ISubmodelElement) (string, error) {
	submodelDatabaseID, err := persistenceutils.GetSubmodelDatabaseIDForNoKeyUpdateContext(ctx, tx, submodelID)
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
	err = tx.QueryRowContext(ctx, selectQuery, selectArgs...).Scan(&maxPosition)
	if err != nil {
		return "", err
	}

	startPosition := 0
	if maxPosition.Valid {
		startPosition = int(maxPosition.Int64) + 1
	}

	if err = s.ensureVisibleSubmodelElementCreateDoesNotExist(
		ctx,
		tx,
		submodelID,
		submodelDatabaseID,
		nil,
		submodelElement,
		"SMREPO-ADDSME-COLLISION Duplicate submodel element idShort",
		"SMREPO-ADDSME-CHKDUP-ABACDENIED existing submodel element is not accessible under ABAC constraints",
	); err != nil {
		return "", err
	}

	_, err = submodelelements.InsertSubmodelElementsForSubmodelDatabaseIDContext(
		ctx,
		s.db,
		submodelDatabaseID,
		[]types.ISubmodelElement{submodelElement},
		tx,
		&submodelelements.BatchInsertContext{
			StartPosition: startPosition,
		},
		nil,
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
func (s *SubmodelDatabase) GetSubmodelElement(ctx context.Context, submodelID string, idShortOrPath string, includeBlobValue bool, level string) (types.ISubmodelElement, error) {
	if submodelID == "" || idShortOrPath == "" || (level != "" && level != "core" && level != "deep") {
		return submodelelements.GetSubmodelElementByIDShortOrPath(ctx, nil, submodelID, idShortOrPath, includeBlobValue, level)
	}
	var result types.ISubmodelElement
	err := common.ExecuteInReadTransaction(ctx, s.readDB(ctx), "SMREPO-GETSME-STARTTX", "SMREPO-GETSME-COMMIT", func(tx *sql.Tx) error {
		var txErr error
		result, txErr = submodelelements.GetSubmodelElementByIDShortOrPathTx(ctx, tx, submodelID, idShortOrPath, includeBlobValue, level)
		return txErr
	})
	return result, err
}

// GetSubmodelElements retrieves submodel elements and applies optional ABAC formula filters from ctx.
func (s *SubmodelDatabase) GetSubmodelElements(ctx context.Context, submodelID string, limit *int, cursor string, includeBlobValue bool, level string) ([]types.ISubmodelElement, string, error) {
	if submodelID == "" || (limit != nil && *limit < -1) {
		return submodelelements.GetSubmodelElementsBySubmodelID(ctx, nil, submodelID, limit, cursor, includeBlobValue, level)
	}
	var result []types.ISubmodelElement
	var nextCursor string
	err := common.ExecuteInReadTransaction(ctx, s.readDB(ctx), "SMREPO-GETSMES-STARTTX", "SMREPO-GETSMES-COMMIT", func(tx *sql.Tx) error {
		var txErr error
		result, nextCursor, txErr = submodelelements.GetSubmodelElementsBySubmodelIDTx(ctx, tx, submodelID, limit, cursor, includeBlobValue, level)
		return txErr
	})
	return result, nextCursor, err
}

// GetSubmodelElementPaths retrieves submodel element paths directly from persisted idshort_path values.
func (s *SubmodelDatabase) GetSubmodelElementPaths(ctx context.Context, submodelID string, level string) ([]string, error) {
	if submodelID == "" || (level != "" && level != "core" && level != "deep") {
		return submodelelements.GetSubmodelElementPathsBySubmodelID(ctx, nil, submodelID, level)
	}
	var result []string
	err := common.ExecuteInReadTransaction(ctx, s.readDB(ctx), "SMREPO-GETSMEPATHS-STARTTX", "SMREPO-GETSMEPATHS-COMMIT", func(tx *sql.Tx) error {
		var txErr error
		result, txErr = submodelelements.GetSubmodelElementPathsBySubmodelID(ctx, tx, submodelID, level)
		return txErr
	})
	return result, err
}

// GetSubmodelElementPathPage retrieves paged submodel element paths directly from persisted idshort_path values.
func (s *SubmodelDatabase) GetSubmodelElementPathPage(ctx context.Context, submodelID string, limit *int, cursor string, level string) ([]string, string, error) {
	if submodelID == "" || (level != "" && level != "core" && level != "deep") || (limit != nil && *limit < 0) {
		return submodelelements.GetSubmodelElementPathsPageBySubmodelID(ctx, nil, submodelID, limit, cursor, level)
	}
	var result []string
	var nextCursor string
	err := common.ExecuteInReadTransaction(ctx, s.readDB(ctx), "SMREPO-GETSMEPATHSPAGE-STARTTX", "SMREPO-GETSMEPATHSPAGE-COMMIT", func(tx *sql.Tx) error {
		var txErr error
		result, nextCursor, txErr = submodelelements.GetSubmodelElementPathsPageBySubmodelID(ctx, tx, submodelID, limit, cursor, level)
		return txErr
	})
	return result, nextCursor, err
}

// GetSubmodelElementPathsByPath retrieves path notation for a specific submodel element path.
func (s *SubmodelDatabase) GetSubmodelElementPathsByPath(ctx context.Context, submodelID string, idShortPath string, level string) ([]string, error) {
	if submodelID == "" || idShortPath == "" || (level != "" && level != "core" && level != "deep") {
		return submodelelements.GetSubmodelElementPathsByPath(ctx, nil, submodelID, idShortPath, level)
	}
	var result []string
	err := common.ExecuteInReadTransaction(ctx, s.readDB(ctx), "SMREPO-GETSMEPATHSBYPATH-STARTTX", "SMREPO-GETSMEPATHSBYPATH-COMMIT", func(tx *sql.Tx) error {
		var txErr error
		result, txErr = submodelelements.GetSubmodelElementPathsByPath(ctx, tx, submodelID, idShortPath, level)
		return txErr
	})
	return result, err
}

// GetSubmodelElementReferences retrieves SME references and applies optional ABAC formula filters from ctx.
func (s *SubmodelDatabase) GetSubmodelElementReferences(ctx context.Context, submodelID string, limit *int, cursor string) ([]types.IReference, string, error) {
	if submodelID == "" || (limit != nil && *limit < -1) {
		return submodelelements.GetSubmodelElementReferencesBySubmodelID(ctx, nil, submodelID, limit, cursor)
	}
	var result []types.IReference
	var nextCursor string
	err := common.ExecuteInReadTransaction(ctx, s.readDB(ctx), "SMREPO-GETSMEREFS-STARTTX", "SMREPO-GETSMEREFS-COMMIT", func(tx *sql.Tx) error {
		var txErr error
		result, nextCursor, txErr = submodelelements.GetSubmodelElementReferencesBySubmodelID(ctx, tx, submodelID, limit, cursor)
		return txErr
	})
	return result, nextCursor, err
}

// AddSubmodelElement adds a top-level submodel element and performs an ABAC re-check before commit when ABAC is enabled.
func (s *SubmodelDatabase) AddSubmodelElement(ctx context.Context, submodelID string, submodelElement types.ISubmodelElement) (err error) {
	tx, cleanup, err := common.StartTransactionContext(ctx, s.db)
	if err != nil {
		return err
	}
	defer cleanup(&err)
	previousSnapshot, err := s.loadSubmodelHistorySnapshotBeforeMutationTx(ctx, tx, submodelID)
	if err != nil {
		return err
	}

	insertedPath, err := s.addTopLevelSubmodelElementInTransaction(ctx, tx, submodelID, submodelElement)
	if err != nil {
		return err
	}

	shouldEnforce, enforceErr := shouldEnforceFormula(ctx, "SMREPO-ADDSME-SHOULDENFORCE")
	if enforceErr != nil {
		return enforceErr
	}
	if shouldCheckInsertedElementVisibility(shouldEnforce, insertedPath) {
		if err = s.ensureCreatedSubmodelElementIsVisible(ctx, tx, submodelID, insertedPath); err != nil {
			return err
		}
	}

	if insertedPath == "" {
		err = s.appendCurrentSubmodelHistoryTx(ctx, tx, submodelID, previousSnapshot, history.ChangeUpdated)
	} else {
		err = s.appendChangedSubmodelElementHistoryTx(ctx, tx, submodelID, previousSnapshot, submodelElementRootMutation{
			currentPath: insertedPath,
		})
	}
	if err != nil {
		return err
	}

	return tx.Commit()
}

func (s *SubmodelDatabase) addSubmodelElementWithPathInTransaction(ctx context.Context, tx *sql.Tx, submodelID string, parentPath string, submodelElement types.ISubmodelElement) error {
	parent, err := s.lockSubmodelElementParentForInsert(ctx, tx, submodelID, parentPath)
	if err != nil {
		return err
	}
	isFromList := false
	switch parent.modelType {
	case types.ModelTypeSubmodelElementCollection, types.ModelTypeEntity, types.ModelTypeAnnotatedRelationshipElement:
		isFromList = false
	case types.ModelTypeSubmodelElementList:
		isFromList = true
	default:
		return common.NewErrBadRequest("SMREPO-ADDSMEBYPATH-BADPARENT Parent element does not support child elements")
	}

	if err = s.ensureVisibleSubmodelElementCreateDoesNotExist(
		ctx,
		tx,
		submodelID,
		parent.submodelDatabaseID,
		&parent.elementID,
		submodelElement,
		"SMREPO-ADDSMEBYPATH-COLLISION Duplicate submodel element idShort",
		"SMREPO-ADDSMEBYPATH-CHKDUP-ABACDENIED existing submodel element is not accessible under ABAC constraints",
	); err != nil {
		return err
	}

	_, err = submodelelements.InsertSubmodelElementsForSubmodelDatabaseIDContext(
		ctx,
		s.db,
		parent.submodelDatabaseID,
		[]types.ISubmodelElement{submodelElement},
		tx,
		&submodelelements.BatchInsertContext{
			ParentID:      parent.elementID,
			ParentPath:    parentPath,
			RootSmeID:     parent.rootSmeID,
			IsFromList:    isFromList,
			StartPosition: parent.nextPosition,
			StartDepth:    parent.childDepth,
		},
		nil,
	)
	if err != nil {
		return err
	}

	return nil
}

type submodelElementInsertParent struct {
	submodelDatabaseID int
	elementID          int
	rootSmeID          int
	modelType          types.ModelType
	nextPosition       int
	childDepth         int
}

func (s *SubmodelDatabase) lockSubmodelElementParentForInsert(ctx context.Context, tx *sql.Tx, submodelID string, parentPath string) (submodelElementInsertParent, error) {
	query, args, err := submodelqueries.BuildSubmodelElementParentForInsertSQL(submodelID, parentPath)
	if err != nil {
		return submodelElementInsertParent{}, common.NewInternalServerError("SMREPO-ADDSMEBYPATH-BUILDPARENTQ " + err.Error())
	}

	var parent submodelElementInsertParent
	err = tx.QueryRowContext(ctx, query, args...).Scan(
		&parent.submodelDatabaseID,
		&parent.elementID,
		&parent.rootSmeID,
		&parent.modelType,
		&parent.childDepth,
	)
	if err == nil {
		parent.nextPosition, err = nextSubmodelElementPosition(ctx, tx, parent.elementID)
		return parent, err
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return submodelElementInsertParent{}, common.NewInternalServerError("SMREPO-ADDSMEBYPATH-EXECPARENTQ " + err.Error())
	}
	return submodelElementInsertParent{}, submodelElementParentNotFoundError(ctx, tx, submodelID, parentPath)
}

func nextSubmodelElementPosition(ctx context.Context, tx *sql.Tx, parentElementID int) (int, error) {
	query, args, err := submodelqueries.BuildSubmodelElementNextPositionSQL(parentElementID)
	if err != nil {
		return 0, common.NewInternalServerError("SMREPO-ADDSMEBYPATH-BUILDNEXTPOSQ " + err.Error())
	}

	var nextPosition int
	if err = tx.QueryRowContext(ctx, query, args...).Scan(&nextPosition); err != nil {
		return 0, common.NewInternalServerError("SMREPO-ADDSMEBYPATH-EXECNEXTPOSQ " + err.Error())
	}
	return nextPosition, nil
}

func submodelElementParentNotFoundError(ctx context.Context, tx *sql.Tx, submodelID string, parentPath string) error {
	query, args, err := submodelqueries.SelectVisibleSubmodelDataset(submodelID).Prepared(true).ToSQL()
	if err != nil {
		return common.NewInternalServerError("SMREPO-ADDSMEBYPATH-BUILDSMEXISTSQ " + err.Error())
	}

	var submodelDatabaseID int
	err = tx.QueryRowContext(ctx, query, args...).Scan(&submodelDatabaseID)
	if errors.Is(err, sql.ErrNoRows) {
		return common.NewErrNotFound("SMREPO-ADDSMEBYPATH-SMNOTFOUND Submodel with ID '" + submodelID + "' not found")
	}
	if err != nil {
		return common.NewInternalServerError("SMREPO-ADDSMEBYPATH-EXECSMEXISTSQ " + err.Error())
	}
	return common.NewErrNotFound("SMREPO-ADDSMEBYPATH-PARENTNOTFOUND Submodel element with path '" + parentPath + "' not found")
}

// AddSubmodelElementWithPath adds a submodel element under an existing container path
// while preserving ABAC visibility checks from ctx.
func (s *SubmodelDatabase) AddSubmodelElementWithPath(ctx context.Context, submodelID string, parentPath string, submodelElement types.ISubmodelElement) error {
	tx, cleanup, err := common.StartTransactionContext(ctx, s.db)
	if err != nil {
		return common.NewInternalServerError("SMREPO-ADDSMEBYPATH-STARTTX " + err.Error())
	}
	defer cleanup(&err)

	previousSnapshot, err := s.loadSubmodelHistorySnapshotBeforeMutationTx(ctx, tx, submodelID)
	if err != nil {
		return err
	}

	err = s.addSubmodelElementWithPathInTransaction(ctx, tx, submodelID, parentPath, submodelElement)
	if err != nil {
		return err
	}

	if err = s.appendChangedSubmodelElementHistoryTx(ctx, tx, submodelID, previousSnapshot, submodelElementRootMutation{
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

	elementExists, err := s.PutSubmodelElementInTransaction(ctx, tx, submodelID, idShortPath, submodelElement)
	if err != nil {
		return false, err
	}

	if err = tx.Commit(); err != nil {
		return false, err
	}

	return elementExists, nil
}

// PutSubmodelElementInTransaction creates or replaces a submodel element within an existing transaction.
func (s *SubmodelDatabase) PutSubmodelElementInTransaction(
	ctx context.Context,
	tx *sql.Tx,
	submodelID string,
	idShortPath string,
	submodelElement types.ISubmodelElement,
) (bool, error) {
	submodelDatabaseID, err := persistenceutils.GetSubmodelDatabaseID(tx, submodelID)
	if err != nil {
		if err == sql.ErrNoRows {
			return false, common.NewErrNotFound("SMREPO-PUTSME-SMNOTFOUND Submodel with ID '" + submodelID + "' not found")
		}
		return false, err
	}

	var elementExists bool
	var historyMutation submodelElementRootMutation
	shouldEnforce, enforceErr := shouldEnforceFormula(ctx, "SMREPO-PUTSME-SHOULDENFORCE")
	if enforceErr != nil {
		return false, enforceErr
	}

	ctx, elementExists, err = s.determinePutSubmodelElementExistence(ctx, tx, submodelID, submodelDatabaseID, idShortPath, shouldEnforce)
	if err != nil {
		return false, err
	}
	previousSnapshot, err := s.loadSubmodelHistorySnapshotBeforeMutationTx(ctx, tx, submodelID)
	if err != nil {
		return false, err
	}

	if elementExists {
		historyMutation, err = s.replaceSubmodelElementForPut(tx, submodelID, idShortPath, submodelElement)
		if err != nil {
			return false, err
		}
	} else {
		historyMutation, err = s.createSubmodelElementForPut(ctx, tx, submodelID, idShortPath, submodelElement)
		if err != nil {
			return false, err
		}
	}

	if shouldEnforce {
		if err = s.ensurePutSubmodelElementIsVisible(ctx, tx, submodelID, idShortPath); err != nil {
			return false, err
		}
	}

	if err = s.appendChangedSubmodelElementHistoryTx(ctx, tx, submodelID, previousSnapshot, historyMutation); err != nil {
		return false, err
	}

	return elementExists, nil
}

func shouldCheckInsertedElementVisibility(shouldEnforce bool, insertedPath string) bool {
	return shouldEnforce && insertedPath != ""
}

func (s *SubmodelDatabase) ensureCreatedSubmodelElementIsVisible(ctx context.Context, tx *sql.Tx, submodelID string, idShortPath string) error {
	exists, visible, err := s.checkSubmodelElementVisibilityInTx(ctx, tx, submodelID, idShortPath)
	if err != nil {
		return err
	}
	if !exists {
		return common.NewInternalServerError("SMREPO-ADDSME-ABACCHECKMISSING created submodel element not found before commit")
	}
	if !visible {
		return common.NewErrDenied("SMREPO-ADDSME-ABACDENIED Created submodel element is not accessible under ABAC constraints")
	}
	return nil
}

func (s *SubmodelDatabase) determinePutSubmodelElementExistence(
	ctx context.Context,
	tx *sql.Tx,
	submodelID string,
	submodelDatabaseID int,
	idShortPath string,
	shouldEnforce bool,
) (context.Context, bool, error) {
	if !shouldEnforce {
		elementExists, err := submodelElementPathExistsInTx(tx, submodelDatabaseID, idShortPath)
		return ctx, elementExists, err
	}

	elementExists, err := s.putSubmodelElementExistsForCurrentAccess(ctx, tx, submodelID, idShortPath)
	if err != nil {
		return ctx, false, err
	}
	ctx = auth.SelectPutFormulaByExistence(ctx, elementExists)
	if !elementExists {
		return ctx, false, nil
	}
	return ctx, true, s.ensureExistingSubmodelElementCanBeReplaced(ctx, tx, submodelID, idShortPath)
}

func (s *SubmodelDatabase) putSubmodelElementExistsForCurrentAccess(ctx context.Context, tx *sql.Tx, submodelID string, idShortPath string) (bool, error) {
	exists, _, err := s.checkSubmodelElementVisibilityInTx(ctx, tx, submodelID, idShortPath)
	return exists, err
}

func (s *SubmodelDatabase) ensureExistingSubmodelElementCanBeReplaced(ctx context.Context, tx *sql.Tx, submodelID string, idShortPath string) error {
	_, visible, err := s.checkSubmodelElementVisibilityInTx(ctx, tx, submodelID, idShortPath)
	if err != nil {
		return err
	}
	if !visible {
		return common.NewErrDenied("SMREPO-PUTSME-ABACDENIED Existing submodel element is not accessible under ABAC constraints")
	}
	return nil
}

func (s *SubmodelDatabase) replaceSubmodelElementForPut(
	tx *sql.Tx,
	submodelID string,
	idShortPath string,
	submodelElement types.ISubmodelElement,
) (submodelElementRootMutation, error) {
	if err := s.updateSubmodelElementInTransaction(tx, submodelID, idShortPath, submodelElement, true); err != nil {
		return submodelElementRootMutation{}, err
	}

	return submodelElementRootMutation{
		previousPath: idShortPath,
		currentPath:  submodelelements.ResolveUpdatedPath(idShortPath, submodelElement, true),
	}, nil
}

func (s *SubmodelDatabase) createSubmodelElementForPut(
	ctx context.Context,
	tx *sql.Tx,
	submodelID string,
	idShortPath string,
	submodelElement types.ISubmodelElement,
) (submodelElementRootMutation, error) {
	parentPath, targetIDShort, err := resolvePutCreateTargetPathParts(idShortPath)
	if err != nil {
		return submodelElementRootMutation{}, err
	}
	if err = ensurePayloadIDShortMatchesTargetPath(submodelElement, targetIDShort); err != nil {
		return submodelElementRootMutation{}, err
	}
	submodelElement.SetIDShort(&targetIDShort)

	if isTopLevelPutCreate(parentPath) {
		if _, err = s.addTopLevelSubmodelElementInTransaction(ctx, tx, submodelID, submodelElement); err != nil {
			return submodelElementRootMutation{}, err
		}
		return submodelElementRootMutation{currentPath: idShortPath}, nil
	}

	if err = s.addSubmodelElementWithPathInTransaction(ctx, tx, submodelID, parentPath, submodelElement); err != nil {
		return submodelElementRootMutation{}, err
	}
	return submodelElementRootMutation{
		previousPath: parentPath,
		currentPath:  parentPath,
	}, nil
}

func ensurePayloadIDShortMatchesTargetPath(submodelElement types.ISubmodelElement, targetIDShort string) error {
	if submodelElement == nil {
		return common.NewErrBadRequest("SMREPO-PUTSME-BADREQUEST Missing submodel element payload")
	}

	if submodelElement.IDShort() == nil {
		return nil
	}

	payloadIDShort := strings.TrimSpace(*submodelElement.IDShort())
	if payloadIDShort != "" && payloadIDShort != targetIDShort {
		return common.NewErrBadRequest("SMREPO-PUTSME-BADREQUEST Payload idShort must match path idShort when creating")
	}
	return nil
}

func isTopLevelPutCreate(parentPath string) bool {
	return parentPath == ""
}

func (s *SubmodelDatabase) ensurePutSubmodelElementIsVisible(ctx context.Context, tx *sql.Tx, submodelID string, idShortPath string) error {
	exists, visible, err := s.checkSubmodelElementVisibilityInTx(ctx, tx, submodelID, idShortPath)
	if err != nil {
		return err
	}
	if !exists {
		return common.NewInternalServerError("SMREPO-PUTSME-ABACCHECKMISSING Written submodel element not found before commit")
	}
	if !visible {
		return common.NewErrDenied("SMREPO-PUTSME-ABACDENIED Written submodel element is not accessible under ABAC constraints")
	}
	return nil
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
		if err = s.ensureSubmodelElementCanBeDeleted(ctx, tx, submodelID, idShortPath); err != nil {
			return err
		}
	}

	deletedRootPath, err := submodelElementRootPath(idShortPath)
	if err != nil {
		return err
	}
	previousSnapshot, err := s.loadSubmodelHistorySnapshotBeforeMutationTx(ctx, tx, submodelID)
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
	if err = s.appendChangedSubmodelElementHistoryTx(ctx, tx, submodelID, previousSnapshot, submodelElementRootMutation{
		previousPath: deletedRootPath,
		currentPath:  currentRootPath,
	}); err != nil {
		return err
	}

	return tx.Commit()
}

func (s *SubmodelDatabase) ensureSubmodelElementCanBeDeleted(ctx context.Context, tx *sql.Tx, submodelID string, idShortPath string) error {
	exists, visible, err := s.checkSubmodelElementVisibilityInTx(ctx, tx, submodelID, idShortPath)
	if err != nil {
		return err
	}
	if !exists {
		return common.NewErrNotFound("SMREPO-DELSMEBPATH-NOTFOUND Submodel-Element ID-Short: " + idShortPath)
	}
	if !visible {
		return common.NewErrDenied("SMREPO-DELSMEBPATH-ABACDENIED Deleting this submodel element is not allowed")
	}
	return nil
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
		ctx, err = s.ensureSubmodelElementCanBeUpdated(ctx, tx, submodelID, idShortOrPath)
		if err != nil {
			return err
		}
	}
	previousSnapshot, err := s.loadSubmodelHistorySnapshotBeforeMutationTx(ctx, tx, submodelID)
	if err != nil {
		return err
	}

	err = s.updateSubmodelElementInTransaction(tx, submodelID, idShortOrPath, submodelElement, isPut)
	if err != nil {
		return err
	}

	if shouldEnforce {
		if err = s.ensureUpdatedSubmodelElementIsVisible(ctx, tx, submodelID, idShortOrPath); err != nil {
			return err
		}
	}

	if err = s.appendChangedSubmodelElementHistoryTx(ctx, tx, submodelID, previousSnapshot, submodelElementRootMutation{
		previousPath: idShortOrPath,
		currentPath:  submodelelements.ResolveUpdatedPath(idShortOrPath, submodelElement, isPut),
	}); err != nil {
		return err
	}

	return tx.Commit()
}

func (s *SubmodelDatabase) ensureSubmodelElementCanBeUpdated(ctx context.Context, tx *sql.Tx, submodelID string, idShortOrPath string) (context.Context, error) {
	exists, _, err := s.checkSubmodelElementVisibilityInTx(ctx, tx, submodelID, idShortOrPath)
	if err != nil {
		return ctx, err
	}
	if !exists {
		return ctx, common.NewErrNotFound("SMREPO-UPDSME-NOTFOUND Submodel-Element ID-Short: " + idShortOrPath)
	}

	ctx = auth.SelectPutFormulaByExistence(ctx, exists)
	_, visible, err := s.checkSubmodelElementVisibilityInTx(ctx, tx, submodelID, idShortOrPath)
	if err != nil {
		return ctx, err
	}
	if !visible {
		return ctx, common.NewErrDenied("SMREPO-UPDSME-ABACDENIED Existing submodel element is not accessible under ABAC constraints")
	}
	return ctx, nil
}

func (s *SubmodelDatabase) ensureUpdatedSubmodelElementIsVisible(ctx context.Context, tx *sql.Tx, submodelID string, idShortOrPath string) error {
	exists, visible, err := s.checkSubmodelElementVisibilityInTx(ctx, tx, submodelID, idShortOrPath)
	if err != nil {
		return err
	}
	if !exists || !visible {
		return common.NewErrDenied("SMREPO-UPDSME-ABACDENIED Updated submodel element is not accessible under ABAC constraints")
	}
	return nil
}

// UpdateSubmodelElementValueOnly updates a submodel element using value-only representation
// while preserving ABAC visibility checks from ctx.
func (s *SubmodelDatabase) UpdateSubmodelElementValueOnly(ctx context.Context, submodelID string, idShortOrPath string, valueOnly gen.SubmodelElementValue) (err error) {
	tx, cleanup, err := common.StartTransaction(s.db)
	if err != nil {
		return err
	}
	defer cleanup(&err)
	previousSnapshot, err := s.loadSubmodelHistorySnapshotBeforeMutationTx(ctx, tx, submodelID)
	if err != nil {
		return err
	}

	if err = s.updateSubmodelElementValueOnly(tx, submodelID, idShortOrPath, valueOnly); err != nil {
		return err
	}
	if err = s.appendChangedSubmodelElementHistoryTx(ctx, tx, submodelID, previousSnapshot, submodelElementRootMutation{
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
	previousSnapshot, err := s.loadSubmodelHistorySnapshotBeforeMutationTx(ctx, tx, submodelID)
	if err != nil {
		return err
	}

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

	if err = s.appendChangedSubmodelElementHistoryTx(ctx, tx, submodelID, previousSnapshot, mutations...); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *SubmodelDatabase) ensureVisibleSubmodelElementCreateDoesNotExist(
	ctx context.Context,
	tx *sql.Tx,
	submodelID string,
	submodelDatabaseID int,
	parentElementID *int,
	submodelElement types.ISubmodelElement,
	conflictMessage string,
	deniedMessage string,
) error {
	if createprecheck.CanSkipExistenceCheck(ctx) {
		return nil
	}
	idShortPtr := submodelElement.IDShort()
	if idShortPtr == nil || *idShortPtr == "" {
		return nil
	}

	duplicatePath := ""
	return createprecheck.EnsureVisibleCreate(
		ctx,
		func(readCtx context.Context) (bool, error) {
			path, exists, err := siblingIDShortCollisionPathInTx(readCtx, tx, submodelDatabaseID, parentElementID, *idShortPtr)
			if err != nil {
				return false, err
			}
			duplicatePath = path
			return exists, nil
		},
		func(readCtx context.Context) error {
			if duplicatePath == "" {
				return common.NewInternalServerError("SMREPO-CHKSMEDUP-EMPTYPATH existing submodel element duplicate path is empty")
			}
			exists, visible, err := s.checkSubmodelElementVisibilityInTx(readCtx, tx, submodelID, duplicatePath)
			if err != nil {
				return err
			}
			if !exists {
				return common.NewErrNotFound("SMREPO-CHKSMEDUP-NOTFOUND existing submodel element not found")
			}
			if !visible {
				return common.NewErrDenied(deniedMessage)
			}
			return nil
		},
		conflictMessage,
		deniedMessage,
	)
}

func siblingIDShortCollisionPathInTx(ctx context.Context, tx *sql.Tx, submodelDatabaseID int, parentElementID *int, idShort string) (string, bool, error) {
	query, args, err := submodelqueries.BuildSiblingIDShortCollisionPathSQL(submodelDatabaseID, parentElementID, idShort)
	if err != nil {
		return "", false, common.NewInternalServerError("SMREPO-CHKSMEDUP-BUILDPATHQ " + err.Error())
	}

	var idShortPath string
	err = tx.QueryRowContext(ctx, query, args...).Scan(&idShortPath)
	if err == nil {
		return idShortPath, true, nil
	}
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}

	return "", false, common.NewInternalServerError("SMREPO-CHKSMEDUP-EXECPATHQ " + err.Error())
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
