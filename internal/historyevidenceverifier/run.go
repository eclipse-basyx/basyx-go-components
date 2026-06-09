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

// Package historyevidenceverifier contains the operational implementation for
// cmd/historyevidenceverifier.
package historyevidenceverifier

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/eclipse-basyx/basyx-go-components/internal/common"
)

const (
	exitSuccess = 0
	exitFailure = 1
	exitUsage   = 2
)

// Run executes the history evidence verifier CLI.
//
// The function parses command-line arguments, loads the BaSyx configuration,
// dispatches the selected operation, and writes JSON reports to stdout or the
// configured output file. It returns a process exit code instead of calling
// os.Exit so tests and the thin cmd package can control process termination.
//
// Parameters:
//   - ctx: Context used for PostgreSQL and evidence-store operations.
//   - args: Command-line arguments without the executable name.
//   - stdout: Destination for JSON reports when -out is not set.
//   - stderr: Destination for flag usage and error messages.
//
// Returns:
//   - int: Process exit code.
func Run(ctx context.Context, args []string, stdout io.Writer, stderr io.Writer) int {
	stdout = fallbackWriter(stdout)
	stderr = fallbackWriter(stderr)
	options, err := parseFlags(args, stderr)
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return exitSuccess
		}
		return exitUsage
	}
	if err = validateCLIOptions(options); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitFailure
	}
	if err = run(ctx, options, stdout); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitFailure
	}
	return exitSuccess
}

func run(ctx context.Context, options cliOptions, stdout io.Writer) error {
	cfg, err := common.LoadConfig(options.configPath, common.QUIET)
	if err != nil {
		return err
	}
	if options.recover && strings.TrimSpace(options.recoveryCatalogPath) != "" {
		return recoverEvidence(ctx, cfg, nil, options, stdout)
	}
	db, err := openDatabase(cfg)
	if err != nil {
		return err
	}
	defer closeDatabase(db)
	if options.writeEvidence {
		return writeEvidence(ctx, cfg, db, options, stdout)
	}
	if options.catalogExport {
		return exportRecoveryCatalog(ctx, db, options, stdout)
	}
	if options.recover {
		return recoverEvidence(ctx, cfg, db, options, stdout)
	}
	return verifyEvidence(ctx, cfg, db, options, stdout)
}
