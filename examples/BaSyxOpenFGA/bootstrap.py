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
import json
import pathlib
import time
import urllib.error
import urllib.request

ROOT = pathlib.Path(__file__).resolve().parent
BASE = "http://127.0.0.1:28089"
HEADERS = {"Authorization": "Bearer demo-openfga-key", "Content-Type": "application/json"}

def request(path, body=None):
    data = None if body is None else json.dumps(body).encode()
    req = urllib.request.Request(BASE + path, data=data, headers=HEADERS)
    with urllib.request.urlopen(req, timeout=10) as response:
        return json.load(response)

for attempt in range(90):
    try:
        stores = request("/stores").get("stores", [])
        break
    except (OSError, urllib.error.URLError):
        time.sleep(2)
else:
    raise SystemExit("OpenFGA did not become ready")

existing = ROOT / ".env"
if existing.exists():
    print("Existing OpenFGA IDs retained; remove .env only when resetting this disposable database.")
else:
    store = request("/stores", {"name": "basyx-openfga-demo"})["id"]
    model = request("/stores/" + store + "/authorization-models", json.loads((ROOT / "model.json").read_text()))["authorization_model_id"]
    existing.write_text("REBAC_STORE_ID=" + store + "\nREBAC_MODEL_ID=" + model + "\n")
    print("Pinned OpenFGA store and model IDs written to .env")

for attempt in range(90):
    try:
        with urllib.request.urlopen("http://127.0.0.1:28080/realms/basyx/.well-known/openid-configuration", timeout=5):
            break
    except (OSError, urllib.error.URLError):
        time.sleep(2)
else:
    raise SystemExit("Keycloak did not become ready")
