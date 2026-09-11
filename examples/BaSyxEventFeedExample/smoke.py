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
from collections import Counter
import json
import os
from pathlib import Path
import signal
import sys
import time
from urllib.error import HTTPError, URLError
from urllib.parse import urlencode, urlsplit
from urllib.request import Request, urlopen
import uuid


PREFIX = "io.admin-shell."
PRESENTATIONS = ("REGULAR", "COMPACT")
PCN_SEMANTIC_ID = "0173-1#01-AHE582#003"


def check(condition, step, message):
    if not condition:
        raise RuntimeError(f"EVENTFEED-SMOKE-{step}: {message}")


def request(method, url, payload=None, expected=(200,), raw_response=False):
    body = None if payload is None else json.dumps(payload).encode("utf-8")
    headers = {"Accept": "application/json"}
    if body is not None:
        headers["Content-Type"] = "application/json"
    call = Request(url, data=body, headers=headers, method=method)
    try:
        with urlopen(call, timeout=5) as response:
            status, raw = response.status, response.read()
    except HTTPError as error:
        status, raw = error.code, error.read()
    except (URLError, TimeoutError) as error:
        raise RuntimeError(f"EVENTFEED-SMOKE-REQUEST: {method} {url}: {error}") from error
    check(status in expected, "HTTP", f"{method} {url}: HTTP {status}: {raw[:1000]!r}")
    if raw_response:
        return raw
    if not raw:
        return None
    try:
        return json.loads(raw)
    except json.JSONDecodeError as error:
        raise RuntimeError(f"EVENTFEED-SMOKE-JSON: {method} {url}: {error}") from error


def wait_for_url(url):
    deadline = time.monotonic() + 60
    last_error = None
    while time.monotonic() < deadline:
        try:
            return request("GET", url, raw_response=True)
        except RuntimeError as error:
            last_error = error
            time.sleep(1)
    raise RuntimeError(f"EVENTFEED-SMOKE-STARTUP: startup timed out: {last_error}")


def verify_seed(base):
    sample = json.loads((Path(__file__).parent / "aas" / "playground.json").read_text(encoding="utf-8"))
    check("EventFeedPlayground" in {item.get("idShort") for item in sample.get("assetAdministrationShells", [])},
          "SEED", "packaged example has no EventFeedPlayground AAS")
    check({"NoSemanticId", "ProductChangeNotifications"} <= {item.get("idShort") for item in sample.get("submodels", [])},
          "SEED", "packaged example is missing its demonstration Submodels")
    for route, group in (("shells", "assetAdministrationShells"), ("submodels", "submodels")):
        for expected in sample[group]:
            location = base + "/" + route + "/" + encoded(expected["id"])
            actual = json.loads(wait_for_url(location))
            check(actual.get("id") == expected["id"] and actual.get("idShort") == expected["idShort"],
                  "SEED", f"preconfigured model missing or incorrect: {expected['id']}")
            if expected["idShort"] == "NoSemanticId":
                check("semanticId" not in actual, "SEED", "NoSemanticId must have no semanticId")
            if expected["idShort"] == "ProductChangeNotifications":
                check(actual.get("semanticId", {}).get("keys") == [{"type": "GlobalReference", "value": PCN_SEMANTIC_ID}],
                      "SEED", "PCN semanticId is incorrect")
            if expected["idShort"] == "EventFeedPlayground":
                check(actual.get("submodels") == expected["submodels"], "SEED", "playground Submodel references are incorrect")


def verify_ui(ui_base):
    check(wait_for_url(ui_base + "/"), "UI", "UI root returned an empty document")
    served = wait_for_url(ui_base + "/config/basyx-infra.yml")
    packaged = (Path(__file__).parent / "basyx-infra.yml").read_bytes()
    check(served == packaged, "UI", "served basyx-infra.yml differs from the packaged configuration")


def read_capabilities(base):
    capabilities = request("GET", base + "/.well-known/event-feed.json")
    expected_types = {PREFIX + family + "." + action + ".v1"
                      for family in ("aas", "asset", "submodel")
                      for action in ("created", "updated", "deleted")}
    expected_types.add(PREFIX + "pcn.v1")
    check(expected_types <= capabilities.get("eventTypes", {}).keys(), "CAPABILITIES", "missing event types")
    check(set(PRESENTATIONS) <= set(capabilities.get("presentation", {}).get("supported", [])),
          "CAPABILITIES", "REGULAR and COMPACT must be supported")
    check(capabilities.get("auth", {}).get("inherited") is True, "CAPABILITIES", "authentication must be inherited")
    schemas = {}
    for definition in capabilities["eventTypes"].values():
        for location in definition["schemas"].values():
            if location not in schemas:
                check(urlsplit(location).scheme in ("http", "https"), "SCHEMA", f"schema is not HTTP(S): {location}")
                schemas[location] = request("GET", location)
                check(schemas[location].get("$schema") and schemas[location].get("$id"),
                      "SCHEMA", f"not a JSON Schema: {location}")
    for mode in PRESENTATIONS:
        location = capabilities["eventTypes"][PREFIX + "submodel.created.v1"]["schemas"][mode]
        check("semanticId" not in schemas[location].get("required", []), "SCHEMA", "semanticId must be optional")
    return capabilities


