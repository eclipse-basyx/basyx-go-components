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
"""Start, inspect and connect the local read-first BaSyx MCP pilot."""

import argparse
import datetime
import json
import os
import re
import shutil
import subprocess
import sys
import tempfile
import time
import tomllib
import urllib.error
import urllib.parse
import urllib.request
from pathlib import Path

DIRECTORY = Path(__file__).resolve().parent


class PilotError(Exception):
    pass


class NoRedirect(urllib.request.HTTPRedirectHandler):
    def redirect_request(self, request, fp, code, message, headers, newurl):
        return None


class API:
    def __init__(self, base, issuer, user="alice"):
        self.base = base.rstrip("/")
        self.opener = urllib.request.build_opener(NoRedirect())
        form = urllib.parse.urlencode(
            {
                "grant_type": "password",
                "client_id": "basyx-mcp",
                "username": user,
                "password": "sandbox-" + user,
                "scope": "openid",
            }
        ).encode()
        request = urllib.request.Request(
            issuer + "/protocol/openid-connect/token", data=form
        )
        try:
            with self.opener.open(request, timeout=15) as response:
                self.token = json.load(response)["access_token"]
        except (urllib.error.URLError, KeyError, ValueError) as error:
            raise PilotError(
                "PILOT-AUTH-LOGIN: sandbox sign-in failed; check Keycloak and the selected sandbox account"
            ) from error

    def request(self, method, path, payload=None):
        data = None if payload is None else json.dumps(payload).encode()
        request = urllib.request.Request(
            self.base + path,
            data=data,
            method=method,
            headers={
                "Authorization": "Bearer " + self.token,
                "Content-Type": "application/json",
                "Accept": "application/json, text/event-stream",
                "MCP-Protocol-Version": "2025-11-25",
            },
        )
        try:
            response = self.opener.open(request, timeout=45)
        except urllib.error.HTTPError as error:
            response = error
        except urllib.error.URLError as error:
            raise PilotError(
                "PILOT-API-UNAVAILABLE: the AAS Environment is unreachable"
            ) from error
        with response:
            raw = response.read(2097153)
            if len(raw) > 2097152:
                raise PilotError(
                    "PILOT-API-BUDGET: response exceeds the setup client budget"
                )
            body = json.loads(raw) if raw else {}
            return response.status, body, dict(response.headers)

    def rpc(self, method, params=None):
        status, body, _ = self.request(
            "POST",
            "/mcp",
            {"jsonrpc": "2.0", "id": 1, "method": method, "params": params or {}},
        )
        if status == 401:
            raise PilotError("PILOT-MCP-AUTH: sign-in is required")
        if status != 200 or "error" in body:
            raise PilotError(
                "PILOT-MCP-REQUEST: "
                + method
                + " failed; check the server and selected permissions"
            )
        return body["result"]

    def call(self, name, arguments):
        result = self.rpc("tools/call", {"name": name, "arguments": arguments})
        if result.get("isError"):
            raise PilotError(
                "PILOT-MCP-TOOL: " + json.dumps(result.get("structuredContent", {}))
            )
        return result["structuredContent"]


def seed_environment(api, environment):
    result = {"created": 0, "existing": 0}
    for key, endpoint in [
        ("conceptDescriptions", "/concept-descriptions"),
        ("submodels", "/submodels"),
        ("assetAdministrationShells", "/shells"),
    ]:
        for item in environment.get(key, []):
            status, _, _ = api.request("POST", endpoint, item)
            if status == 201:
                result["created"] += 1
            elif status == 409:
                result["existing"] += 1
            else:
                raise PilotError(
                    "PILOT-SEED-CREATE: initialization failed with status "
                    + str(status)
                    + "; existing resources were not overwritten"
                )
    return result


def seed_once(api, environment, marker):
    marker = Path(marker)
    if marker.exists():
        return {"initialized": True, "skipped": True}
    result = seed_environment(api, environment)
    marker.parent.mkdir(parents=True, exist_ok=True)
    pending = marker.with_suffix(".pending")
    pending.write_text(json.dumps(result))
    os.replace(pending, marker)
    return result


