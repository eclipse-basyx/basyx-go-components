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
	"flag"
	"fmt"
	"io"
	"strings"
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
	mutationEvidence      bool
}

func parseFlags(args []string, stderr io.Writer) (cliOptions, error) {
	options := cliOptions{}
	flags := flag.NewFlagSet("historyevidenceverifier", flag.ContinueOnError)
	flags.SetOutput(stderr)
	bindFlags(flags, &options)
	if err := flags.Parse(args); err != nil {
		return cliOptions{}, err
	}
	return options, nil
}

func bindFlags(flags *flag.FlagSet, options *cliOptions) {
	flags.StringVar(&options.configPath, "config", "", "Path to BaSyx config YAML")
	flags.StringVar(&options.historyTable, "table", "", "History table, for example aas_history or submodel_history")
	flags.StringVar(&options.identifier, "identifier", "", "Optional entity identifier scope")
	flags.Int64Var(&options.firstHistoryID, "from", 0, "First history_id in the range")
	flags.Int64Var(&options.lastHistoryID, "to", 0, "Last history_id in the range")
	flags.BoolVar(&options.writeEvidence, "write", false, "Write snapshot and manifest artifacts before verifying")
	flags.BoolVar(&options.recover, "recover", false, "Recover/export verified history rows from WORM history_event artifacts")
	flags.BoolVar(&options.catalogExport, "catalog-export", false, "Export a recovery catalog from PostgreSQL evidence receipts")
	flags.StringVar(&options.recoveryCatalogPath, "recovery-catalog", "", "Recovery catalog JSON file to use instead of PostgreSQL receipt discovery")
	flags.StringVar(&options.outputPath, "out", "", "Optional JSON output file")
	flags.StringVar(&options.manifestObjectKey, "manifest-object-key", "", "Stored manifest object key to verify")
	flags.StringVar(&options.manifestVersionID, "manifest-version-id", "", "Stored manifest object version ID")
	flags.StringVar(&options.manifestSHA256, "manifest-sha256", "", "Expected SHA-256 for the stored manifest object")
	flags.StringVar(&options.signerKeyID, "signer-key-id", "", "Optional manifest signer key id")
	flags.BoolVar(&options.requireSignedManifest, "require-signed-manifest", false, "Reject unsigned manifests during verification")
	flags.BoolVar(&options.mutationEvidence, "mutation", false, "Verify independent mutation evidence; -from and -to select event sequences")
}

func validateCLIOptions(options cliOptions) error {
	if enabledModeCount(options) > 1 {
		return fmt.Errorf("HISTORY-EVIDENCE-CLI-MODE choose only one of -write, -recover, or -catalog-export")
	}
	if strings.TrimSpace(options.recoveryCatalogPath) != "" && !options.recover {
		return fmt.Errorf("HISTORY-EVIDENCE-CLI-RECOVERYCATALOG -recovery-catalog is only valid with -recover")
	}
	if options.mutationEvidence {
		if options.writeEvidence || options.catalogExport || strings.TrimSpace(options.recoveryCatalogPath) != "" {
			return fmt.Errorf("HISTORY-EVIDENCE-CLI-MUTATIONMODE -mutation supports verification and direct recovery only")
		}
		if strings.TrimSpace(options.identifier) == "" {
			return fmt.Errorf("HISTORY-EVIDENCE-CLI-MUTATIONIDENTIFIER -identifier is required with -mutation")
		}
	}
	if !isCatalogRecovery(options) {
		if err := validateHistoryRangeOptions(options); err != nil {
			return err
		}
	}
	return validateManifestReferenceOptions(options)
}

func enabledModeCount(options cliOptions) int {
	count := 0
	for _, enabled := range []bool{options.writeEvidence, options.recover, options.catalogExport} {
		if enabled {
			count++
		}
	}
	return count
}

func isCatalogRecovery(options cliOptions) bool {
	return options.recover && strings.TrimSpace(options.recoveryCatalogPath) != ""
}

func validateHistoryRangeOptions(options cliOptions) error {
	if strings.TrimSpace(options.historyTable) == "" {
		return fmt.Errorf("HISTORY-EVIDENCE-CLI-TABLE -table is required")
	}
	if options.firstHistoryID < 1 {
		return fmt.Errorf("HISTORY-EVIDENCE-CLI-FROM -from must be positive")
	}
	if options.lastHistoryID < options.firstHistoryID {
		return fmt.Errorf("HISTORY-EVIDENCE-CLI-TO -to must be greater than or equal to -from")
	}
	return nil
}

func validateManifestReferenceOptions(options cliOptions) error {
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