def encoded(identifier):
    return base64.urlsafe_b64encode(identifier.encode("utf-8")).decode("ascii").rstrip("=")


def prop(name, value):
    return {"modelType": "Property", "idShort": name, "valueType": "xs:string", "value": value}


def collection(name, *values):
    result = {"modelType": "SubmodelElementCollection", "value": list(values)}
    if name:
        result["idShort"] = name
    return result


def multilingual(name, text):
    return {"modelType": "MultiLanguageProperty", "idShort": name, "value": [{"language": "en", "text": text}]}


def collection_list(name, value):
    return {"modelType": "SubmodelElementList", "idShort": name,
            "typeValueListElement": "SubmodelElementCollection", "value": [value]}


def pcn_record(change_id):
    return collection(None,
        collection("Manufacturer", multilingual("ManufacturerName", "Event Feed Smoke Test"),
                   collection("PhysicalAddress", multilingual("Street", "Example Street 1"),
                              multilingual("CityTown", "Example City"))),
        prop("ManufacturerChangeID", change_id), prop("PcnType", "PCN"),
        collection_list("ReasonsOfChange", collection(None,
                        prop("ReasonClassificationSystem", "VDMA24903"), prop("ReasonId", "RAWM"))),
        collection_list("ItemCategories", collection(None,
                        prop("ItemClassificationSystem", "VDMA24903"), prop("ItemCategory", "ELME"))),
        collection("PcnChangeInformation", multilingual("ChangeTitle", "Example material change"),
                   multilingual("ChangeDetail", "Synthetic smoke-test notification")),
        prop("DateOfRecord", "2026-01-01T12:00:00Z"),
        collection("ItemOfChange", multilingual("ManufacturerProductFamily", "Smoke Test"),
                   multilingual("ManufacturerProductDesignation", "Smoke Test Component")))


def fixtures():
    run_id = uuid.uuid4().hex
    namespace = "urn:example:event-feed:smoke:" + run_id
    submodel = {"modelType": "Submodel", "id": namespace + ":submodel", "idShort": "SmokeSubmodel",
                "submodelElements": [prop("Status", "created")]}
    pcn = {"modelType": "Submodel", "id": namespace + ":pcn", "idShort": "SmokePCN",
           "semanticId": {"type": "ExternalReference", "keys": [{"type": "GlobalReference", "value": PCN_SEMANTIC_ID}]},
           "submodelElements": [collection_list("Records", pcn_record(run_id))]}
    shell = {"modelType": "AssetAdministrationShell", "id": namespace + ":aas", "idShort": "SmokeAAS",
             "assetInformation": {"assetKind": "Instance", "globalAssetId": namespace + ":asset"},
             "submodels": [{"type": "ModelReference", "keys": [{"type": "Submodel", "value": item["id"]}]}
                           for item in (submodel, pcn)]}
    return submodel, pcn, shell, run_id


def read_feed(base, subjects, presentation, page_size):
    parameters = {"filter": "rsql:event.subject=in=(" + ",".join(subjects) + ")",
                  "presentation": presentation, "limit": page_size}
    records, cursors = [], set()
    for _ in range(30):
        response = request("GET", base + "/events?" + urlencode(parameters))
        page = response.get("records")
        check(isinstance(page, list), "FEED", "response has no records array")
        records.extend(page)
        if "cursor" not in response:
            return records
        cursor = response["cursor"]
        check(isinstance(cursor, str) and cursor and cursor not in cursors, "CURSOR", "empty or repeated cursor")
        cursors.add(cursor)
        parameters = {"cursor": cursor, "limit": page_size}
    raise RuntimeError("EVENTFEED-SMOKE-CURSOR: pagination exceeded 30 pages")


def wait_for_events(base, subjects, expected, capabilities):
    deadline = time.monotonic() + 30
    actual = Counter()
    page_size = min(2, capabilities["maxPageSize"])
    check(page_size > 0, "CAPABILITIES", "maxPageSize must be positive")
    while time.monotonic() < deadline:
        records = read_feed(base, subjects, "REGULAR", page_size)
        actual = Counter((record.get("subject"), record.get("type")) for record in records)
        if all(actual[key] >= count for key, count in expected.items()):
            return records
        time.sleep(0.25)
    raise RuntimeError(f"EVENTFEED-SMOKE-PUBLISH: events did not arrive; expected={expected}, actual={actual}")


