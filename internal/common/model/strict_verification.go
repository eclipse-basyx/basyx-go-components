package model

import (
	"fmt"
	"strings"
	"sync/atomic"
)

// VerificationMode controls semantic request-body validation behavior.
type VerificationMode string

const (
	// VerificationModeOff skips semantic verification.
	VerificationModeOff VerificationMode = "off"
	// VerificationModePermissive logs semantic verification errors as warnings.
	VerificationModePermissive VerificationMode = "permissive"
	// VerificationModeStrict returns errors for semantic verification failures.
	VerificationModeStrict VerificationMode = "strict"
)

var verificationMode atomic.Value

func init() {
	verificationMode.Store(VerificationModeStrict)
}

// ParseVerificationMode validates and normalizes a verification mode string.
func ParseVerificationMode(raw string) (VerificationMode, error) {
	normalized := VerificationMode(strings.ToLower(strings.TrimSpace(raw)))
	switch normalized {
	case VerificationModeOff, VerificationModePermissive, VerificationModeStrict:
		return normalized, nil
	default:
		return "", fmt.Errorf("invalid strictVerification mode %q (allowed values: off, permissive, strict)", raw)
	}
}

// NormalizeVerificationMode ensures unknown or empty modes default to strict.
func NormalizeVerificationMode(mode VerificationMode) VerificationMode {
	switch mode {
	case VerificationModeOff, VerificationModePermissive, VerificationModeStrict:
		return mode
	default:
		return VerificationModeStrict
	}
}

// SetVerificationMode validates and stores the process-wide verification mode.
func SetVerificationMode(mode string) error {
	parsed, err := ParseVerificationMode(mode)
	if err != nil {
		return err
	}
	setVerificationModeValue(parsed)
	return nil
}

func setVerificationModeValue(mode VerificationMode) {
	verificationMode.Store(NormalizeVerificationMode(mode))
}

// GetVerificationMode returns the current process-wide verification mode.
func GetVerificationMode() VerificationMode {
	loaded, ok := verificationMode.Load().(VerificationMode)
	if !ok {
		return VerificationModeStrict
	}
	return NormalizeVerificationMode(loaded)
}

// SetStrictVerificationEnabled keeps backward compatibility for tests and legacy call paths.
func SetStrictVerificationEnabled(enabled bool) {
	if enabled {
		setVerificationModeValue(VerificationModeStrict)
		return
	}
	setVerificationModeValue(VerificationModeOff)
}
