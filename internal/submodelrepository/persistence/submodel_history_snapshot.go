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

package persistence

import (
	"context"
	"database/sql"
	"strings"

	"github.com/FriedJannik/aas-go-sdk/jsonization"
	"github.com/FriedJannik/aas-go-sdk/types"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/history"
	auth "github.com/eclipse-basyx/basyx-go-components/internal/common/security"
	submodelpath "github.com/eclipse-basyx/basyx-go-components/internal/submodelrepository/path"
	submodelelements "github.com/eclipse-basyx/basyx-go-components/internal/submodelrepository/persistence/submodelElements"
)

const submodelElementsSnapshotField = "submodelElements"

type submodelElementRootMutation struct {
	previousPath string
	currentPath  string
}

func (s *SubmodelDatabase) appendChangedSubmodelElementHistoryTx(
	ctx context.Context,
	tx *sql.Tx,
	submodelID string,
	previousSnapshot map[string]any,
	mutations ...submodelElementRootMutation,
) error {
	return s.appendMutatedSubmodelHistoryTx(ctx, tx, submodelID, previousSnapshot, func(snapshot map[string]any) error {
		for _, mutation := range mutations {
			previousRoot, err := submodelElementRootPath(mutation.previousPath)
			if err != nil {
				return err
			}

			var currentRootSnapshot map[string]any
			if mutation.currentPath != "" {
				currentRoot, rootErr := submodelElementRootPath(mutation.currentPath)
				if rootErr != nil {
					return rootErr
				}
				currentRootSnapshot, err = loadSubmodelElementRootSnapshotTx(ctx, tx, submodelID, currentRoot)
				if err != nil {
					return err
				}
			}

			if err = replaceSubmodelElementRootSnapshot(snapshot, previousRoot, currentRootSnapshot); err != nil {
				return err
			}
		}
		return nil
	})
}

func (s *SubmodelDatabase) appendSubmodelMetadataHistoryTx(ctx context.Context, tx *sql.Tx, submodelID string, previousSnapshot map[string]any, submodel types.ISubmodel) error {
	metadata, err := submodelToHistorySnapshot(submodel)
	if err != nil {
		return err
	}
	delete(metadata, submodelElementsSnapshotField)

	return s.appendMutatedSubmodelHistoryTx(ctx, tx, submodelID, previousSnapshot, func(snapshot map[string]any) error {
		elements, hasElements := snapshot[submodelElementsSnapshotField]
		clear(snapshot)
		for key, value := range metadata {
			snapshot[key] = value
		}
		if hasElements {
			snapshot[submodelElementsSnapshotField] = elements
		}
		return nil
	})
}

func (s *SubmodelDatabase) appendMutatedSubmodelHistoryTx(ctx context.Context, tx *sql.Tx, submodelID string, previousSnapshot map[string]any, mutate history.SnapshotMutator) error {
	err := history.AppendMutatedVersionTx(ctx, tx, history.TableSubmodel, submodelID, history.ChangeUpdated, previousSnapshot, func(snapshot map[string]any) error {
		return mutate(snapshot)
	})
	if err == nil || !common.IsErrNotFound(err) {
		return err
	}
	return s.appendCurrentSubmodelHistoryTx(ctx, tx, submodelID, previousSnapshot, history.ChangeUpdated)
}

func loadSubmodelElementRootSnapshotTx(ctx context.Context, tx *sql.Tx, submodelID string, rootPath string) (map[string]any, error) {
	stateReadCtx := ctx
	if history.ActiveConfig().EvidenceEnabled {
		stateReadCtx = auth.ContextWithoutQueryFilter(ctx)
	}
	rootElement, err := submodelelements.GetSubmodelElementByIDShortOrPathTx(stateReadCtx, tx, submodelID, rootPath, "deep")
	if err != nil {
		return nil, err
	}
	jsonable, err := jsonization.ToJsonable(rootElement)
	if err != nil {
		return nil, common.NewInternalServerError("SMREPO-HISTORY-SME-TOJSONABLE " + err.Error())
	}
	return jsonable, nil
}

func submodelElementRootPath(idShortPath string) (string, error) {
	if idShortPath == "" {
		return "", nil
	}
	segments, err := submodelpath.ParseIDShortPathSegments(idShortPath)
	if err != nil {
		return "", common.NewInternalServerError("SMREPO-HISTORY-SME-BADPATH " + err.Error())
	}
	if len(segments) == 0 || segments[0].IsIndex || strings.TrimSpace(segments[0].Value) == "" {
		return "", common.NewInternalServerError("SMREPO-HISTORY-SME-BADROOT invalid top-level submodel element path")
	}
	return segments[0].Value, nil
}

func replaceSubmodelElementRootSnapshot(snapshot map[string]any, previousRoot string, currentRoot map[string]any) error {
	if previousRoot == "" {
		if currentRoot == nil {
			return common.NewInternalServerError("SMREPO-HISTORY-SME-EMPTYMUTATION missing current submodel element snapshot")
		}
		return history.AppendSnapshotArrayItem(snapshot, submodelElementsSnapshotField, currentRoot)
	}

	matchesPreviousRoot := func(element map[string]any) bool {
		return element["idShort"] == previousRoot
	}
	if currentRoot != nil {
		return history.ReplaceSnapshotArrayItem(snapshot, submodelElementsSnapshotField, matchesPreviousRoot, currentRoot)
	}
	return history.RemoveSnapshotArrayItem(snapshot, submodelElementsSnapshotField, matchesPreviousRoot)
}
