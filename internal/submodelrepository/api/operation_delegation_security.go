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

package api

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

type delegationAuthoritySet map[string]struct{}

type delegationAddressGuard struct {
	trustedAuthorities delegationAuthoritySet
	resolveHost        func(context.Context, string) ([]netip.Addr, error)
	dialContext        func(context.Context, string, string) (net.Conn, error)
}

type trustedDelegationTarget struct {
	originalAuthority string
	resolvedAuthority string
}

func parseTrustedDelegationAuthorities() delegationAuthoritySet {
	rawHosts := strings.TrimSpace(os.Getenv(delegationTrustedHostsKey))
	if rawHosts == "" {
		return delegationAuthoritySet{}
	}

	trustedAuthorities := delegationAuthoritySet{}
	for _, rawAuthority := range strings.Split(rawHosts, ",") {
		authority, ok := normalizeTrustedDelegationAuthority(rawAuthority)
		if !ok {
			continue
		}
		trustedAuthorities[authority] = struct{}{}
	}

	return trustedAuthorities
}

func normalizeTrustedDelegationAuthority(rawAuthority string) (string, bool) {
	host, port, err := net.SplitHostPort(strings.TrimSpace(rawAuthority))
	if err != nil {
		return "", false
	}

	authority, err := normalizeDelegationAuthority(host, port)
	if err != nil {
		return "", false
	}

	return authority, true
}

func normalizeDelegationAuthority(host string, port string) (string, error) {
	normalizedHost, err := normalizeDelegationHost(host)
	if err != nil {
		return "", err
	}

	normalizedPort, err := normalizeDelegationPort(port)
	if err != nil {
		return "", err
	}

	return net.JoinHostPort(normalizedHost, normalizedPort), nil
}

func normalizeDelegationHost(host string) (string, error) {
	normalizedHost := strings.ToLower(strings.TrimSpace(host))
	if strings.HasPrefix(normalizedHost, "[") && strings.HasSuffix(normalizedHost, "]") {
		normalizedHost = strings.TrimPrefix(strings.TrimSuffix(normalizedHost, "]"), "[")
	}

	if normalizedHost == "" {
		return "", errors.New("SMREPO-NORMDELAUTH-MISSINGHOST delegation authority host is missing")
	}
	if strings.Contains(normalizedHost, "*") {
		return "", errors.New("SMREPO-NORMDELAUTH-HOSTWILDCARD delegation authority host wildcards are not supported")
	}
	if strings.Contains(normalizedHost, "/") {
		return "", errors.New("SMREPO-NORMDELAUTH-CIDR delegation authority CIDR ranges are not supported")
	}

	if ip, parseErr := netip.ParseAddr(normalizedHost); parseErr == nil {
		return ip.Unmap().String(), nil
	}

	return normalizedHost, nil
}

func normalizeDelegationPort(port string) (string, error) {
	normalizedPort := strings.TrimSpace(port)
	if normalizedPort == "" {
		return "", errors.New("SMREPO-NORMDELAUTH-MISSINGPORT delegation authority port is missing")
	}
	if normalizedPort == "*" {
		return normalizedPort, nil
	}

	portNumber, err := strconv.Atoi(normalizedPort)
	if err != nil || portNumber < 1 || portNumber > 65535 {
		return "", errors.New("SMREPO-NORMDELAUTH-INVALIDPORT delegation authority port must be 1-65535 or *")
	}

	return strconv.Itoa(portNumber), nil
}

func isTrustedDelegationAuthority(authority string) bool {
	normalizedAuthority, err := normalizeDelegationAuthorityFromAddress(authority)
	if err != nil {
		return false
	}

	return parseTrustedDelegationAuthorities().contains(normalizedAuthority)
}

func normalizeDelegationAuthorityFromAddress(address string) (string, error) {
	host, port, err := net.SplitHostPort(strings.TrimSpace(address))
	if err != nil {
		return "", err
	}

	return normalizeDelegationAuthority(host, port)
}

func delegationURLPort(parsedDelegationURL *url.URL) (string, error) {
	port := strings.TrimSpace(parsedDelegationURL.Port())
	if port != "" {
		return normalizeDelegationPort(port)
	}

	switch parsedDelegationURL.Scheme {
	case "http":
		return "80", nil
	case "https":
		return "443", nil
	default:
		return "", errors.New("SMREPO-DELURLPORT-UNSUPPORTEDSCHEME delegation URL must use http or https")
	}
}

func newDelegationAddressGuard(
	resolveHost func(context.Context, string) ([]netip.Addr, error),
	dialContext func(context.Context, string, string) (net.Conn, error),
) delegationAddressGuard {
	if resolveHost == nil {
		resolveHost = func(ctx context.Context, host string) ([]netip.Addr, error) {
			return net.DefaultResolver.LookupNetIP(ctx, "ip", host)
		}
	}

	if dialContext == nil {
		netDialer := &net.Dialer{}
		dialContext = netDialer.DialContext
	}

	return delegationAddressGuard{
		trustedAuthorities: parseTrustedDelegationAuthorities(),
		resolveHost:        resolveHost,
		dialContext:        dialContext,
	}
}

func (g delegationAddressGuard) isTrusted(authority string) bool {
	normalizedAuthority, err := normalizeDelegationAuthorityFromAddress(authority)
	if err != nil {
		return false
	}

	return g.trustedAuthorities.contains(normalizedAuthority)
}

