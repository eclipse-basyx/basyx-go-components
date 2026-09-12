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
from pathlib import Path
import select
import subprocess
import time
from urllib.request import Request, urlopen
import uuid

BASE = os.environ.get("BASYX_KAFKA_EXAMPLE_URL", "http://localhost:8082")
COMPOSE = str(Path(__file__).with_name("docker-compose.yml"))


def request(method, path, data=None):
    raw = None if data is None else json.dumps(data).encode()
    with urlopen(Request(BASE + path, data=raw, method=method,
                         headers={"Content-Type": "application/json"}), timeout=15) as response:
        content = response.read()
        return json.loads(content) if content else None


def main():
    identifier = "urn:example:kafka:" + str(uuid.uuid4())
    endpoint = "/submodels/" + base64.urlsafe_b64encode(identifier.encode()).decode().rstrip("=")
    request("POST", "/submodels", {"id": identifier, "modelType": "Submodel", "submodelElements": []})
    command = ["docker", "compose", "-f", COMPOSE, "exec", "-T", "kafka",
               "/opt/kafka/bin/kafka-console-consumer.sh", "--bootstrap-server", "kafka:19092",
               "--topic", "basyx.events", "--from-beginning", "--timeout-ms", "30000",
               "--property", "print.key=true", "--property", "print.headers=true"]
    process = subprocess.Popen(command, stdout=subprocess.PIPE, stderr=subprocess.PIPE, bufsize=0)
    try:
        deadline = time.monotonic() + 45
        while True:
            remaining = deadline - time.monotonic()
            if remaining <= 0 or not select.select([process.stdout], [], [], remaining)[0]:
                raise RuntimeError("KAFKA-SMOKE-TIMEOUT broker message missing")
            line = process.stdout.readline()
            if not line:
                raise RuntimeError("KAFKA-SMOKE-CONSUMER consumer stopped: " + process.stderr.read().decode())
            headers, key, value = line.decode().rstrip("\n").split("\t", 2)
            event = json.loads(value)
            if event.get("subject") != identifier:
                continue
            assert headers == "content-type:application/cloudevents+json"
            assert key == "submodel_history:" + identifier
            assert event["type"] == "io.admin-shell.submodel.created.v1"
            assert event["specversion"] == "1.0"
            assert event["datacontenttype"] == "application/json"
            assert event["id"] and event["time"] and event["dataschema"]
            print("Kafka smoke passed: Submodel creation published a structured CloudEvent")
            return
    finally:
        process.terminate()
        process.wait(timeout=10)
        request("DELETE", endpoint)


if __name__ == "__main__":
    main()
