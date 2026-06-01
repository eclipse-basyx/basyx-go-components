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

// Package main starts the Ory Hydra consent bridge for the BaSyx demo.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

const (
	maximumResponseBytes = 1024 * 1024
	requestTimeout       = 10 * time.Second
	authorizedClientID   = "basyx-ui"
	authorizedAudience   = "basyx-api"
)

type consentBridge struct {
	hydraAdminURL  string
	kratosAdminURL string
	httpClient     *http.Client
}

type consentRequest struct {
	RequestedScope               []string `json:"requested_scope"`
	RequestedAccessTokenAudience []string `json:"requested_access_token_audience"`
	Subject                      string   `json:"subject"`
	Client                       struct {
		ClientID string `json:"client_id"`
	} `json:"client"`
}

type kratosIdentity struct {
	Traits struct {
		Email string `json:"email"`
		Role  string `json:"role"`
	} `json:"traits"`
}

type acceptConsentRequest struct {
	GrantScope               []string             `json:"grant_scope"`
	GrantAccessTokenAudience []string             `json:"grant_access_token_audience"`
	Remember                 bool                 `json:"remember"`
	RememberFor              int                  `json:"remember_for"`
	Session                  acceptConsentSession `json:"session"`
}

type acceptConsentSession struct {
	AccessToken map[string]string `json:"access_token"`
	IDToken     map[string]string `json:"id_token"`
}

type redirectResponse struct {
	RedirectTo string `json:"redirect_to"`
}

