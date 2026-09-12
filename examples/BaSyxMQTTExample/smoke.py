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

BASE = os.environ.get("BASYX_MQTT_EXAMPLE_URL", "http://localhost:8082")
COMPOSE = str(Path(__file__).with_name("docker-compose.yml"))


def request(method, path, data=None):
    raw = None if data is None else json.dumps(data).encode()
    with urlopen(Request(BASE + path, data=raw, method=method,
                         headers={"Content-Type": "application/json"}), timeout=15) as response:
        content = response.read()
        return json.loads(content) if content else None


def line_before(process, deadline):
    remaining = deadline - time.monotonic()
    if remaining <= 0 or not select.select([process.stdout], [], [], remaining)[0]:
        raise RuntimeError("MQTT-SMOKE-TIMEOUT broker message missing")
    line = process.stdout.readline()
    if not line:
        raise RuntimeError("MQTT-SMOKE-SUBSCRIBER subscriber stopped")
    return line


def main():
    identifier = "urn:example:mqtt:" + str(uuid.uuid4())
    endpoint = "/submodels/" + base64.urlsafe_b64encode(identifier.encode()).decode().rstrip("=")
    broker = ["docker", "compose", "-f", COMPOSE, "exec", "-T", "mqtt"]
    readiness = "test/readiness/" + str(uuid.uuid4())
    subprocess.run(broker + ["mosquitto_pub", "-V", "mqttv5", "-t", readiness,
                             "-r", "-m", "ready"], check=True, timeout=10)
    command = broker + [
               "mosquitto_sub", "-V", "mqttv5", "-t", "basyx/submodelrepository/submodel/created",
               "-t", readiness, "-C", "2", "-W", "20", "-F", "%p"]
    process = subprocess.Popen(command, stdout=subprocess.PIPE, stderr=subprocess.STDOUT, bufsize=0)
    created = False
    try:
        deadline = time.monotonic() + 25
        while line_before(process, deadline).strip() != b"ready":
            pass
        request("POST", "/submodels", {"id": identifier, "modelType": "Submodel", "submodelElements": []})
        created = True
        while True:
            line = line_before(process, deadline).strip()
            if line.startswith(b"{"):
                event = json.loads(line)
                break
        assert event["subject"] == identifier
        assert event["specversion"] == "1.0"
        assert event["datacontenttype"] == "application/json"
        assert event["type"] == "io.admin-shell.submodel.created.v1"
        assert event["id"] and event["time"] and event["dataschema"]
        print("MQTT smoke passed: Submodel creation published a CloudEvent")
    finally:
        if created:
            request("DELETE", endpoint)
        process.terminate()
        process.wait(timeout=10)
        subprocess.run(broker + ["mosquitto_pub", "-V", "mqttv5", "-t", readiness,
                                 "-r", "-n"], check=True, timeout=10)


if __name__ == "__main__":
    main()
