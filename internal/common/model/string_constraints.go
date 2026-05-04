package model

import (
	"errors"
	"regexp"
)

const (
	unicodeStringConstraintPattern = `^([\\x09\\x0a\\x0d\\x20-\\ud7ff\\ue000-\\ufffd]|\\ud800[\\udc00-\\udfff]|[\\ud801-\\udbfe][\\udc00-\\udfff]|\\udbff[\\udc00-\\udfff])*$`
	idShortConstraintPattern       = `^[a-zA-Z][a-zA-Z0-9_-]*[a-zA-Z0-9_]+$`
)

var idShortRegexp = regexp.MustCompile(idShortConstraintPattern)

func validateUnicodeStringConstraint(value string) error {
	for _, r := range value {
		if r == 0x9 || r == 0xA || r == 0xD {
			continue
		}
		if r >= 0x20 && r <= 0xD7FF {
			continue
		}
		if r >= 0xE000 && r <= 0xFFFD {
			continue
		}
		if r >= 0x10000 && r <= 0x10FFFF {
			continue
		}
		return errors.New(`must match "` + unicodeStringConstraintPattern + `"`)
	}
	return nil
}

func validateIDShortConstraint(value string) error {
	if !idShortRegexp.MatchString(value) {
		return errors.New(`must match "` + idShortConstraintPattern + `"`)
	}
	return nil
}
