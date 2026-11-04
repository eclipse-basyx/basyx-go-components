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
"""Offline contracts for pilot setup and non-destructive initialization."""

import contextlib
import io
import json
import tempfile
import tomllib
import unittest
from pathlib import Path
from types import SimpleNamespace
from unittest.mock import patch

import pilot


class FakeAPI:
    def __init__(self, status=409):
        self.status = status
        self.calls = []

    def request(self, method, path, payload=None):
        self.calls.append((method, path, payload))
        return self.status, {}, {}


class PilotTests(unittest.TestCase):
    def test_seed_never_overwrites_existing_resources(self):
        api = FakeAPI()
        environment = {
            "assetAdministrationShells": [{"id": "urn:motor"}],
            "submodels": [{"id": "urn:technical"}],
        }
        result = pilot.seed_environment(api, environment)
        self.assertEqual({"created": 0, "existing": 2}, result)
        self.assertEqual(["/submodels", "/shells"], [call[1] for call in api.calls])
        self.assertTrue(all(call[0] == "POST" for call in api.calls))

    def test_seed_does_not_treat_authorization_failure_as_existing(self):
        with self.assertRaisesRegex(pilot.PilotError, "SEED"):
            pilot.seed_environment(
                FakeAPI(403), {"submodels": [{"id": "urn:technical"}]}
            )

    def test_seed_marker_preserves_user_edits_and_deletions(self):
        with tempfile.TemporaryDirectory() as directory:
            marker = Path(directory) / "initialized.json"
            api = FakeAPI(201)
            pilot.seed_once(api, {"submodels": [{"id": "urn:technical"}]}, marker)
            self.assertTrue(marker.exists())
            api.calls.clear()
            pilot.seed_once(api, {"submodels": [{"id": "urn:technical"}]}, marker)
            self.assertEqual([], api.calls)

    def test_codex_configuration_preserves_other_settings_and_is_repeatable(self):
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / "config.toml"
            original = '# custom settings\nmodel = "my-model"\n[mcp_servers.other]\nurl = "https://example.org/mcp"\n'
            path.write_text(original)
            pilot.configure_codex(
                path, "http://localhost:18584/mcp", "basyx-pilot", 18586
            )
            first = path.read_text()
            pilot.configure_codex(
                path, "http://localhost:18584/mcp", "basyx-pilot", 18586
            )
            self.assertEqual(first, path.read_text())
            self.assertTrue(first.startswith(original))
            data = tomllib.loads(first)
            self.assertEqual(
                18586, data["mcp_servers"]["basyx-pilot"]["oauth"]["callback_port"]
            )
            self.assertEqual("my-model", data["model"])

    def test_codex_configuration_refuses_an_unrelated_existing_connection(self):
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / "config.toml"
            text = '[mcp_servers.basyx-pilot]\nurl = "https://another.example/mcp"\n'
            path.write_text(text)
            with self.assertRaisesRegex(pilot.PilotError, "CONFIG"):
                pilot.configure_codex(
                    path, "http://localhost:18584/mcp", "basyx-pilot", 18586
                )
            self.assertEqual(text, path.read_text())

    def test_runtime_uses_selected_ports_and_disables_startup_reimport(self):
        with tempfile.TemporaryDirectory() as directory:
            files = pilot.prepare_runtime(Path(directory), 18594, 18590, False)
            config = json.loads(Path(files["BASYX_MCP_CONFIG_FILE"]).read_text())
            self.assertEqual([], config["general"]["aasPreconfigPaths"])
            self.assertEqual("http://localhost:18594/mcp", config["mcp"]["publicUrl"])
            self.assertEqual("disabled", config["mcp"]["writeMode"])
            realm = json.loads(Path(files["BASYX_MCP_REALM_FILE"]).read_text())
            client = next(
                c for c in realm["clients"] if c["clientId"] == "codex-basyx-mvp"
            )
            self.assertIn("http://127.0.0.1:18586/callback", client["redirectUris"])
            self.assertFalse(client["directAccessGrantsEnabled"])

    def test_start_refuses_a_legacy_database_without_recreating_it(self):
        args = SimpleNamespace(project="pilot", api_port=18594, oidc_port=18590)
        replies = [
            SimpleNamespace(stdout=""),
            SimpleNamespace(stdout=""),
            SimpleNamespace(stdout="database-id"),
            SimpleNamespace(
                stdout='[{"Type":"volume","Name":"anonymous-old-volume","Destination":"/var/lib/postgresql"}]'
            ),
        ]
        with (
            patch("pilot.subprocess.run", side_effect=replies),
            patch("pilot.compose") as compose,
            patch("pilot.wait_for_api"),
            patch("pilot.status", return_value={}),
        ):
            with self.assertRaisesRegex(pilot.PilotError, "LEGACY"):
                pilot.start(args, {})
            compose.assert_not_called()

    def test_explicit_disable_writes_overrides_saved_project_choice(self):
        with tempfile.TemporaryDirectory() as directory:
            runtime = Path(directory) / ".pilot" / "pilot"
            runtime.mkdir(parents=True)
            (runtime / "state.json").write_text(json.dumps({"writes": True}))
            with (
                patch("pilot.DIRECTORY", Path(directory)),
                patch(
                    "sys.argv",
                    ["pilot.py", "start", "--project", "pilot", "--disable-writes"],
                ),
                patch("pilot.prepare_runtime", return_value={}) as prepare,
                patch("pilot.start", return_value={}),
                contextlib.redirect_stdout(io.StringIO()),
            ):
                pilot.main()
                self.assertFalse(prepare.call_args.args[3])
            self.assertFalse(json.loads((runtime / "state.json").read_text())["writes"])

    def test_status_does_not_claim_tools_are_attached_to_a_chat(self):
        status = pilot.connection_status(["find_asset", "get_asset_measurement"])
        self.assertEqual("ready", status["server"])
        self.assertEqual("not_verified", status["chat_attachment"])
        self.assertEqual(2, status["available_tools"])


if __name__ == "__main__":
    unittest.main()
