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
	"crypto/sha256"
	"encoding/json"
	"fmt"
	submodelelements "github.com/eclipse-basyx/basyx-go-components/internal/submodelrepository/persistence/submodelElements"
	"hash"
	"strings"
)

func protectReconciliationListIdentities(previous, submitted []submodelelements.ReconciliationElementRow) error {
	before, err := reconciliationListSignatures(previous)
	if err != nil {
		return err
	}
	after, err := reconciliationListSignatures(submitted)
	if err != nil {
		return err
	}
	targets := indexReconciliationRows(submitted)
	oldCounts := reconciliationListCounts(previous)
	newCounts := reconciliationListCounts(submitted)
	for index, row := range previous {
		if !isReconciliationListEntry(row) {
			continue
		}
		_, exists := targets[row.Path]
		if !exists {
			continue
		}
		if oldCounts[row.ParentPath] != newCounts[row.ParentPath] || before[row.Path] != after[row.Path] {
			previous[index].ModelType = -1
		}
	}
	return nil
}

func reconciliationListCounts(rows []submodelelements.ReconciliationElementRow) map[string]int {
	counts := map[string]int{}
	for _, row := range rows {
		if isReconciliationListEntry(row) {
			counts[row.ParentPath]++
		}
	}
	return counts
}

func isReconciliationListEntry(row submodelelements.ReconciliationElementRow) bool {
	return strings.HasPrefix(row.Path, row.ParentPath+"[") && strings.HasSuffix(row.Path, "]")
}

func reconciliationListSignatures(rows []submodelelements.ReconciliationElementRow) (map[string]string, error) {
	signatures := map[string]hash.Hash{}
	for _, row := range rows {
		if !strings.Contains(row.Path, "[") {
			continue
		}
		encoded, err := json.Marshal(row)
		if err != nil {
			return nil, fmt.Errorf("SMREPO-REBAC-LISTSIGNATURE %w", err)
		}
		for index, character := range row.Path {
			if character != ']' {
				continue
			}
			path := row.Path[:index+1]
			signature := signatures[path]
			if signature == nil {
				signature = sha256.New()
				signatures[path] = signature
			}
			_, _ = signature.Write(encoded)
		}
	}
	result := make(map[string]string, len(signatures))
	for path, signature := range signatures {
		result[path] = string(signature.Sum(nil))
	}
	return result, nil
}
