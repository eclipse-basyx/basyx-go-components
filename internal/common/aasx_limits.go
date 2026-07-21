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
	"context"
	"fmt"

	aasx "github.com/aas-core-works/aas-package3-golang/v2"
)

const (
	defaultAASXMaxPartCount              = 10000
	defaultAASXMaxOPCMetadataSizeBytes   = 16 << 20
	defaultAASXMaxPartExpandedSizeBytes  = 128 << 20
	defaultAASXMaxTotalExpandedSizeBytes = 128 << 20
	defaultAASXMaxThumbnailSizeBytes     = 16 << 20
)

// AASXLimits contains the bounded-memory limits applied to AASX readers and writers.
// All values are required to be positive after configuration defaults are applied.
type AASXLimits struct {
	// MaxPartCount is the maximum number of non-directory ZIP entries.
	MaxPartCount uint64
	// MaxOPCMetadataSizeBytes is the maximum combined expanded OPC metadata size.
	MaxOPCMetadataSizeBytes uint64
	// MaxPartExpandedSizeBytes is the maximum expanded size of one payload part.
	MaxPartExpandedSizeBytes uint64
	// MaxTotalExpandedSizeBytes is the maximum combined expanded payload size.
	MaxTotalExpandedSizeBytes uint64
	// MaxThumbnailSizeBytes is the maximum expanded thumbnail size.
	MaxThumbnailSizeBytes uint64
}

// AASXLimitsFromConfig resolves bounded-memory AASX limits from configuration.
//
// Parameters:
//   - cfg: Process configuration. Nil or non-positive fields use secure defaults.
//
// Returns:
//   - AASXLimits: Fully populated limits suitable for package processing.
func AASXLimitsFromConfig(cfg *Config) AASXLimits {
	if cfg == nil {
		return defaultAASXLimits()
	}
	limits := AASXLimits{
		MaxPartCount:              positiveUint64FromInt(cfg.General.AASXMaxPartCount, defaultAASXMaxPartCount),
		MaxOPCMetadataSizeBytes:   positiveUint64(cfg.General.AASXMaxOPCMetadataSizeBytes, defaultAASXMaxOPCMetadataSizeBytes),
		MaxPartExpandedSizeBytes:  positiveUint64(cfg.General.AASXMaxPartExpandedSizeBytes, defaultAASXMaxPartExpandedSizeBytes),
		MaxTotalExpandedSizeBytes: positiveUint64(cfg.General.AASXMaxTotalExpandedSizeBytes, defaultAASXMaxTotalExpandedSizeBytes),
		MaxThumbnailSizeBytes:     positiveUint64(cfg.General.AASXMaxThumbnailSizeBytes, defaultAASXMaxThumbnailSizeBytes),
	}
	return limits
}

// AASXLimitsFromContext resolves AASX limits from request-scoped configuration.
//
// Parameters:
//   - ctx: Request context populated by ConfigMiddleware.
//
// Returns:
//   - AASXLimits: Request limits, or secure defaults when no configuration exists.
func AASXLimitsFromContext(ctx context.Context) AASXLimits {
	cfg, _ := ConfigFromContext(ctx)
	return AASXLimitsFromConfig(cfg)
}

// ReaderOptions converts limits to aas-package3 lazy-reader options.
//
// Returns:
//   - []aasx.ReaderOption: Options covering entry count, metadata, per-part, and total expansion.
func (limits AASXLimits) ReaderOptions() []aasx.ReaderOption {
	return []aasx.ReaderOption{
		aasx.WithMaxPartCount(limits.MaxPartCount),
		aasx.WithMaxOPCMetadataBytes(limits.MaxOPCMetadataSizeBytes),
		aasx.WithMaxPartExpandedBytes(limits.MaxPartExpandedSizeBytes),
		aasx.WithMaxTotalExpandedBytes(limits.MaxTotalExpandedSizeBytes),
	}
}

// CheckPartSize validates one expanded package part before it is consumed.
//
// Parameters:
//   - size: Expanded part size in bytes.
//   - thumbnail: Whether to apply the stricter thumbnail limit.
//
// Returns:
//   - error: A 413-classified error when the applicable limit is exceeded.
func (limits AASXLimits) CheckPartSize(size uint64, thumbnail bool) error {
	maximum := limits.MaxPartExpandedSizeBytes
	label := "part"
	if thumbnail {
		maximum = limits.MaxThumbnailSizeBytes
		label = "thumbnail"
	}
	if size > maximum {
		return NewErrPayloadTooLarge(fmt.Sprintf("AASX-LIMIT-%s expanded %s size %d exceeds configured maximum %d", label, label, size, maximum))
	}
	return nil
}

func defaultAASXLimits() AASXLimits {
	return AASXLimits{
		MaxPartCount:              defaultAASXMaxPartCount,
		MaxOPCMetadataSizeBytes:   defaultAASXMaxOPCMetadataSizeBytes,
		MaxPartExpandedSizeBytes:  defaultAASXMaxPartExpandedSizeBytes,
		MaxTotalExpandedSizeBytes: defaultAASXMaxTotalExpandedSizeBytes,
		MaxThumbnailSizeBytes:     defaultAASXMaxThumbnailSizeBytes,
	}
}

func positiveUint64FromInt(value int, fallback uint64) uint64 {
	if value <= 0 {
		return fallback
	}
	return uint64(value)
}

func positiveUint64(value int64, fallback uint64) uint64 {
	if value <= 0 {
		return fallback
	}
	return uint64(value)
}
