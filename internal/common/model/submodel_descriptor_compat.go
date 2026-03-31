package model

import "sync/atomic"

const (
	supplementalSemanticIdsKey        = "supplementalSemanticIds"
	supplementalSemanticIdSingularKey = "supplementalSemanticId"
)

var supportsSingularSupplementalSemanticId atomic.Bool

func SetSupportsSingularSupplementalSemanticId(enabled bool) {
	supportsSingularSupplementalSemanticId.Store(enabled)
}

func useSingularSupplementalSemanticId() bool {
	return supportsSingularSupplementalSemanticId.Load()
}
