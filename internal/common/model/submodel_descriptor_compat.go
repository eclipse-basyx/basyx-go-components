package model

import "sync/atomic"

const (
	supplementalSemanticIdsKey        = "supplementalSemanticIds"
	supplementalSemanticIdSingularKey = "supplementalSemanticId"
)

var supportsSingularSupplementalSemanticId atomic.Bool

// SetSupportsSingularSupplementalSemanticId toggles support for singular supplemental semantic ID input.
func SetSupportsSingularSupplementalSemanticId(enabled bool) {
	supportsSingularSupplementalSemanticId.Store(enabled)
}

func useSingularSupplementalSemanticId() bool {
	return supportsSingularSupplementalSemanticId.Load()
}