def prepare_runtime(directory, api_port, oidc_port, writes):
    directory = Path(directory)
    directory.mkdir(parents=True, exist_ok=True)
    config = json.loads((DIRECTORY / "config.yaml").read_text())
    config["general"]["aasPreconfigPaths"] = []
    config["general"]["externalUrl"] = "http://localhost:" + str(api_port)
    config["mcp"]["publicUrl"] = "http://localhost:" + str(api_port) + "/mcp"
    config["mcp"]["authorizationServers"] = [
        "http://keycloak.localhost:" + str(oidc_port) + "/realms/basyx-mcp"
    ]
    config["mcp"]["writeMode"] = "confirm" if writes else "disabled"
    realm = json.loads((DIRECTORY / "keycloak/realm.json").read_text())
    for client in realm["clients"]:
        client["redirectUris"] = [
            uri.replace(":18584/", ":" + str(api_port) + "/")
            for uri in client.get("redirectUris", [])
        ]
        client["webOrigins"] = [
            uri.replace(":18584", ":" + str(api_port))
            for uri in client.get("webOrigins", [])
        ]
    trust = json.loads((DIRECTORY / "security/trustlist.json").read_text())
    for entry in trust:
        entry["issuer"] = config["mcp"]["authorizationServers"][0]
    files = {}
    for key, name, data in [
        ("BASYX_MCP_CONFIG_FILE", "config.json", config),
        ("BASYX_MCP_REALM_FILE", "realm.json", realm),
        ("BASYX_MCP_TRUSTLIST_FILE", "trustlist.json", trust),
    ]:
        path = (directory / name).resolve()
        path.write_text(json.dumps(data, indent=2) + "\n")
        files[key] = str(path)
    files.update(
        {"BASYX_MCP_API_PORT": str(api_port), "BASYX_MCP_OIDC_PORT": str(oidc_port)}
    )
    return files


def configure_codex(path, url, name, port):
    if not re.fullmatch(r"[a-zA-Z0-9_-]+", name):
        raise PilotError(
            "PILOT-CONFIG-NAME: use letters, digits, hyphens or underscores"
        )
    path = Path(path)
    original = path.read_text() if path.exists() else ""
    parsed = tomllib.loads(original)
    expected = url if "?" in url else url + "?profile=analyst"
    existing = parsed.get("mcp_servers", {}).get(name)
    if existing is not None:
        if existing.get("url") != expected:
            raise PilotError(
                "PILOT-CONFIG-CONFLICT: this connection name is already used; choose another --name"
            )
        return
    addition = (
        "\n[mcp_servers."
        + name
        + "]\nurl = "
        + json.dumps(expected)
        + '\nenabled = true\nstartup_timeout_sec = 30\nscopes = ["openid", "offline_access"]\n\n[mcp_servers.'
        + name
        + '.oauth]\nclient_id = "codex-basyx-mvp"\ncallback_url = "http://127.0.0.1:'
        + str(port)
        + '/callback"\ncallback_port = '
        + str(port)
        + "\n"
    )
    updated = original + addition
    tomllib.loads(updated)
    path.parent.mkdir(parents=True, exist_ok=True)
    if path.exists():
        backup = path.with_name(
            path.name
            + ".basyx-backup-"
            + datetime.datetime.now(datetime.UTC).strftime("%Y%m%dT%H%M%S%fZ")
        )
        shutil.copy2(path, backup)
        os.chmod(backup, 0o600)
    with tempfile.NamedTemporaryFile(
        mode="w", dir=path.parent, delete=False
    ) as pending:
        pending.write(updated)
        temporary = Path(pending.name)
    os.chmod(temporary, 0o600)
    os.replace(temporary, path)


def connection_status(names):
    return {
        "server": "ready",
        "authentication": "verified",
        "available_tools": len(names),
        "tools": names,
        "chat_attachment": "not_verified",
        "next_step": "Enable this connection in your MCP client and refresh its tool list. Server readiness does not prove attachment to this chat.",
    }


def compose(args, environment, *command):
    result = subprocess.run(
        [
            "docker",
            "compose",
            "-p",
            args.project,
            "-f",
            str(DIRECTORY / "docker-compose.yml"),
            *command,
        ],
        env=os.environ | environment,
        check=False,
    )
    if result.returncode:
        raise PilotError(
            "PILOT-COMPOSE-FAILED: Docker Compose failed; inspect the service logs"
        )


def wait_for_api(args):
    deadline = time.monotonic() + 120
    while time.monotonic() < deadline:
        try:
            return API(
                "http://localhost:" + str(args.api_port),
                "http://localhost:" + str(args.oidc_port) + "/realms/basyx-mcp",
                args.user,
            )
        except PilotError:
            time.sleep(2)
    raise PilotError(
        "PILOT-START-TIMEOUT: identity provider or AAS Environment is not ready; inspect Docker Compose logs"
    )


def status(args):
    api = API(
        "http://localhost:" + str(args.api_port),
        "http://localhost:" + str(args.oidc_port) + "/realms/basyx-mcp",
        args.user,
    )
    api.rpc(
        "initialize",
        {
            "protocolVersion": "2025-11-25",
            "clientInfo": {"name": "BaSyx pilot doctor", "version": "1"},
            "capabilities": {},
        },
    )
    names = [tool["name"] for tool in api.rpc("tools/list")["tools"]]
    result = connection_status(names)
    result["endpoint"] = "http://localhost:" + str(args.api_port) + "/mcp"
    return result


