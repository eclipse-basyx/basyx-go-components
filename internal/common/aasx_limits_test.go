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
* MERCHANTABILITY, FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT.
* IN NO EVENT SHALL THE AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY
* CLAIM, DAMAGES OR OTHER LIABILITY, WHETHER IN AN ACTION OF CONTRACT,
* TORT OR OTHERWISE, ARISING FROM, OUT OF OR IN CONNECTION WITH THE
* SOFTWARE OR THE USE OR OTHER DEALINGS IN THE SOFTWARE.
*
* SPDX-License-Identifier: MIT
******************************************************************************/

package common

import (
	"bytes"
	"errors"
	"net/url"
	"testing"

	aasx "github.com/aas-core-works/aas-package3-golang/v2"
)

func TestAASXLimitsFromConfigMapsConfiguredValues(t *testing.T) {
	cfg := &Config{General: GeneralConfig{
		AASXMaxPartCount:              17,
		AASXMaxOPCMetadataSizeBytes:   23,
		AASXMaxPartExpandedSizeBytes:  31,
		AASXMaxTotalExpandedSizeBytes: 47,
		AASXMaxThumbnailSizeBytes:     13,
	}}

	limits := AASXLimitsFromConfig(cfg)
	if limits.MaxPartCount != 17 || limits.MaxOPCMetadataSizeBytes != 23 ||
		limits.MaxPartExpandedSizeBytes != 31 || limits.MaxTotalExpandedSizeBytes != 47 ||
		limits.MaxThumbnailSizeBytes != 13 {
		t.Fatalf("unexpected mapped limits: %+v", limits)
	}
}

func TestAASXReaderOptionsEnforceEveryConfiguredReaderLimit(t *testing.T) {
	packageBytes := buildAASXLimitFixture(t, bytes.Repeat([]byte("x"), 64))
	tests := []struct {
		name   string
		limits AASXLimits
	}{
		{
			name: "part count",
			limits: AASXLimits{MaxPartCount: 1, MaxOPCMetadataSizeBytes: 1 << 20,
				MaxPartExpandedSizeBytes: 1 << 20, MaxTotalExpandedSizeBytes: 1 << 20},
		},
		{
			name: "OPC metadata",
			limits: AASXLimits{MaxPartCount: 100, MaxOPCMetadataSizeBytes: 1,
				MaxPartExpandedSizeBytes: 1 << 20, MaxTotalExpandedSizeBytes: 1 << 20},
		},
		{
			name: "part expansion",
			limits: AASXLimits{MaxPartCount: 100, MaxOPCMetadataSizeBytes: 1 << 20,
				MaxPartExpandedSizeBytes: 32, MaxTotalExpandedSizeBytes: 1 << 20},
		},
		{
			name: "total expansion",
			limits: AASXLimits{MaxPartCount: 100, MaxOPCMetadataSizeBytes: 1 << 20,
				MaxPartExpandedSizeBytes: 1 << 20, MaxTotalExpandedSizeBytes: 32},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			reader, err := aasx.NewPackaging().OpenReadFromStream(bytes.NewReader(packageBytes), test.limits.ReaderOptions()...)
			if reader != nil {
				_ = reader.Close()
			}
			if !errors.Is(err, aasx.ErrReaderLimitExceeded) {
				t.Fatalf("expected reader-limit error, got %v", err)
			}
		})
	}
}

func TestAASXLimitsCheckThumbnailUsesStricterLimit(t *testing.T) {
	limits := AASXLimits{MaxPartExpandedSizeBytes: 128, MaxThumbnailSizeBytes: 16}
	if err := limits.CheckPartSize(17, false); err != nil {
		t.Fatalf("ordinary part should be accepted: %v", err)
	}
	if err := limits.CheckPartSize(17, true); !IsErrPayloadTooLarge(err) {
		t.Fatalf("expected typed thumbnail limit error, got %v", err)
	}
}

func buildAASXLimitFixture(t *testing.T, specification []byte) []byte {
	t.Helper()
	var destination bytes.Buffer
	writer, err := aasx.NewPackaging().CreateWriter(&destination)
	if err != nil {
		t.Fatal(err)
	}
	specificationURI, err := url.Parse("/aasx/content.json")
	if err != nil {
		t.Fatal(err)
	}
	specificationPart, err := writer.PutPartFromStream(specificationURI, "application/json", bytes.NewReader(specification))
	if err != nil {
		t.Fatal(err)
	}
	if err = writer.MakeSpec(specificationPart); err != nil {
		t.Fatal(err)
	}
	if err = writer.Close(); err != nil {
		t.Fatal(err)
	}
	return destination.Bytes()
}
