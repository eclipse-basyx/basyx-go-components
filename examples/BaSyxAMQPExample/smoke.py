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
import os
import time
from urllib.request import Request, urlopen
import uuid

BASE = os.environ.get("BASYX_AMQP_EXAMPLE_URL", "http://localhost:8082")
BROKER = os.environ.get("BASYX_AMQP_EXAMPLE_MANAGEMENT_URL", "http://localhost:15672")


def request(method, url, data=None, broker=False):
    raw = None if data is None else json.dumps(data).encode()
    headers = {"Content-Type": "application/json"}
    if broker:
        credentials = base64.b64encode(b"basyx:basyx-demo").decode()
        headers["Authorization"] = "Basic " + credentials
    with urlopen(Request(url, data=raw, method=method, headers=headers), timeout=15) as response:
        content = response.read()
        return json.loads(content) if content else None


def main():
    identifier = "urn:example:amqp:" + str(uuid.uuid4())
    endpoint = BASE + "/submodels/" + base64.urlsafe_b64encode(identifier.encode()).decode().rstrip("=")
    request("POST", BASE + "/submodels", {"id": identifier, "modelType": "Submodel", "submodelElements": []})
    try:
        deadline = time.monotonic() + 45
        while time.monotonic() < deadline:
            messages = request("POST", BROKER + "/api/queues/%2F/basyx.events/get",
                               {"count": 1000, "ackmode": "ack_requeue_true", "encoding": "auto"}, broker=True)
            for message in messages:
                payload = message["payload"]
                if message["payload_encoding"] == "base64":
                    payload = base64.b64decode(payload)
                event = json.loads(payload)
                if event.get("subject") != identifier:
                    continue
                assert message["properties"]["content_type"] == "application/cloudevents+json"
                assert event["type"] == "io.admin-shell.submodel.created.v1"
                assert event["specversion"] == "1.0"
                assert event["datacontenttype"] == "application/json"
                assert event["id"] and event["time"] and event["dataschema"]
                print("AMQP smoke passed: Submodel creation published a structured CloudEvent")
                return
            time.sleep(0.25)
        raise RuntimeError("AMQP-SMOKE-TIMEOUT broker message missing")
    finally:
        request("DELETE", endpoint)


if __name__ == "__main__":
    main()
