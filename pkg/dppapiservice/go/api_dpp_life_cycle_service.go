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

import (
	"context"
	"fmt"
	"net/http"
	"time"
)

type DPPLifeCycleAPIService struct {
	delegate DPPLifeCycleAPIServicer
}

func NewDPPLifeCycleAPIService() *DPPLifeCycleAPIService {
	return &DPPLifeCycleAPIService{}
}

func NewDPPLifeCycleAPIServiceWithDelegate(delegate DPPLifeCycleAPIServicer) *DPPLifeCycleAPIService {
	return &DPPLifeCycleAPIService{delegate: delegate}
}

func (s *DPPLifeCycleAPIService) ReadDPPById(ctx context.Context, dppID string, representation Representation) (ImplResponse, error) {
	if s.delegate != nil {
		return s.delegate.ReadDPPById(ctx, dppID, representation)
	}
	return serviceNotConfigured("ReadDPPById"), nil
}

func (s *DPPLifeCycleAPIService) DeleteDPPById(ctx context.Context, dppID string) (ImplResponse, error) {
	if s.delegate != nil {
		return s.delegate.DeleteDPPById(ctx, dppID)
	}
	return serviceNotConfigured("DeleteDPPById"), nil
}

func (s *DPPLifeCycleAPIService) UpdateDPPById(ctx context.Context, dppID string, patch DigitalProductPassportPatch) (ImplResponse, error) {
	if s.delegate != nil {
		return s.delegate.UpdateDPPById(ctx, dppID, patch)
	}
	return serviceNotConfigured("UpdateDPPById"), nil
}

func (s *DPPLifeCycleAPIService) CreateDPP(ctx context.Context, passport DigitalProductPassport) (ImplResponse, error) {
	if s.delegate != nil {
		return s.delegate.CreateDPP(ctx, passport)
	}
	return serviceNotConfigured("CreateDPP"), nil
}

func (s *DPPLifeCycleAPIService) ReadDPPByProductId(ctx context.Context, productID string, representation Representation) (ImplResponse, error) {
	if s.delegate != nil {
		return s.delegate.ReadDPPByProductId(ctx, productID, representation)
	}
	return serviceNotConfigured("ReadDPPByProductId"), nil
}

func (s *DPPLifeCycleAPIService) ReadDPPVersionByIdAndDate(ctx context.Context, dppID string, date time.Time, representation Representation) (ImplResponse, error) {
	if s.delegate != nil {
		return s.delegate.ReadDPPVersionByIdAndDate(ctx, dppID, date, representation)
	}
	return serviceNotConfigured("ReadDPPVersionByIdAndDate"), nil
}

func (s *DPPLifeCycleAPIService) ReadDPPIdsByProductIds(ctx context.Context, request ReadDppIdsByProductIdsRequest, limit int32, cursor string) (ImplResponse, error) {
	if s.delegate != nil {
		return s.delegate.ReadDPPIdsByProductIds(ctx, request, limit, cursor)
	}
	return serviceNotConfigured("ReadDPPIdsByProductIds"), nil
}

func serviceNotConfigured(operation string) ImplResponse {
	return errorResponse(http.StatusNotImplemented, fmt.Errorf("DPP-SERVICE-NOTCONFIGURED %s service is not configured", operation))
}