func main() {
	bridge := consentBridge{
		hydraAdminURL:  requiredEnvironment("HYDRA_ADMIN_URL"),
		kratosAdminURL: requiredEnvironment("KRATOS_ADMIN_URL"),
		httpClient:     &http.Client{Timeout: requestTimeout},
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/health", health)
	mux.HandleFunc("/consent", bridge.consent)
	mux.HandleFunc("/error", oauthError)

	server := http.Server{
		Addr:              environmentOrDefault("SERVER_ADDRESS", ":3001"),
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       30 * time.Second,
	}

	log.Printf("ORY-DEMO-START-LISTEN: consent bridge listening on %s", server.Addr)
	if err := server.ListenAndServe(); err != nil {
		log.Fatalf("ORY-DEMO-START-SERVE: %v", err)
	}
}

func requiredEnvironment(name string) string {
	value := os.Getenv(name)
	if value == "" {
		log.Fatalf("ORY-DEMO-CONFIG-MISSINGENV: %s must be configured", name)
	}
	return strings.TrimRight(value, "/")
}

func environmentOrDefault(name string, fallback string) string {
	value := os.Getenv(name)
	if value == "" {
		return fallback
	}
	return value
}

func health(response http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodGet {
		http.Error(response, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	response.WriteHeader(http.StatusNoContent)
}

func oauthError(response http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodGet {
		http.Error(response, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	http.Error(response, "OAuth request failed", http.StatusBadRequest)
}

func (bridge consentBridge) consent(response http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodGet {
		http.Error(response, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	challenge := request.URL.Query().Get("consent_challenge")
	if challenge == "" {
		http.Error(response, "missing consent challenge", http.StatusBadRequest)
		return
	}

	redirectTo, err := bridge.acceptConsent(request.Context(), challenge)
	if err != nil {
		log.Printf("ORY-DEMO-CONSENT-ACCEPT: %v", err)
		http.Error(response, "failed to accept consent request", http.StatusBadGateway)
		return
	}

	http.Redirect(response, request, redirectTo, http.StatusFound)
}

func (bridge consentBridge) acceptConsent(context context.Context, challenge string) (string, error) {
	var consent consentRequest
	if err := bridge.getJSON(context, bridge.hydraConsentURL(challenge), &consent); err != nil {
		return "", fmt.Errorf("ORY-DEMO-CONSENT-GETREQUEST: %w", err)
	}
	if err := validateConsentRequest(consent); err != nil {
		return "", err
	}

	identity, err := bridge.identity(context, consent.Subject)
	if err != nil {
		return "", err
	}

	accept := acceptConsentRequest{
		GrantScope:               consent.RequestedScope,
		GrantAccessTokenAudience: consent.RequestedAccessTokenAudience,
		Remember:                 true,
		RememberFor:              3600,
		Session: acceptConsentSession{
			AccessToken: map[string]string{"role": identity.Traits.Role},
			IDToken:     map[string]string{"email": identity.Traits.Email, "role": identity.Traits.Role},
		},
	}

	var redirect redirectResponse
	if err := bridge.putJSON(context, bridge.hydraAcceptConsentURL(challenge), accept, &redirect); err != nil {
		return "", fmt.Errorf("ORY-DEMO-CONSENT-PUTACCEPT: %w", err)
	}
	if redirect.RedirectTo == "" {
		return "", fmt.Errorf("ORY-DEMO-CONSENT-MISSINGREDIRECT: Hydra returned no redirect URL")
	}
	return redirect.RedirectTo, nil
}

func validateConsentRequest(consent consentRequest) error {
	if consent.Client.ClientID != authorizedClientID {
		return fmt.Errorf("ORY-DEMO-CONSENT-INVALIDCLIENT: client %q is not allowed", consent.Client.ClientID)
	}
	if len(consent.RequestedAccessTokenAudience) != 1 || consent.RequestedAccessTokenAudience[0] != authorizedAudience {
		return fmt.Errorf(
			"ORY-DEMO-CONSENT-INVALIDAUDIENCE: expected only %q but got %q",
			authorizedAudience,
			consent.RequestedAccessTokenAudience,
		)
	}
	return nil
}

func (bridge consentBridge) identity(context context.Context, identityID string) (kratosIdentity, error) {
	if identityID == "" {
		return kratosIdentity{}, fmt.Errorf("ORY-DEMO-CONSENT-MISSINGSUBJECT: Hydra returned no subject")
	}

	var identity kratosIdentity
	identityURL := bridge.kratosAdminURL + "/admin/identities/" + url.PathEscape(identityID)
	if err := bridge.getJSON(context, identityURL, &identity); err != nil {
		return kratosIdentity{}, fmt.Errorf("ORY-DEMO-CONSENT-GETIDENTITY: %w", err)
	}
	if identity.Traits.Role != "admin" && identity.Traits.Role != "viewer" {
		return kratosIdentity{}, fmt.Errorf("ORY-DEMO-CONSENT-INVALIDROLE: identity has unsupported role %q", identity.Traits.Role)
	}
	return identity, nil
}

func (bridge consentBridge) hydraConsentURL(challenge string) string {
	return bridge.hydraAdminURL + "/admin/oauth2/auth/requests/consent?consent_challenge=" + url.QueryEscape(challenge)
}

func (bridge consentBridge) hydraAcceptConsentURL(challenge string) string {
	return bridge.hydraAdminURL + "/admin/oauth2/auth/requests/consent/accept?consent_challenge=" + url.QueryEscape(challenge)
}

func (bridge consentBridge) getJSON(context context.Context, requestURL string, destination any) error {
	return bridge.requestJSON(context, http.MethodGet, requestURL, nil, destination)
}

func (bridge consentBridge) putJSON(context context.Context, requestURL string, source any, destination any) error {
	return bridge.requestJSON(context, http.MethodPut, requestURL, source, destination)
}

func (bridge consentBridge) requestJSON(
	context context.Context,
	method string,
	requestURL string,
	source any,
	destination any,
) error {
	requestContext, cancel := contextWithTimeout(context)
	defer cancel()

	body, err := encodeRequest(source)
	if err != nil {
		return err
	}

	request, err := http.NewRequestWithContext(requestContext, method, requestURL, body)
	if err != nil {
		return fmt.Errorf("ORY-DEMO-HTTP-CREATEREQUEST: %w", err)
	}
	if source != nil {
		request.Header.Set("Content-Type", "application/json")
	}

	response, err := bridge.httpClient.Do(request)
	if err != nil {
		return fmt.Errorf("ORY-DEMO-HTTP-EXECUTE: %w", err)
	}
	defer response.Body.Close()

	responseBody, err := io.ReadAll(io.LimitReader(response.Body, maximumResponseBytes))
	if err != nil {
		return fmt.Errorf("ORY-DEMO-HTTP-READRESPONSE: %w", err)
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("ORY-DEMO-HTTP-STATUS: received HTTP %d: %s", response.StatusCode, responseBody)
	}
	if err := json.Unmarshal(responseBody, destination); err != nil {
		return fmt.Errorf("ORY-DEMO-HTTP-DECODE: %w", err)
	}
	return nil
}

func contextWithTimeout(parent context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(parent, requestTimeout)
}

func encodeRequest(source any) (io.Reader, error) {
	if source == nil {
		return nil, nil
	}
	body, err := json.Marshal(source)
	if err != nil {
		return nil, fmt.Errorf("ORY-DEMO-HTTP-ENCODE: %w", err)
	}
	return bytes.NewReader(body), nil
}
