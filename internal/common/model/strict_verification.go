package model

import "sync/atomic"

var strictVerificationEnabled atomic.Bool

// SetStrictVerificationEnabled toggles strict verification behavior for model validation.
func SetStrictVerificationEnabled(enabled bool) {
	strictVerificationEnabled.Store(enabled)
}

func isStrictVerificationEnabled() bool {
	return strictVerificationEnabled.Load()
}
