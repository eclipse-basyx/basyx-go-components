package model

import (
	"errors"
	"log"
	"strings"

	"github.com/aas-core-works/aas-core3.1-golang/verification"
)

// ValidateWithMode executes semantic verification based on the selected mode.
// In off mode, verification is skipped. In permissive mode, verification errors
// are logged as warnings. In strict mode, verification errors are returned.
func ValidateWithMode(mode VerificationMode, warningContext string, verify func(func(*verification.VerificationError) bool), strictErrorFactory func(string) error) error {
	normalizedMode := NormalizeVerificationMode(mode)
	if normalizedMode == VerificationModeOff {
		return nil
	}

	validationErrors := make([]string, 0)
	verify(func(verErr *verification.VerificationError) bool {
		validationErrors = append(validationErrors, verErr.Error())
		return false
	})

	if len(validationErrors) == 0 {
		return nil
	}

	joined := strings.Join(validationErrors, "; ")
	if normalizedMode == VerificationModePermissive {
		log.Printf("WARN: %s: %s", warningContext, joined)
		return nil
	}

	if strictErrorFactory != nil {
		return strictErrorFactory(joined)
	}
	return errors.New(joined)
}
