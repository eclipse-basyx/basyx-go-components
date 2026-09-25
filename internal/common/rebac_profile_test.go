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
	"slices"
	"testing"
)

func TestProfilesWithReBACFollowsEffectiveConfiguration(t *testing.T) {
	base := []string{"base-profile"}
	disabled := ProfilesWithReBAC(ContextWithConfig(t.Context(), &Config{}), base)
	if !slices.Equal(disabled, base) {
		t.Fatalf("disabled profiles = %v", disabled)
	}
	enabled := ProfilesWithReBAC(ContextWithConfig(t.Context(), &Config{ReBAC: ReBACConfig{Enabled: true}}), base)
	if !slices.Equal(enabled, []string{"base-profile", ReBACProfile}) {
		t.Fatalf("enabled profiles = %v", enabled)
	}
	if !slices.Equal(base, []string{"base-profile"}) {
		t.Fatalf("input profiles mutated = %v", base)
	}
}

func TestProfilesWithReBACDoesNotDuplicateProfile(t *testing.T) {
	profiles := []string{"base-profile", ReBACProfile}
	actual := ProfilesWithReBAC(ContextWithConfig(t.Context(), &Config{ReBAC: ReBACConfig{Enabled: true}}), profiles)
	if !slices.Equal(actual, profiles) {
		t.Fatalf("profiles = %v", actual)
	}
}
