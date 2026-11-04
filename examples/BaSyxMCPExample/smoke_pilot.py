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
"""Live acceptance for a disposable pilot: large assets and recreation persistence.

Creates and removes uniquely named fixtures. Never run against a shared pilot.
"""

import argparse
import base64
import json
import subprocess
import urllib.parse
import urllib.request
import uuid

from pilot import DIRECTORY, PilotError, compose, prepare_runtime, wait_for_api


def encoded(value):
    return base64.urlsafe_b64encode(value.encode()).decode().rstrip("=")


def expect(api, method, path, payload=None, status=200):
    code, body, _ = api.request(method, path, payload)
    if code != status:
        raise PilotError(
            "PILOT-ACCEPTANCE-REST: " + method + " " + path + " returned " + str(code)
        )
    return body


def large_asset(api):
    suffix = uuid.uuid4().hex
    shell = "urn:pilot:large:" + suffix
    models = []
    try:
        for i in range(12):
            identifier = "urn:pilot:large-sm:" + suffix + ":" + str(i)
            model = {
                "modelType": "Submodel",
                "id": identifier,
                "idShort": "Large" + str(i),
                "submodelElements": [
                    {
                        "modelType": "Property",
                        "idShort": "Reading" + str(j),
                        "valueType": "xs:string",
                        "value": "sample-" + ("x" * 50000),
                    }
                    for j in range(4)
                ],
            }
            if i == 0:
                model["submodelElements"].append(
                    {
                        "modelType": "SubmodelElementCollection",
                        "idShort": "Telemetry",
                        "value": [
                            {
                                "modelType": "Property",
                                "idShort": "Temperature",
                                "valueType": "xs:double",
                                "value": "51",
                            }
                        ],
                    }
                )
            expect(api, "POST", "/submodels", model, 201)
            models.append(identifier)
        expect(
            api,
            "POST",
            "/shells",
            {
                "modelType": "AssetAdministrationShell",
                "id": shell,
                "idShort": "PilotLarge" + suffix,
                "assetInformation": {"assetKind": "Instance", "globalAssetId": suffix},
                "submodels": [
                    {
                        "type": "ModelReference",
                        "keys": [{"type": "Submodel", "value": identifier}],
                    }
                    for identifier in models
                ],
            },
            201,
        )
        found = api.call("find_asset", {"query": suffix, "limit": 1})
        assert len(found["result"]) == 1
        assert len(json.dumps(found)) < 4096
        measurement = api.call(
            "get_asset_measurement", {"shell": shell, "query": "Temperature"}
        )
        assert (
            measurement["data"] == 51 and measurement["path"] == "Telemetry.Temperature"
        )
        ambiguous = api.call(
            "get_asset_measurement", {"shell": shell, "query": "Reading"}
        )
        assert (
            ambiguous["status"] == "ambiguous" and ambiguous["total_candidates"] == 48
        )
        cursor = ""
        seen = set()
        actual = []
        while True:
            description = api.call(
                "describe_asset", {"shell": shell, "cursor": cursor, "limit": 3}
            )
            assert not description["omissions"], description["omissions"]
            assert len(json.dumps(description)) < 65536
            for model in description["submodels"]:
                actual.append(model["id"])
                assert any(
                    isinstance(value["value"], dict) and value["value"].get("truncated")
                    for value in model["values"]
                )
            cursor = description["paging_metadata"].get("cursor", "")
            if not cursor:
                break
            assert cursor not in seen
            seen.add(cursor)
        assert set(actual) == set(models) and len(actual) == len(models)
        print(
            "PASS: multi-megabyte asset is discoverable and described in bounded pages without lost submodels",
            flush=True,
        )
    finally:
        api.request("DELETE", "/shells/" + encoded(shell))
        for identifier in models:
            api.request("DELETE", "/submodels/" + encoded(identifier))


def identity_request(issuer, method, path, payload=None, token=None):
    headers = {"Content-Type": "application/json"}
    if token:
        headers["Authorization"] = "Bearer " + token
    data = None if payload is None else json.dumps(payload).encode()
    with urllib.request.urlopen(
        urllib.request.Request(
            issuer + path, data=data, method=method, headers=headers
        ),
        timeout=20,
    ) as response:
        raw = response.read()
        return json.loads(raw) if raw else None