def event_counts(submodel, pcn, shell):
    expected = Counter()
    for subject, family in ((submodel["id"], "submodel"), (pcn["id"], "submodel"),
                            (shell["id"], "aas"), (shell["assetInformation"]["globalAssetId"], "asset")):
        for action in ("created", "deleted"):
            expected[(subject, PREFIX + family + "." + action + ".v1")] = 1
        if family != "submodel" or subject == submodel["id"]:
            expected[(subject, PREFIX + family + ".updated.v1")] = 1
    expected[(pcn["id"], PREFIX + "pcn.v1")] = 1
    return expected


def verify_records(records, expected, presentation, capabilities, submodel_id, pcn_id, change_id):
    counts = Counter((record.get("subject"), record.get("type")) for record in records)
    check(counts == expected, "RECORDS", f"unexpected {presentation} event counts: {counts}; expected {expected}")
    check(len({record.get("id") for record in records}) == len(records), "RECORDS", "duplicate event IDs")
    for record in records:
        event_type = record["type"]
        check(record.get("specversion") == "1.0" and record.get("time") and record.get("source"),
              "ENVELOPE", f"invalid CloudEvents envelope: {record}")
        check(record.get("dataschema") == capabilities["eventTypes"][event_type]["schemas"][presentation],
              "SCHEMA", f"schema differs from capabilities: {record}")
        if record["subject"] == submodel_id:
            check("semanticId" not in record.get("data", {}), "SEMANTICID", "invented semanticId for untyped Submodel")
        if record["subject"] == pcn_id and event_type == PREFIX + "pcn.v1":
            data = record.get("data", {})
            if presentation == "REGULAR":
                check(data.get("record", {}).get("ManufacturerChangeID") == change_id, "PCN", "notification record is missing")
            else:
                check(data.get("submodelId") == pcn_id, "PCN", "compact PCN cannot identify its Submodel")


def remove_resources(resources):
    failures = []
    for endpoint in reversed(resources):
        try:
            request("DELETE", endpoint, expected=(204, 404))
        except RuntimeError as error:
            failures.append(str(error))
    check(not failures, "CLEANUP", "; ".join(failures))


def exercise_feed(base, capabilities):
    submodel, pcn, shell, change_id = fixtures()
    resources = []
    try:
        for route, payload in (("submodels", submodel), ("submodels", pcn), ("shells", shell)):
            resources.append(base + "/" + route + "/" + encoded(payload["id"]))
            request("POST", base + "/" + route, payload, expected=(201,))
        submodel["submodelElements"][0]["value"] = "updated"
        request("PUT", resources[0], submodel, expected=(204,))
        shell["idShort"] = "UpdatedSmokeAAS"
        request("PUT", resources[2], shell, expected=(204,))
        remove_resources(resources)
        resources.clear()
        subjects = (submodel["id"], pcn["id"], shell["id"], shell["assetInformation"]["globalAssetId"])
        expected = event_counts(submodel, pcn, shell)
        regular = wait_for_events(base, subjects, expected, capabilities)
        compact = read_feed(base, subjects, "COMPACT", min(2, capabilities["maxPageSize"]))
        for presentation, records in (("REGULAR", regular), ("COMPACT", compact)):
            verify_records(records, expected, presentation, capabilities, submodel["id"], pcn["id"], change_id)
        check({record["id"] for record in regular} == {record["id"] for record in compact},
              "PRESENTATION", "presentations returned different event IDs")
        print(f"Event Feed example smoke passed: {len(regular)} CRUD/PCN events, both presentations, cursor pagination, served schemas, seeded models, and UI wiring.")
    finally:
        remove_resources(resources)


def terminate(signum, _frame):
    raise SystemExit(128 + signum)


def main():
    signal.signal(signal.SIGTERM, terminate)
    base = os.environ.get("BASYX_EVENT_FEED_BASE_URL", "http://localhost:8082").rstrip("/")
    check(urlsplit(base).scheme in ("http", "https") and urlsplit(base).netloc, "BASEURL", "expected an HTTP(S) API base URL")
    ui_base = os.environ.get("BASYX_EVENT_FEED_UI_BASE_URL", "http://localhost:3001").rstrip("/")
    check(urlsplit(ui_base).scheme in ("http", "https") and urlsplit(ui_base).netloc,
          "UIBASEURL", "expected an HTTP(S) UI base URL")
    wait_for_url(base + "/health")
    verify_seed(base)
    verify_ui(ui_base)
    capabilities = read_capabilities(base)
    exercise_feed(base, capabilities)


if __name__ == "__main__":
    try:
        main()
    except (RuntimeError, KeyboardInterrupt) as error:
        print(str(error) or "EVENTFEED-SMOKE-INTERRUPTED: stopped", file=sys.stderr)
        sys.exit(1)
