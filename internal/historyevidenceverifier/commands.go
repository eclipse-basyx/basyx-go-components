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
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/history"
)

func writeEvidence(ctx context.Context, cfg *common.Config, db *sql.DB, options cliOptions, stdout io.Writer) error {
	if !cfg.History.Evidence.Enabled {
		return fmt.Errorf("HISTORY-EVIDENCE-CLI-DISABLED history.evidence.enabled must be true for -write")
	}
	store, err := newS3EvidenceStore(ctx, cfg)
	if err != nil {
		return err
	}
	signer, err := newManifestSigner(cfg, options.signerKeyID)
	if err != nil {
		return err
	}
	if cfg.History.Evidence.Signing.Required && signer == nil {
		return fmt.Errorf("HISTORY-EVIDENCE-CLI-SIGNINGREQUIRED history.evidence.signing.required needs a private signing key for -write")
	}
	result, err := history.WriteHistoryEvidence(ctx, history.WriteHistoryEvidenceOptions{
		DB:             db,
		Store:          store,
		HistoryTable:   options.historyTable,
		Identifier:     options.identifier,
		FirstHistoryID: options.firstHistoryID,
		LastHistoryID:  options.lastHistoryID,
		Signer:         signer,
	})
	if err != nil {
		return err
	}
	return writeJSONOutput(result, options.outputPath, stdout)
}

func verifyEvidence(ctx context.Context, cfg *common.Config, db *sql.DB, options cliOptions, stdout io.Writer) error {
	verifyOptions, err := buildVerifyOptions(ctx, cfg, options)
	if err != nil {
		return err
	}
	report, err := history.VerifyHistoryRange(ctx, db, verifyOptions)
	if err != nil {
		return err
	}
	if printErr := writeJSONOutput(report, options.outputPath, stdout); printErr != nil {
		return printErr
	}
	if !report.Valid {
		return fmt.Errorf("HISTORY-EVIDENCE-CLI-VERIFYFAILED verification report contains critical findings")
	}
	return nil
}

func buildVerifyOptions(ctx context.Context, cfg *common.Config, options cliOptions) (history.VerifyHistoryRangeOptions, error) {
	verifyOptions := history.VerifyHistoryRangeOptions{
		HistoryTable:          options.historyTable,
		Identifier:            options.identifier,
		FirstHistoryID:        options.firstHistoryID,
		LastHistoryID:         options.lastHistoryID,
		RequireSignedManifest: cfg.History.Evidence.Signing.Required || options.requireSignedManifest,
	}
	verifier, err := newManifestVerifier(cfg)
	if err != nil {
		return history.VerifyHistoryRangeOptions{}, err
	}
	verifyOptions.ManifestVerifier = verifier
	store, err := optionalS3EvidenceStore(ctx, cfg)
	if err != nil {
		return history.VerifyHistoryRangeOptions{}, err
	}
	verifyOptions.EvidenceStore = store
	if strings.TrimSpace(options.manifestObjectKey) == "" {
		return verifyOptions, nil
	}
	return withManifestArtifact(ctx, cfg, options, verifyOptions)
}

func withManifestArtifact(ctx context.Context, cfg *common.Config, options cliOptions, verifyOptions history.VerifyHistoryRangeOptions) (history.VerifyHistoryRangeOptions, error) {
	store := verifyOptions.EvidenceStore
	if store == nil {
		newStore, err := newS3EvidenceStore(ctx, cfg)
		if err != nil {
			return history.VerifyHistoryRangeOptions{}, err
		}
		store = newStore
	}
	ref := history.EvidenceReference{
		Provider:  history.EvidenceProviderS3,
		Bucket:    cfg.History.Evidence.Bucket,
		ObjectKey: strings.TrimSpace(options.manifestObjectKey),
		VersionID: strings.TrimSpace(options.manifestVersionID),
	}
	object, err := store.GetArtifact(ctx, ref)
	if err != nil {
		return history.VerifyHistoryRangeOptions{}, err
	}
	verifyOptions.ManifestArtifactData = object.Data
	verifyOptions.ManifestArtifactContentType = object.ContentType
	verifyOptions.EvidenceStore = store
	verifyOptions.ManifestArtifactRef = ref
	verifyOptions.ManifestArtifactHash = strings.TrimSpace(options.manifestSHA256)
	return verifyOptions, nil
}

