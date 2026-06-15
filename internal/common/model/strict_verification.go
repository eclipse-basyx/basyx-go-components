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

package model

import (
	"fmt"
	"strings"
	"sync/atomic"
)

// VerificationMode controls semantic request-body validation behavior.
type VerificationMode string

const (
	// VerificationModeOff skips semantic verification.
	VerificationModeOff VerificationMode = "off"
	// VerificationModePermissive logs semantic verification errors as warnings.
	VerificationModePermissive VerificationMode = "permissive"
	// VerificationModeStrict returns errors for semantic verification failures.
	VerificationModeStrict VerificationMode = "strict"
)

var verificationMode atomic.Value

func init() {
	verificationMode.Store(VerificationModeStrict)
}

// ParseVerificationMode validates and normalizes a verification mode string.
func ParseVerificationMode(raw string) (VerificationMode, error) {
	normalized := VerificationMode(strings.ToLower(strings.TrimSpace(raw)))
	switch normalized {
	case VerificationModeOff, VerificationModePermissive, VerificationModeStrict:
		return normalized, nil
	default:
		return "", fmt.Errorf("invalid strictVerification mode %q (allowed values: off, permissive, strict)", raw)
	}
}

// NormalizeVerificationMode ensures unknown or empty modes default to strict.
func NormalizeVerificationMode(mode VerificationMode) VerificationMode {
	switch mode {
	case VerificationModeOff, VerificationModePermissive, VerificationModeStrict:
		return mode
	default:
		return VerificationModeStrict
	}
}

// SetVerificationMode validates and stores the process-wide verification mode.
func SetVerificationMode(mode string) error {
	parsed, err := ParseVerificationMode(mode)
	if err != nil {
		return err
	}
	setVerificationModeValue(parsed)
	return nil
}

func setVerificationModeValue(mode VerificationMode) {
	verificationMode.Store(NormalizeVerificationMode(mode))
}

// GetVerificationMode returns the current process-wide verification mode.
func GetVerificationMode() VerificationMode {
	loaded, ok := verificationMode.Load().(VerificationMode)
	if !ok {
		return VerificationModeStrict
	}
	return NormalizeVerificationMode(loaded)
}

// SetStrictVerificationEnabled keeps backward compatibility for tests and legacy call paths.
func SetStrictVerificationEnabled(enabled bool) {
	if enabled {
		setVerificationModeValue(VerificationModeStrict)
		return
	}
	setVerificationModeValue(VerificationModeOff)
}