def admin_token(issuer):
    data = urllib.parse.urlencode(
        {
            "grant_type": "password",
            "client_id": "admin-cli",
            "username": "admin",
            "password": "sandbox-admin",
        }
    ).encode()
    with urllib.request.urlopen(
        urllib.request.Request(
            issuer + "/realms/master/protocol/openid-connect/token", data=data
        ),
        timeout=20,
    ) as response:
        return json.load(response)["access_token"]


def recreation(args, api, environment):
    issuer = "http://localhost:" + str(args.oidc_port)
    token = admin_token(issuer)
    client_id = "pilot-persistence-" + uuid.uuid4().hex
    identity_request(
        issuer,
        "POST",
        "/admin/realms/basyx-mcp/clients",
        {
            "clientId": client_id,
            "publicClient": True,
            "protocol": "openid-connect",
            "standardFlowEnabled": True,
        },
        token,
    )
    identifier = "urn:pilot:persistence:" + uuid.uuid4().hex
    path = "/submodels/" + encoded(identifier)
    initial = {
        "modelType": "Submodel",
        "id": identifier,
        "idShort": "PersistenceSentinel",
        "submodelElements": [
            {
                "modelType": "Property",
                "idShort": "Reading",
                "valueType": "xs:integer",
                "value": "71",
            }
        ],
    }
    expect(api, "POST", "/submodels", initial, 201)
    try:
        expect(api, "PATCH", path + "/submodel-elements/Reading/$value", 72, 204)
        # Recreate the service containers, retaining only named volumes.
        compose(
            args,
            environment,
            "up",
            "-d",
            "--force-recreate",
            "keycloak",
            "aas-environment",
            "seed",
        )
        compose(args, environment, "wait", "seed")
        api = wait_for_api(args)
        value = api.call("get_element", {"submodel": identifier, "path": "Reading"})
        assert value["data"] == 72
        clients = identity_request(
            issuer,
            "GET",
            "/admin/realms/basyx-mcp/clients?clientId=" + client_id,
            token=admin_token(issuer),
        )
        assert len(clients) == 1
        print(
            "PASS: edited AAS data and runtime OAuth clients survive service-container recreation",
            flush=True,
        )
    finally:
        api.request("DELETE", path)
        clients = identity_request(
            issuer,
            "GET",
            "/admin/realms/basyx-mcp/clients?clientId=" + client_id,
            token=admin_token(issuer),
        )
        for client in clients:
            identity_request(
                issuer,
                "DELETE",
                "/admin/realms/basyx-mcp/clients/" + client["id"],
                token=admin_token(issuer),
            )


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--project", required=True)
    args = parser.parse_args()
    if args.project != "basyx-mcp-pilot-it" and not args.project.startswith(
        "basyx-mcp-pilot-test-"
    ):
        raise PilotError(
            "PILOT-ACCEPTANCE-PROJECT: use a disposable project named basyx-mcp-pilot-test-*"
        )
    state = json.loads((DIRECTORY / ".pilot" / args.project / "state.json").read_text())
    args.api_port = state["api_port"]
    args.oidc_port = state["oidc_port"]
    args.user = "alice"
    environment = prepare_runtime(
        DIRECTORY / ".pilot" / args.project,
        args.api_port,
        args.oidc_port,
        state["writes"],
    )
    # Verify ownership before recreating anything.
    for port in (args.api_port, args.oidc_port):
        result = subprocess.run(
            [
                "docker",
                "ps",
                "--filter",
                "publish=" + str(port),
                "--format",
                '{{.Label "com.docker.compose.project"}}',
            ],
            capture_output=True,
            text=True,
            check=True,
        )
        if set(result.stdout.splitlines()) != {args.project}:
            raise PilotError(
                "PILOT-ACCEPTANCE-OWNERSHIP: port is not owned by the disposable project"
            )
    api = wait_for_api(args)
    large_asset(api)
    recreation(args, api, environment)


if __name__ == "__main__":
    main()
