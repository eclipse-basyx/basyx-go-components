package model

import "sync/atomic"

var strictVerificationEnabled atomic.Bool

func SetStrictVerificationEnabled(enabled bool) {
	strictVerificationEnabled.Store(enabled)
}

func isStrictVerificationEnabled() bool {
	return strictVerificationEnabled.Load()
}