func (trustedAuthorities delegationAuthoritySet) contains(normalizedAuthority string) bool {
	if _, trusted := trustedAuthorities[normalizedAuthority]; trusted {
		return true
	}

	host, _, err := net.SplitHostPort(normalizedAuthority)
	if err != nil {
		return false
	}

	wildcardPortAuthority, err := normalizeDelegationAuthority(host, "*")
	if err != nil {
		return false
	}

	_, trusted := trustedAuthorities[wildcardPortAuthority]
	return trusted
}

func (g delegationAddressGuard) resolveTrustedURLTarget(ctx context.Context, parsedDelegationURL *url.URL) (trustedDelegationTarget, error) {
	if parsedDelegationURL == nil {
		return trustedDelegationTarget{}, errors.New("SMREPO-RSLVDELAUTH-NILURL delegation URL is nil")
	}

	if parsedDelegationURL.Scheme != "http" && parsedDelegationURL.Scheme != "https" {
		return trustedDelegationTarget{}, errors.New("SMREPO-DOOPDELG-UNSUPPORTEDSCHEME delegation URL must use http or https")
	}
	if strings.TrimSpace(parsedDelegationURL.Host) == "" {
		return trustedDelegationTarget{}, errors.New("SMREPO-DOOPDELG-MISSINGHOST delegation URL host is missing")
	}

	port, err := delegationURLPort(parsedDelegationURL)
	if err != nil {
		return trustedDelegationTarget{}, err
	}

	return g.resolveTrustedDialTarget(ctx, parsedDelegationURL.Hostname(), port)
}

func (g delegationAddressGuard) resolveTrustedDialTarget(ctx context.Context, host string, port string) (trustedDelegationTarget, error) {
	trustedTargets, err := g.resolveTrustedDialTargets(ctx, host, port)
	if err != nil {
		return trustedDelegationTarget{}, err
	}

	return trustedTargets[0], nil
}

func (g delegationAddressGuard) resolveTrustedDialTargets(ctx context.Context, host string, port string) ([]trustedDelegationTarget, error) {
	originalAuthority, err := normalizeDelegationAuthority(host, port)
	if err != nil {
		return nil, err
	}
	if !g.isTrusted(originalAuthority) {
		return nil, fmt.Errorf("SMREPO-RSLVDELAUTH-UNTRUSTED delegation URL address %q is not in %s allowlist", originalAuthority, delegationTrustedHostsKey)
	}

	resolvedIPs, err := g.resolveHostToIPs(ctx, host)
	if err != nil {
		return nil, err
	}

	trustedTargets := make([]trustedDelegationTarget, 0, len(resolvedIPs))
	for _, resolvedIP := range resolvedIPs {
		resolvedAuthority, authorityErr := normalizeDelegationAuthority(resolvedIP.String(), port)
		if authorityErr != nil {
			continue
		}
		if g.isTrusted(resolvedAuthority) {
			trustedTargets = append(trustedTargets, trustedDelegationTarget{
				originalAuthority: originalAuthority,
				resolvedAuthority: resolvedAuthority,
			})
		}
	}

	if len(trustedTargets) == 0 {
		return nil, fmt.Errorf("SMREPO-RSLVDELAUTH-UNTRUSTEDRESOLVED delegation URL address %q resolved to addresses that are not in %s allowlist", originalAuthority, delegationTrustedHostsKey)
	}

	return trustedTargets, nil
}

func (g delegationAddressGuard) resolveHostToIPs(ctx context.Context, host string) ([]netip.Addr, error) {
	normalizedHost, err := normalizeDelegationHost(host)
	if err != nil {
		return nil, err
	}

	if ip, parseErr := netip.ParseAddr(normalizedHost); parseErr == nil {
		return []netip.Addr{ip}, nil
	}

	resolvedIPs, lookupErr := g.resolveHost(ctx, normalizedHost)
	if lookupErr != nil {
		return nil, fmt.Errorf("SMREPO-RSLVDELAUTH-LOOKUP %w", lookupErr)
	}
	if len(resolvedIPs) == 0 {
		return nil, fmt.Errorf("SMREPO-RSLVDELAUTH-NOIP delegation URL host %q did not resolve to an IP address", normalizedHost)
	}

	return resolvedIPs, nil
}

func (g delegationAddressGuard) dialTrustedContext(ctx context.Context, network string, address string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, fmt.Errorf("SMREPO-DELDIAL-SPLITADDR %w", err)
	}

	targets, err := g.resolveTrustedDialTargets(ctx, host, port)
	if err != nil {
		return nil, err
	}

	return g.dialTrustedTargets(ctx, network, targets)
}

func (g delegationAddressGuard) dialTrustedTargets(ctx context.Context, network string, targets []trustedDelegationTarget) (net.Conn, error) {
	var lastDialErr error
	for _, target := range targets {
		conn, err := g.dialContext(ctx, network, target.resolvedAuthority)
		if err == nil {
			return conn, nil
		}
		lastDialErr = err
	}

	if lastDialErr == nil {
		return nil, errors.New("SMREPO-DELDIAL-NOTARGET no trusted delegation dial target is available")
	}

	return nil, fmt.Errorf("SMREPO-DELDIAL-EXEC %w", lastDialErr)
}

func newDelegationHTTPClient(timeout time.Duration, guard delegationAddressGuard) *http.Client {
	return &http.Client{
		Timeout: timeout,
		Transport: &http.Transport{
			Proxy:       nil,
			DialContext: guard.dialTrustedContext,
		},
	}
}
