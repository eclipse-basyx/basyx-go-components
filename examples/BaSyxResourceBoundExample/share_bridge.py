# /*******************************************************************************
# * Copyright (C) 2026 the Eclipse BaSyx Authors and Fraunhofer IESE
# *
# * Permission is hereby granted, free of charge, to any person obtaining
# * a copy of this software and associated documentation files (the
# * "Software"), to deal in the Software without restriction, including
# * without limitation the rights to use, copy, modify, merge, publish,
# * distribute, sublicense, and/or sell copies of the Software, and to
# * permit persons to whom the Software is furnished to do so, subject to
# * the following conditions:
# *
# * The above copyright notice and this permission notice shall be
# * included in all copies or substantial portions of the Software.
# *
# * THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND,
# * EXPRESS OR IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF
# * MERCHANTABILITY, FITNESS FOR A PARTICULAR PURPOSE AND
# * NONINFRINGEMENT. IN NO EVENT SHALL THE AUTHORS OR COPYRIGHT HOLDERS BE
# * LIABLE FOR ANY CLAIM, DAMAGES OR OTHER LIABILITY, WHETHER IN AN ACTION
# * OF CONTRACT, TORT OR OTHERWISE, ARISING FROM, OUT OF OR IN CONNECTION
# * WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE SOFTWARE.
# *
# * SPDX-License-Identifier: MIT
# ******************************************************************************/
# 
import base64
import json
import os
import urllib.request
import uuid


def request(method, path, body=None, etag=None):
    headers = {"Authorization": "Bearer " + os.environ["OWNER_TOKEN"]}
    if etag:
        headers["If-Match"] = etag
    data = None
    if body is not None:
        data = json.dumps(body).encode()
        headers["Content-Type"] = "application/json"
    req = urllib.request.Request(base_url + path, data=data, headers=headers, method=method)
    with urllib.request.urlopen(req, timeout=30) as response:
        payload = response.read()
        return (json.loads(payload) if payload else None), response.headers.get("ETag")


def change(access, suffix, body, method="PUT"):
    _, etag = request("GET", access)
    return request(method, access + suffix, body, etag)


def encoded(identifier):
    return base64.urlsafe_b64encode(identifier.encode()).decode().rstrip("=")


base_url = os.environ.get("BASYX_URL", "http://localhost:8080").rstrip("/")
issuer = os.environ["ISSUER"]
owner = {"issuer": issuer, "subject": os.environ["OWNER_SUBJECT"]}
reader = {"issuer": issuer, "subject": os.environ["READER_SUBJECT"]}
manager = {"issuer": issuer, "subject": os.environ["MANAGER_SUBJECT"]}
identifier = "urn:example:bridge:" + str(uuid.uuid4())
submodel_id = identifier + ":inspection"
request("POST", "/submodels", {
    "modelType": "Submodel", "id": submodel_id, "idShort": "Inspection",
    "submodelElements": [
        {"modelType": "Property", "idShort": "Status", "valueType": "xs:string", "value": "Inspected"},
        {"modelType": "Property", "idShort": "InternalCost", "valueType": "xs:integer", "value": "1200"},
    ],
})
request("POST", "/shells", {
    "modelType": "AssetAdministrationShell", "id": identifier, "idShort": "Bridge",
    "assetInformation": {"assetKind": "Instance", "globalAssetId": identifier + ":asset"},
    "submodels": [{"type": "ModelReference", "keys": [{"type": "Submodel", "value": submodel_id}]}],
})
path = "/submodels/" + encoded(submodel_id)
access = path + "/$access"
change(access, "/policy", {"RESOURCE": {"IDENTIFIABLE": "$sm(" + json.dumps(submodel_id) + ")"}, "rules": []})
change(access, "/grants", {"principal": owner, "rights": ["ALL"]}, "POST")
change(access, "/grants", {"principal": reader, "rights": ["READ"]}, "POST")
change(access, "/managers", [manager])
private_access = path + "/submodel-elements/InternalCost/$access"
change(private_access, "/policy", {
    "RESOURCE": {"REFERABLE": "$sme(" + json.dumps(submodel_id) + ").InternalCost"}, "rules": [],
})
print("Bridge:", base_url + "/shells/" + encoded(identifier))
print("Access management:", base_url + access)
print("Reader and inherited manager can read Status; InternalCost has a local override.")
print("A matching ABAC auditor grant can still authorize InternalCost through fallback.")
