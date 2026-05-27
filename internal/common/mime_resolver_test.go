package common

import "testing"

func TestResolveUploadedContentType(t *testing.T) {
	tests := []struct {
		name                string
		detected            string
		declared            string
		fileName            string
		expectedContentType string
		expectedMismatch    bool
	}{
		{
			name:                "detected specific wins",
			detected:            "image/gif",
			declared:            "image/png",
			fileName:            "demo.bin",
			expectedContentType: "image/gif",
			expectedMismatch:    true,
		},
		{
			name:                "weak detected falls back to declared",
			detected:            "application/octet-stream",
			declared:            "image/tiff",
			fileName:            "demo.bin",
			expectedContentType: "image/tiff",
			expectedMismatch:    false,
		},
		{
			name:                "weak detected with invalid declared falls back to extension",
			detected:            "application/octet-stream",
			declared:            "not/a valid content type",
			fileName:            "picture.tif",
			expectedContentType: "image/tiff",
			expectedMismatch:    false,
		},
		{
			name:                "all weak falls back to binary",
			detected:            "application/octet-stream",
			declared:            "",
			fileName:            "",
			expectedContentType: "application/octet-stream",
			expectedMismatch:    false,
		},
		{
			name:                "detected with parameters normalized",
			detected:            "text/plain; charset=utf-8",
			declared:            "text/plain",
			fileName:            "doc.txt",
			expectedContentType: "text/plain",
			expectedMismatch:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resolved, mismatch := ResolveUploadedContentType(tt.detected, tt.declared, tt.fileName)

			if resolved != tt.expectedContentType {
				t.Fatalf("expected content type %q, got %q", tt.expectedContentType, resolved)
			}
			if mismatch != tt.expectedMismatch {
				t.Fatalf("expected mismatch %t, got %t", tt.expectedMismatch, mismatch)
			}
		})
	}
}
