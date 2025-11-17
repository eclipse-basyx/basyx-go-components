package auth

import (
	"encoding/json"
	"os"
	"time"
)

// EvalInput is the minimal set of request properties the ABAC engine needs to
// evaluate a decision. IssuedUTC should be in UTC.
type EvalInput struct {
	Method    string
	Path      string
	Claims    Claims
	IssuedUTC time.Time
}

// Claims represents token claims extracted from a verified ID token.
type Claims map[string]any

// LoadEvalInput reads a JSON file and returns an EvalInput object. This function is mainly used in the test setup.
func LoadEvalInput(filename string) (EvalInput, error) {
	var input EvalInput

	file, err := os.Open(filename)
	if err != nil {
		return input, err
	}
	defer func() {
		_ = file.Close()
	}()

	if err := json.NewDecoder(file).Decode(&input); err != nil {
		return input, err
	}

	return input, nil
}
