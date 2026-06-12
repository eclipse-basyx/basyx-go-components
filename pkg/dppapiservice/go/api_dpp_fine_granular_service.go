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

//nolint:revive
package dppapi

import "context"

type DPPFineGranularAPIService struct {
	delegate DPPFineGranularAPIServicer
}

func NewDPPFineGranularAPIService() *DPPFineGranularAPIService {
	return &DPPFineGranularAPIService{}
}

func NewDPPFineGranularAPIServiceWithDelegate(delegate DPPFineGranularAPIServicer) *DPPFineGranularAPIService {
	return &DPPFineGranularAPIService{delegate: delegate}
}

func (s *DPPFineGranularAPIService) ReadDataElement(ctx context.Context, dppID string, elementPath string, representation Representation) (ImplResponse, error) {
	if s.delegate != nil {
		return s.delegate.ReadDataElement(ctx, dppID, elementPath, representation)
	}
	return serviceNotConfigured("ReadDataElement"), nil
}

func (s *DPPFineGranularAPIService) UpdateDataElement(ctx context.Context, dppID string, elementPath string, dataElement DataElement) (ImplResponse, error) {
	if s.delegate != nil {
		return s.delegate.UpdateDataElement(ctx, dppID, elementPath, dataElement)
	}
	return serviceNotConfigured("UpdateDataElement"), nil
}
