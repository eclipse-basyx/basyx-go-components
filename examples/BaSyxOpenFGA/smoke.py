#!/usr/bin/env python3
# Copyright (C) 2026 the Eclipse BaSyx Authors and Fraunhofer IESE
# 
# Permission is hereby granted, free of charge, to any person obtaining
# a copy of this software and associated documentation files (the
# "Software"), to deal in the Software without restriction, including
# without limitation the rights to use, copy, modify, merge, publish,
# distribute, sublicense, and/or sell copies of the Software, and to
# permit persons to whom the Software is furnished to do so, subject to
# the following conditions:
# 
# The above copyright notice and this permission notice shall be
# included in all copies or substantial portions of the Software.
# 
# THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND,
# EXPRESS OR IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF
# MERCHANTABILITY, FITNESS FOR A PARTICULAR PURPOSE AND
# NONINFRINGEMENT. IN NO EVENT SHALL THE AUTHORS OR COPYRIGHT HOLDERS BE
# LIABLE FOR ANY CLAIM, DAMAGES OR OTHER LIABILITY, WHETHER IN AN ACTION
# OF CONTRACT, TORT OR OTHERWISE, ARISING FROM, OUT OF OR IN CONNECTION
# WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE SOFTWARE.
# 
# SPDX-License-Identifier: MIT
import base64
import json
import pathlib
import subprocess
import time
import urllib.error
import urllib.parse
import urllib.request
import uuid

ROOT = pathlib.Path(__file__).resolve().parent
API = "http://127.0.0.1:28082"
UI = "http://127.0.0.1:28030"
ISSUER = "http://keycloak.localhost:28080/realms/basyx"
REBAC_PROFILE = "https://basyx.org/aas/API/3/2/ResourceBoundAccessControl/1.0"

def token(user):
    data = urllib.parse.urlencode({"grant_type": "password", "client_id": "basyx-demo", "username": user, "password": "demo-" + user, "scope": "openid email profile"}).encode()
    with urllib.request.urlopen("http://127.0.0.1:28080/realms/basyx/protocol/openid-connect/token", data=data, timeout=10) as response:
        return json.load(response)["access_token"]

def request(method, path, bearer, body=None, etag=None):
    headers = {"Authorization": "Bearer " + bearer, "Content-Type": "application/json"}
    if etag:
        headers["If-Match"] = etag
    data = None if body is None else json.dumps(body).encode()
    base_path = urllib.parse.urlsplit(API).path.rstrip("/")
    if base_path and path.startswith(base_path + "/"):
        path = path[len(base_path):]
    req = urllib.request.Request(API + path, method=method, headers=headers, data=data)
    try:
        with urllib.request.urlopen(req, timeout=20) as response:
            return response.status, response.headers, response.read()
    except urllib.error.HTTPError as error:
        return error.code, error.headers, error.read()

def wait_request(method, path, bearer, expected, body=None, etag=None):
    for attempt in range(60):
        result = request(method, path, bearer, body, etag)
        if result[0] in expected:
            return result
        if result[0] != 503:
            break
        time.sleep(1)
    raise AssertionError(f"{method} {path}: expected {expected}, got {result[0]} {result[2][:300]!r}")

def replace_grants(path, bearer, grants, etag):
    status, headers, data = wait_request("PUT", path + "/$access/grants", bearer, {200, 202, 204}, {"grants": grants}, etag)
    if status == 202:
        assert headers.get("Location"), "Pending change has no status URL"
        for attempt in range(60):
            _, _, snapshot_data = wait_request("GET", headers["Location"], bearer, {200})
            snapshot = json.loads(snapshot_data)
            if snapshot["appliedRevision"] == snapshot["revision"]:
                return
            time.sleep(1)
        raise AssertionError("Grant projection did not complete")

def main():
    owner, reader, fallback = (token(user) for user in ("owner", "reader", "fallback"))
    _, _, description_data = wait_request("GET", "/description", owner, {200})
    profiles = json.loads(description_data)["profiles"]
    assert profiles.count(REBAC_PROFILE) == 1, "ReBAC profile is not advertised exactly once"
    with urllib.request.urlopen(UI, timeout=10) as response:
        assert response.status == 200, "AAS UI is unavailable"
    identifier = "urn:basyx:openfga:demo:" + str(uuid.uuid4())
    resource = "/submodels/" + base64.urlsafe_b64encode(identifier.encode()).decode().rstrip("=")
    wait_request("POST", "/submodels", owner, {201}, {"modelType": "Submodel", "id": identifier, "idShort": "SharedDemo", "submodelElements": []})
    wait_request("GET", resource, reader, {403, 404})
    status, headers, data = wait_request("GET", resource + "/$access", owner, {200})
    grants = json.loads(data)["grants"]
    assert any(grant["role"] == "owner" for grant in grants), "Creator owner grant missing"
    reader_grant = {"principal": {"kind": "user", "issuer": ISSUER, "id": "22222222-2222-4222-8222-222222222222"}, "role": "viewer"}
    replace_grants(resource, owner, grants + [reader_grant], headers["ETag"])
    wait_request("GET", resource, reader, {200})
    status, headers, data = wait_request("GET", resource + "/$access", owner, {200})
    replace_grants(resource, owner, grants, headers["ETag"])
    wait_request("GET", resource, reader, {403, 404})
    wait_request("GET", resource, fallback, {200})
    try:
        subprocess.run(["docker", "compose", "stop", "openfga"], cwd=ROOT, check=True)
        assert request("GET", resource, fallback)[0] == 503, "OpenFGA outage incorrectly fell back to ABAC"
    finally:
        subprocess.run(["docker", "compose", "start", "openfga"], cwd=ROOT, check=True)
    wait_request("GET", resource, fallback, {200})
    print("Passed UI availability, capability advertisement, ownership, sharing, revocation, ABAC fallback and fail-closed outage checks.")


if __name__ == "__main__":
    main()
