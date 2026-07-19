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
******************************************************************************/

package history

import (
	"encoding/hex"
	"fmt"
	"strings"
	"time"
)

func validateCommittedEvidenceReceipt(receipt EvidenceReceipt, expectedSHA256 string, expectedSize int64, now time.Time) error {
	if receipt.Reference.Provider != EvidenceProviderS3 || strings.TrimSpace(receipt.Reference.ObjectKey) == "" || strings.TrimSpace(receipt.Reference.VersionID) == "" {
		return fmt.Errorf("provider, object key, and immutable object version are required")
	}
	if !validSHA256(receipt.SHA256) || !strings.EqualFold(receipt.SHA256, expectedSHA256) || receipt.SizeBytes != expectedSize {
		return fmt.Errorf("receipt digest or size does not match the stored artifact")
	}
	mode := strings.ToLower(strings.TrimSpace(receipt.RetentionMode))
	if mode != "governance" && mode != "compliance" {
		return fmt.Errorf("WORM retention mode is missing or unsupported")
	}
	if receipt.RetainUntil == nil || !receipt.RetainUntil.After(now.UTC()) {
		return fmt.Errorf("WORM retain-until timestamp is missing or not in the future")
	}
	return nil
}

func validSHA256(value string) bool {
	if len(value) != 64 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}
