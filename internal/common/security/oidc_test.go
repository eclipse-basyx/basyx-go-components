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
