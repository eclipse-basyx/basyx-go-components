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
// Author: Aaron Zielstorff ( Fraunhofer IESE )

package common

import (
	"reflect"
	"testing"
)

func TestServiceProfilesAppendsAnnouncedProfilesOnce(t *testing.T) {
	serviceProfilesMu.Lock()
	previous := additionalProfiles
	additionalProfiles = nil
	serviceProfilesMu.Unlock()
	t.Cleanup(func() {
		serviceProfilesMu.Lock()
		additionalProfiles = previous
		serviceProfilesMu.Unlock()
	})

	const static = "https://example.org/profile/static"
	if got := ServiceProfiles(static); !reflect.DeepEqual(got, []string{static}) {
		t.Fatalf("unexpected profiles without announcements: %v", got)
	}

	AddServiceProfile(ReBACServiceProfile)
	AddServiceProfile(ReBACServiceProfile)
	got := ServiceProfiles(static, ReBACServiceProfile, static)
	want := []string{static, ReBACServiceProfile}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("expected %v, got %v", want, got)
	}
}
