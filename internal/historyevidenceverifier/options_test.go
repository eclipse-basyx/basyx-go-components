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

package historyevidenceverifier

import (
	"testing"

	"github.com/eclipse-basyx/basyx-go-components/internal/common/history"
	"github.com/stretchr/testify/require"
)

func TestValidateCLIOptionsRejectsManifestObjectKeyWithoutHash(t *testing.T) {
	err := validateCLIOptions(cliOptions{
		historyTable:      "aas_history",
		firstHistoryID:    1,
		lastHistoryID:     10,
		manifestObjectKey: "history-evidence/manifests/aas_history/1-10.json",
	})

	require.ErrorContains(t, err, "HISTORY-EVIDENCE-CLI-MANIFESTHASH")
}

func TestValidateCLIOptionsRejectsMultipleModes(t *testing.T) {
	err := validateCLIOptions(cliOptions{
		historyTable:   "aas_history",
		firstHistoryID: 1,
		lastHistoryID:  10,
		writeEvidence:  true,
		recover:        true,
	})

	require.ErrorContains(t, err, "HISTORY-EVIDENCE-CLI-MODE")
}

func TestValidateCLIOptionsRejectsRecoveryCatalogWithoutRecover(t *testing.T) {
	err := validateCLIOptions(cliOptions{
		historyTable:        "aas_history",
		firstHistoryID:      1,
		lastHistoryID:       10,
		recoveryCatalogPath: "catalog.json",
	})

	require.ErrorContains(t, err, "HISTORY-EVIDENCE-CLI-RECOVERYCATALOG")
}

func TestValidateCLIOptionsAllowsCatalogOnlyRecovery(t *testing.T) {
	err := validateCLIOptions(cliOptions{
		recover:             true,
		recoveryCatalogPath: "catalog.json",
	})

	require.NoError(t, err)
}

func TestValidateRecoveryCatalogSelectionRejectsMismatchedFlags(t *testing.T) {
	catalog := history.EvidenceRecoveryCatalog{
		HistoryTable:   "submodel_history",
		Identifier:     "sm-1",
		FirstHistoryID: 1,
		LastHistoryID:  5,
	}

	err := validateRecoveryCatalogSelection(catalog, cliOptions{historyTable: "aas_history"})

	require.ErrorContains(t, err, "HISTORY-EVIDENCE-CLI-CATALOGTABLE")
}
