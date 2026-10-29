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
"""Run the seven MVP demos against the disposable BaSyxMCPExample sandbox."""

import http.client
import http.cookiejar
import json
import os
from pathlib import Path
import socket
import subprocess
import tempfile
import urllib.error
import urllib.parse
import urllib.request
import uuid
from html.parser import HTMLParser

BASE = "http://localhost:18584"
ISSUER = "http://localhost:18580/realms/basyx-mcp"
DIRECTORY = Path(__file__).resolve().parent


def token(user):
    body = urllib.parse.urlencode({"grant_type": "password", "client_id": "basyx-mcp", "username": user, "password": "sandbox-" + user, "scope": "openid"}).encode()
    with urllib.request.urlopen(urllib.request.Request(ISSUER + "/protocol/openid-connect/token", data=body), timeout=15) as response:
        return json.load(response)["access_token"]


class Inspector:
    def __init__(self, user, profile, catalogue):
        self.token = token(user)
        self.url = BASE + "/mcp?profile=" + profile
        executable = os.environ.get("MCP_INSPECTOR_BIN")
        self.command = [executable] if executable else ["npx", "--yes", "@modelcontextprotocol/inspector@2.12.0"]
        self.environment = os.environ | {"MCP_CATALOG_PATH": catalogue}

    def request(self, method, options=()):
        command = self.command + ["--cli", self.url, "--transport", "http", "--method", method] + list(options) + ["--header", "Authorization: Bearer " + self.token]
        result = subprocess.run(command, capture_output=True, text=True, timeout=60, env=self.environment, check=False)
        if result.returncode:
            raise RuntimeError((result.stderr or result.stdout).replace(self.token, "[redacted]"))
        return json.loads(result.stdout)

    def call(self, name, arguments=None):
        options = ["--tool-name", name]
        if arguments:
            options += ["--tool-arg"] + [key + "=" + (value if isinstance(value, str) else json.dumps(value, separators=(",", ":"))) for key, value in arguments.items()]
        result = self.request("tools/call", options)
        if result.get("isError"):
            raise RuntimeError(json.dumps(result.get("structuredContent", result)))
        return result.get("structuredContent") or json.loads(result["content"][0]["text"])


class Page(HTMLParser):
    def __init__(self, text):
        super().__init__()
        self.action, self.inputs, self.pre, self.in_pre = "", {}, [], False
        self.feed(text)

    def handle_starttag(self, tag, attrs):
        attrs = dict(attrs)
        if tag == "form":
            self.action = attrs.get("action", "")
        if tag == "input" and "name" in attrs:
            self.inputs[attrs["name"]] = attrs.get("value", "")
        if tag == "pre":
            self.in_pre = True
            self.pre.append("")

    def handle_endtag(self, tag):
        if tag == "pre":
            self.in_pre = False

    def handle_data(self, text):
        if self.in_pre:
            self.pre[-1] += text


class LocalConnection(http.client.HTTPConnection):
    def connect(self):
        if self.host == "keycloak.localhost":
            self.sock = socket.create_connection(("localhost", self.port), self.timeout)
        else:
            super().connect()


class LocalHTTP(urllib.request.HTTPHandler):
    def http_open(self, request):
        return self.do_open(LocalConnection, request)


class SandboxCookies(http.cookiejar.DefaultCookiePolicy):
    def return_ok_secure(self, cookie, request):
        # Browsers permit Secure cookies on localhost; mirror that only for this fixed test realm.
        if cookie.domain == "keycloak.localhost" and urllib.parse.urlsplit(request.full_url).hostname == "keycloak.localhost":
            return True
        return super().return_ok_secure(cookie, request)


