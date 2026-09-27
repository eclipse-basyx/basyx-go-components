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

import "sync"

// ReBACServiceProfile announces the experimental relationship-based access
// control management API (`$access` sub-resources) in service descriptions.
const ReBACServiceProfile = "https://basyx.org/aas/API/3/2/RelationshipBasedAccessControl/1.0"

var (
	serviceProfilesMu  sync.RWMutex
	additionalProfiles []string
)

// AddServiceProfile announces an optional capability of the running service
// in its self-description. Call it during startup.
func AddServiceProfile(profile string) {
	serviceProfilesMu.Lock()
	defer serviceProfilesMu.Unlock()
	for _, existing := range additionalProfiles {
		if existing == profile {
			return
		}
	}
	additionalProfiles = append(additionalProfiles, profile)
}

// ServiceProfiles returns the static profiles of a service followed by the
// optional profiles announced with AddServiceProfile.
func ServiceProfiles(profiles ...string) []string {
	serviceProfilesMu.RLock()
	defer serviceProfilesMu.RUnlock()
	result := make([]string, 0, len(profiles)+len(additionalProfiles))
	seen := make(map[string]struct{}, len(profiles)+len(additionalProfiles))
	for _, profile := range append(append([]string(nil), profiles...), additionalProfiles...) {
		if _, duplicate := seen[profile]; duplicate {
			continue
		}
		seen[profile] = struct{}{}
		result = append(result, profile)
	}
	return result
}
