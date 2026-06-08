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

// Package main implements the history evidence verifier and publisher CLI.
package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/history"
)

type cliOptions struct {
	configPath            string
	historyTable          string
	identifier            string
	firstHistoryID        int64
	lastHistoryID         int64
	writeEvidence         bool
	recover               bool
	catalogExport         bool
	recoveryCatalogPath   string
	outputPath            string
	manifestObjectKey     string
	manifestVersionID     string
	manifestSHA256        string
	signerKeyID           string
	requireSignedManifest bool
}

func main() {
	options := parseFlags()
	flag.Parse()
	ctx, stop := signal.NotifyContext(context.TODO(), os.Interrupt, syscall.SIGTERM)
	err := run(ctx, *options)
	stop()
	if err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func parseFlags() *cliOptions {
	options := &cliOptions{}
	flag.StringVar(&options.configPath, "config", "", "Path to BaSyx config YAML")
	flag.StringVar(&options.historyTable, "table", "", "History table, for example aas_history or submodel_history")
	flag.StringVar(&options.identifier, "identifier", "", "Optional entity identifier scope")
	flag.Int64Var(&options.firstHistoryID, "from", 0, "First history_id in the range")
	flag.Int64Var(&options.lastHistoryID, "to", 0, "Last history_id in the range")
	flag.BoolVar(&options.writeEvidence, "write", false, "Write snapshot and manifest artifacts before verifying")
	flag.BoolVar(&options.recover, "recover", false, "Recover/export verified history rows from WORM history_event artifacts")
	flag.BoolVar(&options.catalogExport, "catalog-export", false, "Export a recovery catalog from PostgreSQL evidence receipts")
	flag.StringVar(&options.recoveryCatalogPath, "recovery-catalog", "", "Recovery catalog JSON file to use instead of PostgreSQL receipt discovery")
	flag.StringVar(&options.outputPath, "out", "", "Optional JSON output file")
	flag.StringVar(&options.manifestObjectKey, "manifest-object-key", "", "Stored manifest object key to verify")
	flag.StringVar(&options.manifestVersionID, "manifest-version-id", "", "Stored manifest object version ID")
	flag.StringVar(&options.manifestSHA256, "manifest-sha256", "", "Expected SHA-256 for the stored manifest object")
	flag.StringVar(&options.signerKeyID, "signer-key-id", "", "Optional manifest signer key id")
	flag.BoolVar(&options.requireSignedManifest, "require-signed-manifest", false, "Reject unsigned manifests during verification")
	return options
}

func run(ctx context.Context, options cliOptions) error {
	if err := validateCLIOptions(options); err != nil {
		return err
	}
	cfg, err := common.LoadConfig(options.configPath, common.QUIET)
	if err != nil {
		return err
	}
	if options.recover && strings.TrimSpace(options.recoveryCatalogPath) != "" {
		return recoverEvidence(ctx, cfg, nil, options)
	}
	db, err := openDatabase(cfg)
	if err != nil {
		return err
	}
	defer func() {
		_ = db.Close()
	}()
	if options.writeEvidence {
		return writeEvidence(ctx, cfg, db, options)
	}
	if options.catalogExport {
		return exportRecoveryCatalog(ctx, db, options)
	}
	if options.recover {
		return recoverEvidence(ctx, cfg, db, options)
	}
	return verifyEvidence(ctx, cfg, db, options)
}

func validateCLIOptions(options cliOptions) error {
	modeCount := 0
	for _, enabled := range []bool{options.writeEvidence, options.recover, options.catalogExport} {
		if enabled {
			modeCount++
		}
	}
	if modeCount > 1 {
		return fmt.Errorf("HISTORY-EVIDENCE-CLI-MODE choose only one of -write, -recover, or -catalog-export")
	}
	if strings.TrimSpace(options.recoveryCatalogPath) != "" && !options.recover {
		return fmt.Errorf("HISTORY-EVIDENCE-CLI-RECOVERYCATALOG -recovery-catalog is only valid with -recover")
	}
	catalogRecovery := options.recover && strings.TrimSpace(options.recoveryCatalogPath) != ""
	if !catalogRecovery {
		if strings.TrimSpace(options.historyTable) == "" {
			return fmt.Errorf("HISTORY-EVIDENCE-CLI-TABLE -table is required")
		}
		if options.firstHistoryID < 1 {
			return fmt.Errorf("HISTORY-EVIDENCE-CLI-FROM -from must be positive")
		}
		if options.lastHistoryID < options.firstHistoryID {
			return fmt.Errorf("HISTORY-EVIDENCE-CLI-TO -to must be greater than or equal to -from")
		}
	}
	hasManifestObjectKey := strings.TrimSpace(options.manifestObjectKey) != ""
	hasManifestSHA256 := strings.TrimSpace(options.manifestSHA256) != ""
	if hasManifestObjectKey && !hasManifestSHA256 {
		return fmt.Errorf("HISTORY-EVIDENCE-CLI-MANIFESTHASH -manifest-sha256 is required when -manifest-object-key is set")
	}
	if hasManifestSHA256 && !hasManifestObjectKey {
		return fmt.Errorf("HISTORY-EVIDENCE-CLI-MANIFESTOBJECT -manifest-object-key is required when -manifest-sha256 is set")
	}
	if strings.TrimSpace(options.manifestVersionID) != "" && !hasManifestObjectKey {
		return fmt.Errorf("HISTORY-EVIDENCE-CLI-MANIFESTVERSION -manifest-object-key is required when -manifest-version-id is set")
	}
	return nil
}

func openDatabase(cfg *common.Config) (*sql.DB, error) {
	dsn := common.BuildPostgresDSN(cfg.Postgres)
	if err := common.ValidateSchemaVersionByDSN(dsn, common.CURRENT_DATABASE_VERSION); err != nil {
		return nil, err
	}
	db, err := common.NewDatabaseConnection(dsn)
	if err != nil {
		return nil, err
	}
	if cfg.Postgres.MaxOpenConnections > 0 {
		db.SetMaxOpenConns(cfg.Postgres.MaxOpenConnections)
	}
	if cfg.Postgres.MaxIdleConnections > 0 {
		db.SetMaxIdleConns(cfg.Postgres.MaxIdleConnections)
	}
	return db, nil
}

func writeEvidence(ctx context.Context, cfg *common.Config, db *sql.DB, options cliOptions) error {
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
	return writeJSONOutput(result, options.outputPath)
}

func verifyEvidence(ctx context.Context, cfg *common.Config, db *sql.DB, options cliOptions) error {
	verifyOptions := history.VerifyHistoryRangeOptions{
		HistoryTable:          options.historyTable,
		Identifier:            options.identifier,
		FirstHistoryID:        options.firstHistoryID,
		LastHistoryID:         options.lastHistoryID,
		RequireSignedManifest: cfg.History.Evidence.Signing.Required || options.requireSignedManifest,
	}
	verifier, err := newManifestVerifier(cfg)
	if err != nil {
		return err
	}
	verifyOptions.ManifestVerifier = verifier
	store, err := optionalS3EvidenceStore(ctx, cfg)
	if err != nil {
		return err
	}
	verifyOptions.EvidenceStore = store
	if strings.TrimSpace(options.manifestObjectKey) != "" {
		if store == nil {
			store, err = newS3EvidenceStore(ctx, cfg)
			if err != nil {
				return err
			}
			verifyOptions.EvidenceStore = store
		}
		ref := history.EvidenceReference{
			Provider:  history.EvidenceProviderS3,
			Bucket:    cfg.History.Evidence.Bucket,
			ObjectKey: strings.TrimSpace(options.manifestObjectKey),
			VersionID: strings.TrimSpace(options.manifestVersionID),
		}
		object, err := store.GetArtifact(ctx, ref)
		if err != nil {
			return err
		}
		verifyOptions.ManifestArtifactData = object.Data
		verifyOptions.ManifestArtifactContentType = object.ContentType
		verifyOptions.EvidenceStore = store
		verifyOptions.ManifestArtifactRef = ref
		verifyOptions.ManifestArtifactHash = strings.TrimSpace(options.manifestSHA256)
	}
	report, err := history.VerifyHistoryRange(ctx, db, verifyOptions)
	if err != nil {
		return err
	}
	if printErr := writeJSONOutput(report, options.outputPath); printErr != nil {
		return printErr
	}
	if !report.Valid {
		return fmt.Errorf("HISTORY-EVIDENCE-CLI-VERIFYFAILED verification report contains critical findings")
	}
	return nil
}

func exportRecoveryCatalog(ctx context.Context, db *sql.DB, options cliOptions) error {
	catalog, err := history.LoadEvidenceRecoveryCatalog(ctx, db, history.EvidenceRecoveryCatalogOptions{
		HistoryTable:   options.historyTable,
		Identifier:     options.identifier,
		FirstHistoryID: options.firstHistoryID,
		LastHistoryID:  options.lastHistoryID,
	})
	if err != nil {
		return err
	}
	return writeJSONOutput(catalog, options.outputPath)
}

func recoverEvidence(ctx context.Context, cfg *common.Config, db *sql.DB, options cliOptions) error {
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
	if printErr := writeJSONOutput(report, options.outputPath); printErr != nil {
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

func newS3EvidenceStore(ctx context.Context, cfg *common.Config) (*history.S3EvidenceStore, error) {
	if strings.ToLower(strings.TrimSpace(cfg.History.Evidence.Provider)) != history.EvidenceProviderS3 {
		return nil, fmt.Errorf("HISTORY-EVIDENCE-CLI-PROVIDER history.evidence.provider must be s3")
	}
	return history.NewS3EvidenceStore(ctx, history.S3EvidenceStoreConfig{
		Bucket:          cfg.History.Evidence.Bucket,
		Prefix:          cfg.History.Evidence.Prefix,
		Region:          cfg.History.Evidence.Region,
		Endpoint:        cfg.History.Evidence.Endpoint,
		AccessKeyID:     cfg.History.Evidence.AccessKeyID,
		SecretAccessKey: cfg.History.Evidence.SecretAccessKey,
		UsePathStyle:    cfg.History.Evidence.UsePathStyle,
		RetentionMode:   cfg.History.Evidence.RetentionMode,
		RetentionDays:   cfg.History.Evidence.RetentionDays,
	})
}

func optionalS3EvidenceStore(ctx context.Context, cfg *common.Config) (*history.S3EvidenceStore, error) {
	if strings.ToLower(strings.TrimSpace(cfg.History.Evidence.Provider)) != history.EvidenceProviderS3 {
		return nil, nil
	}
	if strings.TrimSpace(cfg.History.Evidence.Bucket) == "" {
		return nil, nil
	}
	return newS3EvidenceStore(ctx, cfg)
}

func newManifestSigner(cfg *common.Config, keyID string) (*history.ManifestJWSSigner, error) {
	keyPath := strings.TrimSpace(cfg.History.Evidence.Signing.PrivateKeyPath)
	if keyPath == "" {
		keyPath = strings.TrimSpace(cfg.JWS.PrivateKeyPath)
	}
	if keyPath == "" {
		return nil, nil
	}
	return history.NewManifestJWSSignerFromKeyFile(keyPath, keyID)
}

func newManifestVerifier(cfg *common.Config) (*history.ManifestJWSVerifier, error) {
	keyPath := strings.TrimSpace(cfg.History.Evidence.Signing.PublicKeyPath)
	if keyPath == "" {
		return nil, nil
	}
	return history.NewManifestJWSVerifierFromKeyFile(keyPath)
}

func writeJSONOutput(value any, outputPath string) error {
	encoded, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return fmt.Errorf("HISTORY-EVIDENCE-CLI-PRINTJSON %w", err)
	}
	if strings.TrimSpace(outputPath) == "" {
		_, err = fmt.Fprintln(os.Stdout, string(encoded))
		return err
	}
	return os.WriteFile(strings.TrimSpace(outputPath), append(encoded, '\n'), 0o600)
}