func exportRecoveryCatalog(ctx context.Context, db *sql.DB, options cliOptions, stdout io.Writer) error {
	catalog, err := history.LoadEvidenceRecoveryCatalog(ctx, db, history.EvidenceRecoveryCatalogOptions{
		HistoryTable:   options.historyTable,
		Identifier:     options.identifier,
		FirstHistoryID: options.firstHistoryID,
		LastHistoryID:  options.lastHistoryID,
	})
	if err != nil {
		return err
	}
	return writeJSONOutput(catalog, options.outputPath, stdout)
}

func recoverEvidence(ctx context.Context, cfg *common.Config, db *sql.DB, options cliOptions, stdout io.Writer) error {
	store, err := newS3EvidenceStore(ctx, cfg)
	if err != nil {
		return err
	}
	catalog, err := recoveryCatalog(ctx, db, options)
	if err != nil {
		return err
	}
	report, err := history.RecoverHistoryFromEvidence(ctx, store, catalog)
	if err != nil {
		return err
	}
	if printErr := writeJSONOutput(report, options.outputPath, stdout); printErr != nil {
		return printErr
	}
	if !report.Valid {
		return fmt.Errorf("HISTORY-EVIDENCE-CLI-RECOVERYFAILED recovery report contains critical findings")
	}
	return nil
}

func recoveryCatalog(ctx context.Context, db *sql.DB, options cliOptions) (history.EvidenceRecoveryCatalog, error) {
	if strings.TrimSpace(options.recoveryCatalogPath) == "" {
		return history.LoadEvidenceRecoveryCatalog(ctx, db, history.EvidenceRecoveryCatalogOptions{
			HistoryTable:   options.historyTable,
			Identifier:     options.identifier,
			FirstHistoryID: options.firstHistoryID,
			LastHistoryID:  options.lastHistoryID,
		})
	}
	data, err := os.ReadFile(strings.TrimSpace(options.recoveryCatalogPath))
	if err != nil {
		return history.EvidenceRecoveryCatalog{}, fmt.Errorf("HISTORY-EVIDENCE-CLI-READCATALOG %w", err)
	}
	var catalog history.EvidenceRecoveryCatalog
	if err = json.Unmarshal(data, &catalog); err != nil {
		return history.EvidenceRecoveryCatalog{}, fmt.Errorf("HISTORY-EVIDENCE-CLI-DECODECATALOG %w", err)
	}
	if err = validateRecoveryCatalogSelection(catalog, options); err != nil {
		return history.EvidenceRecoveryCatalog{}, err
	}
	return catalog, nil
}

func validateRecoveryCatalogSelection(catalog history.EvidenceRecoveryCatalog, options cliOptions) error {
	if strings.TrimSpace(options.historyTable) != "" && strings.TrimSpace(options.historyTable) != catalog.HistoryTable {
		return fmt.Errorf("HISTORY-EVIDENCE-CLI-CATALOGTABLE -table does not match recovery catalog")
	}
	if strings.TrimSpace(options.identifier) != "" && strings.TrimSpace(options.identifier) != strings.TrimSpace(catalog.Identifier) {
		return fmt.Errorf("HISTORY-EVIDENCE-CLI-CATALOGIDENTIFIER -identifier does not match recovery catalog")
	}
	if options.firstHistoryID > 0 && options.firstHistoryID != catalog.FirstHistoryID {
		return fmt.Errorf("HISTORY-EVIDENCE-CLI-CATALOGFROM -from does not match recovery catalog")
	}
	if options.lastHistoryID > 0 && options.lastHistoryID != catalog.LastHistoryID {
		return fmt.Errorf("HISTORY-EVIDENCE-CLI-CATALOGTO -to does not match recovery catalog")
	}
	return nil
}
