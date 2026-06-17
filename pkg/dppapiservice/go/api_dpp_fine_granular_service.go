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
// Author: Jannik Fried ( Fraunhofer IESE ), Aaron Zielstorff ( Fraunhofer IESE )

package dppapi

import "context"

// DPPFineGranularAPIService delegates fine-grained DPP operations to a configured implementation.
//
// Fields:
//   - delegate: Optional service implementation used to execute fine-grained operations
type DPPFineGranularAPIService struct {
	delegate DPPFineGranularAPIServicer
}

// NewDPPFineGranularAPIService creates an unconfigured fine-grained API service.
//
// Returns:
//   - *DPPFineGranularAPIService: Fine-grained service that returns not-configured responses
func NewDPPFineGranularAPIService() *DPPFineGranularAPIService {
	return &DPPFineGranularAPIService{}
}

// NewDPPFineGranularAPIServiceWithDelegate creates a fine-grained API service using the supplied delegate.
//
// Parameters:
//   - delegate: Service implementation used to execute fine-grained DPP operations
//
// Returns:
//   - *DPPFineGranularAPIService: Fine-grained service delegating to the supplied implementation
func NewDPPFineGranularAPIServiceWithDelegate(delegate DPPFineGranularAPIServicer) *DPPFineGranularAPIService {
	return &DPPFineGranularAPIService{delegate: delegate}
}

// ReadDataElement delegates reading one DPP data element.
//
// Parameters:
//   - ctx: Request context used by the delegate
//   - dppID: Identifier of the DPP that owns the element
//   - elementPath: Content section and idShort path in <section>/<path> form
//   - representation: Requested compressed or full element representation
//
// Returns:
//   - ImplResponse: Delegate response or not-configured response
//   - error: Delegate error, if one is returned
func (s *DPPFineGranularAPIService) ReadDataElement(ctx context.Context, dppID string, elementPath string, representation Representation) (ImplResponse, error) {
	if s.delegate != nil {
		return s.delegate.ReadDataElement(ctx, dppID, elementPath, representation)
	}
	return serviceNotConfigured("ReadDataElement"), nil
}

// UpdateDataElement delegates updating one DPP data element.
//
// Parameters:
//   - ctx: Request context used by the delegate
//   - dppID: Identifier of the DPP that owns the element
//   - elementPath: Content section and idShort path in <section>/<path> form
//   - dataElement: Generated DPP data element model used as replacement content
//
// Returns:
//   - ImplResponse: Delegate response or not-configured response
//   - error: Delegate error, if one is returned
func (s *DPPFineGranularAPIService) UpdateDataElement(ctx context.Context, dppID string, elementPath string, dataElement DataElement) (ImplResponse, error) {
	if s.delegate != nil {
		return s.delegate.UpdateDataElement(ctx, dppID, elementPath, dataElement)
	}
	return serviceNotConfigured("UpdateDataElement"), nil
}