def check_existing_stack(args):
    for port in (args.api_port, args.oidc_port):
        current = subprocess.run(
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
        if any(name and name != args.project for name in current.stdout.splitlines()):
            raise PilotError(
                "PILOT-START-PORT: another stack owns this port; select --api-port and --oidc-port or the correct --project"
            )
    current = subprocess.run(
        [
            "docker",
            "ps",
            "-a",
            "--filter",
            "label=com.docker.compose.project=" + args.project,
            "--filter",
            "label=com.docker.compose.service=postgres",
            "--format",
            "{{.ID}}",
        ],
        capture_output=True,
        text=True,
        check=True,
    )
    for container in current.stdout.splitlines():
        inspected = subprocess.run(
            ["docker", "inspect", "--format", "{{json .Mounts}}", container],
            capture_output=True,
            text=True,
            check=True,
        )
        volumes = json.loads(inspected.stdout)
        if not any(
            mount.get("Name") == args.project + "_postgres_data"
            and mount.get("Destination") == "/var/lib/postgresql"
            for mount in volumes
        ):
            raise PilotError(
                "PILOT-START-LEGACY: this project uses a legacy database volume. Keep it intact and choose a new --project and ports; export and import its data explicitly before migration."
            )


def start(args, environment):
    check_existing_stack(args)
    compose(args, environment, "up", "--build", "-d")
    compose(args, environment, "wait", "seed")
    wait_for_api(args)
    return status(args)


def connect(args):
    if shutil.which("codex") is None:
        raise PilotError(
            "PILOT-CONNECT-CLIENT: install Codex before running connect; other MCP clients can use the endpoint printed by status"
        )
    ready = status(args)
    if not ready["available_tools"]:
        raise PilotError("PILOT-CONNECT-TOOLS: the server returned no tools")
    config = (
        Path(os.environ.get("CODEX_HOME", str(Path.home() / ".codex"))) / "config.toml"
    )
    configure_codex(config, ready["endpoint"], args.name, 18586)
    result = subprocess.run(["codex", "mcp", "login", args.name], check=False)
    if result.returncode:
        raise PilotError(
            "PILOT-CONNECT-LOGIN: sign-in did not complete; run connect again"
        )
    return {
        "connection": args.name,
        "sign_in": "complete",
        "chat_attachment": "not_verified",
        "next_step": "Refresh the MCP connection in your client or reopen the chat, then ask for motor M-4711 temperature.",
    }


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument(
        "action", choices=["start", "status", "connect", "stop", "seed"]
    )
    parser.add_argument("--project", default="basyx-mcp-pilot")
    parser.add_argument("--api-port", type=int)
    parser.add_argument("--oidc-port", type=int)
    writes = parser.add_mutually_exclusive_group()
    writes.add_argument("--enable-writes", dest="writes", action="store_true")
    writes.add_argument("--disable-writes", dest="writes", action="store_false")
    parser.set_defaults(writes=None)
    parser.add_argument(
        "--user", choices=["alice", "bob", "carol", "eve"], default="alice"
    )
    parser.add_argument("--name", default="basyx-pilot")
    parser.add_argument("--marker", default="/state/initialized.json")
    args = parser.parse_args()
    if not re.fullmatch(r"[a-z0-9][a-z0-9_-]{0,62}", args.project):
        raise PilotError("PILOT-CONFIG-PROJECT: invalid project name")
    runtime = DIRECTORY / ".pilot" / args.project
    state_path = runtime / "state.json"
    state = json.loads(state_path.read_text()) if state_path.exists() else {}
    args.api_port = args.api_port or state.get("api_port", 18584)
    args.oidc_port = args.oidc_port or state.get("oidc_port", 18580)
    if any(port < 1024 or port > 65535 for port in [args.api_port, args.oidc_port]):
        raise PilotError("PILOT-CONFIG-PORT: select ports between 1024 and 65535")
    if args.action == "seed":
        api = API(
            os.environ.get("MCP_SEED_API_URL", "http://aas-environment:5004"),
            os.environ.get(
                "MCP_SEED_ISSUER_URL", "http://keycloak:8080/realms/basyx-mcp"
            ),
        )
        result = seed_once(
            api,
            json.loads((DIRECTORY / "seed/environment.json").read_text()),
            args.marker,
        )
    elif args.action == "connect":
        result = connect(args)
    elif args.action == "status":
        result = status(args)
    else:
        writes = args.writes if args.writes is not None else state.get("writes", False)
        environment = prepare_runtime(
            runtime,
            args.api_port,
            args.oidc_port,
            writes,
        )
        state_path.write_text(
            json.dumps(
                {
                    "api_port": args.api_port,
                    "oidc_port": args.oidc_port,
                    "writes": writes,
                }
            )
        )
        if args.action == "start":
            result = start(args, environment)
        else:
            compose(args, environment, "stop")
            result = {"server": "stopped", "data": "preserved"}
    print(json.dumps(result, indent=2))


if __name__ == "__main__":
    try:
        main()
    except (PilotError, subprocess.SubprocessError, OSError, ValueError) as error:
        print(str(error), file=sys.stderr)
        sys.exit(1)
