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

package common

import (
	"os"
	"reflect"
	"testing"
)

func TestApplyAASPreconfigPathOverrides_UsesEnvOverride(t *testing.T) {
	const envKey = "GENERAL_AAS_PRECONFIG_PATHS"
	previousValue, hadPreviousValue := os.LookupEnv(envKey)
	defer func() {
		if hadPreviousValue {
			_ = os.Setenv(envKey, previousValue)
			return
		}
		_ = os.Unsetenv(envKey)
	}()

	if err := os.Setenv(envKey, " file:alpha.aasx , ./fixtures , "); err != nil {
		t.Fatalf("failed to set env: %v", err)
	}

	cfg := &Config{General: GeneralConfig{AASPreconfigPaths: []string{"should-be-overridden"}}}
	applyAASPreconfigPathOverrides(cfg)

	expected := []string{"file:alpha.aasx", "./fixtures"}
	if !reflect.DeepEqual(expected, cfg.General.AASPreconfigPaths) {
		t.Fatalf("expected %v, got %v", expected, cfg.General.AASPreconfigPaths)
	}
}

func TestApplyAASPreconfigPathOverrides_NormalizesConfigValues(t *testing.T) {
	const envKey = "GENERAL_AAS_PRECONFIG_PATHS"
	previousValue, hadPreviousValue := os.LookupEnv(envKey)
	defer func() {
		if hadPreviousValue {
			_ = os.Setenv(envKey, previousValue)
			return
		}
		_ = os.Unsetenv(envKey)
	}()
	_ = os.Unsetenv(envKey)

	cfg := &Config{General: GeneralConfig{AASPreconfigPaths: []string{" ./one ", "", "./two"}}}
	applyAASPreconfigPathOverrides(cfg)

	expected := []string{"./one", "./two"}
	if !reflect.DeepEqual(expected, cfg.General.AASPreconfigPaths) {
		t.Fatalf("expected %v, got %v", expected, cfg.General.AASPreconfigPaths)
	}
}