def approve_for_test(uri):
    """Simulate the human approval UI only for the fixed disposable test account."""
    if not uri.startswith(BASE + "/mcp/confirm/"):
        raise ValueError("Approval URL is outside the disposable sandbox")
    client = urllib.request.build_opener(urllib.request.HTTPCookieProcessor(http.cookiejar.CookieJar(policy=SandboxCookies())), LocalHTTP())
    with client.open(uri, timeout=15) as response:
        page = Page(response.read().decode())
    if urllib.parse.urlsplit(page.action).netloc != "keycloak.localhost:18580":
        raise ValueError("Unexpected sandbox identity provider")
    form = page.inputs | {"username": "alice", "password": "sandbox-alice", "credentialId": ""}
    with client.open(urllib.request.Request(page.action, data=urllib.parse.urlencode(form).encode()), timeout=15) as response:
        page = Page(response.read().decode())
    if "csrf" not in page.inputs:
        raise RuntimeError("OAuth did not reach the approval form")
    form = page.inputs | {"approve": "yes"}
    request = urllib.request.Request(uri, data=urllib.parse.urlencode(form).encode(), headers={"Origin": BASE})
    with client.open(request, timeout=15) as response:
        page = Page(response.read().decode())
    if not page.pre or len(page.pre[-1]) != 64:
        raise RuntimeError("Approval did not return a single-use token")
    return page.pre[-1]


def creation_demo(client):
    client.call("list_submodel_templates")
    client.call("get_submodel_template", {"slug": "demo-technical-data"})
    draft = json.loads((DIRECTORY / "draft.json").read_text())
    suffix = uuid.uuid4().hex
    draft["shell"]["id"] += ":" + suffix
    old_id = draft["submodels"][0]["id"]
    new_id = old_id + ":" + suffix
    draft["submodels"][0]["id"] = new_id
    draft["templates"] = {new_id: draft["templates"][old_id]}
    assert client.call("validate_aas_draft", {"draft": draft})["valid"]
    plan = client.call("plan_create_aas", {"draft": draft})
    confirmation = approve_for_test(plan["confirmation_url"])
    assert client.call("apply_plan", {"plan_id": plan["plan_id"], "confirm_token": confirmation})["status"] == "applied"
    changeset = client.call("get_changeset", {"id": plan["plan_id"]})
    confirmation = approve_for_test(changeset["confirmation_url"])
    assert client.call("rollback_changeset", {"id": plan["plan_id"], "confirm_token": confirmation})["status"] == "rolled_back"


def main():
    with tempfile.TemporaryDirectory(prefix="basyx-mcp-inspector-") as directory:
        catalogue = str(Path(directory) / "catalogue.json")
        alice = Inspector("alice", "analyst", catalogue)
        assert alice.request("tools/list")["tools"]
        assert alice.call("find_asset", {"query": "M-4711"})["result"]
        assert alice.call("get_element", {"submodel": "urn:example:sm:technical-public", "path": "Temperature"})["data"] == 48
        print("PASS 1: find motor temperature")
        assert alice.call("describe_asset", {"shell": "urn:example:aas:motor-public"})["submodels"]
        alice.request("prompts/get", ["--prompt-name", "ask_asset", "--prompt-args", "shell=urn:example:aas:motor-public", "question=Describe the motor"])
        print("PASS 2: describe and ask asset")
        for uri in ("aasql://grammar", "aasql://fields"):
            alice.request("resources/read", ["--uri", uri])
        query = {"$condition": {"$match": [{"$eq": [{"$field": "$sm#idShort"}, {"$strVal": "TechnicalData"}]}, {"$gt": [{"$numCast": {"$field": "$sme.RatedPower#value"}}, {"$numVal": 10}]}]}}
        assert alice.call("query_aasql", {"target": "shells", "aasql": query, "dry_run": True})["valid"]
        assert len(alice.call("query_aasql", {"target": "shells", "aasql": query})["result"]) == 2
        print("PASS 3: validated AASQL selection")
        alice.call("list_read_models")
        arguments = {"name": "value_statistics_by_semantic_id", "params": {"semanticId": "urn:example:rated-power"}}
        assert float(alice.call("run_read_model", arguments)["result"][0]["avg"]) == 18
        print("PASS 4: authorized aggregation")
        bob = Inspector("bob", "analyst", catalogue)
        assert len(bob.call("query_aasql", {"target": "shells", "aasql": query})["result"]) == 1
        assert float(bob.call("run_read_model", arguments)["result"][0]["avg"]) == 12
        print("PASS 5: restricted-user parity")
        creation_demo(Inspector("alice", "operator", catalogue))
        print("PASS 6: human approval, creation, verification and rollback")
        assert alice.call("search_docs", {"query": "OTEL tracing"})["result"]
        alice.request("resources/read", ["--uri", "basyx-docs://admin/telemetry.md"])
        print("PASS 7: embedded version-matched documentation")


if __name__ == "__main__":
    main()
