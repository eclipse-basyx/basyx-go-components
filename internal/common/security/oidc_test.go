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
*******************************************************************************/

package auth

import "testing"

func TestOIDCVerifierConfig_UsesClientIDWhenAudienceProvided(t *testing.T) {
	t.Parallel()

	cfg := oidcVerifierConfig("discovery-service")
	if cfg == nil {
		t.Fatalf("expected verifier config, got nil")
	}
	if cfg.SkipClientIDCheck {
		t.Fatalf("expected SkipClientIDCheck=false when audience is provided")
	}
	if cfg.ClientID != "discovery-service" {
		t.Fatalf("expected ClientID=discovery-service, got %q", cfg.ClientID)
	}
}

func TestOIDCVerifierConfig_SkipsClientIDCheckWhenAudienceMissing(t *testing.T) {
	t.Parallel()

	cfg := oidcVerifierConfig("   ")
	if cfg == nil {
		t.Fatalf("expected verifier config, got nil")
	}
	if !cfg.SkipClientIDCheck {
		t.Fatalf("expected SkipClientIDCheck=true when audience is missing")
	}
	if cfg.ClientID != "" {
		t.Fatalf("expected empty ClientID when audience is missing, got %q", cfg.ClientID)
	}
}
