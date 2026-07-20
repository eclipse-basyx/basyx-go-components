################################################################################
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
################################################################################

import argparse
import base64
import concurrent.futures
import hashlib
import json
import math
import statistics
import sys
import time
import urllib.error
import urllib.request
import uuid


DEFAULT_BASE_URL = "http://localhost:5004"
REQUEST_TIMEOUT_SECONDS = 180


def parse_integer_list(raw_value):
    values = [int(value.strip()) for value in raw_value.split(",") if value.strip()]
    if not values or any(value < 1 for value in values):
        raise argparse.ArgumentTypeError("values must be positive comma-separated integers")
    return values


def parse_arguments():
    parser = argparse.ArgumentParser(description="Benchmark Submodel mutation and WORM evidence latency")
    parser.add_argument("--base-url", default=DEFAULT_BASE_URL)
    parser.add_argument("--label", default="local")
    parser.add_argument("--evidence-enabled", action="store_true")
    parser.add_argument("--snapshot-interval", type=int, default=5)
    parser.add_argument("--iterations", type=int, default=5)
    parser.add_argument("--element-counts", type=parse_integer_list, default=[100, 1000, 10000])
    parser.add_argument("--binary-sizes-mib", type=parse_integer_list, default=[1, 32])
    parser.add_argument("--concurrency", type=int, default=1)
    parser.add_argument("--output", default="-")
    args = parser.parse_args()
    if args.iterations < 1 or args.snapshot_interval < 1 or args.concurrency < 1:
        parser.error("iterations, snapshot interval, and concurrency must be positive")
    return args


def encode_identifier(identifier):
    return base64.urlsafe_b64encode(identifier.encode("utf-8")).decode("ascii").rstrip("=")


def property_element(index, value="initial"):
    return {
        "idShort": f"Property{index:05d}",
        "modelType": "Property",
        "valueType": "xs:string",
        "value": value,
    }


def file_element():
    return {
        "idShort": "BenchmarkFile",
        "modelType": "File",
        "contentType": "application/octet-stream",
    }


def build_submodel(identifier, element_count, include_file=False):
    elements = [property_element(index) for index in range(element_count)]
    if include_file:
        elements.append(file_element())
    return {
        "id": identifier,
        "idShort": "EvidenceBenchmark",
        "modelType": "Submodel",
        "submodelElements": elements,
    }


def percentile(values, percentile_value):
    ordered = sorted(values)
    index = max(0, math.ceil((percentile_value / 100) * len(ordered)) - 1)
    return ordered[index]


def summarize(scenario, durations, payload_bytes, failures):
    summary = {
        "scenario": scenario,
        "successes": len(durations),
        "failures": failures,
        "http_failures": [failure for failure in failures if failure.startswith("HTTP ")],
        "payload_bytes": payload_bytes,
    }
    if durations:
        summary.update({
            "p50_ms": round(statistics.median(durations), 3),
            "p95_ms": round(percentile(durations, 95), 3),
            "max_ms": round(max(durations), 3),
        })
    return summary


class HTTPResponse:
    def __init__(self, status_code, headers, content):
        self.status_code = status_code
        self.headers = headers
        self.content = content

    @property
    def text(self):
        return self.content.decode("utf-8", errors="replace")

    def json(self):
        return json.loads(self.content)

    def close(self):
        return None


def encode_multipart(fields, files):
    boundary = "----basyx-benchmark-" + uuid.uuid4().hex
    body = bytearray()
    for name, value in fields.items():
        body.extend(f"--{boundary}\r\n".encode("ascii"))
        body.extend(f'Content-Disposition: form-data; name="{name}"\r\n\r\n'.encode("utf-8"))
        body.extend(str(value).encode("utf-8"))
        body.extend(b"\r\n")
    for name, (file_name, content, content_type) in files.items():
        body.extend(f"--{boundary}\r\n".encode("ascii"))
        body.extend(f'Content-Disposition: form-data; name="{name}"; filename="{file_name}"\r\n'.encode("utf-8"))
        body.extend(f"Content-Type: {content_type}\r\n\r\n".encode("ascii"))
        body.extend(content)
        body.extend(b"\r\n")
    body.extend(f"--{boundary}--\r\n".encode("ascii"))
    return bytes(body), f"multipart/form-data; boundary={boundary}"


def prepare_request_body(kwargs):
    headers = dict(kwargs.get("headers", {}))
    if "json" in kwargs:
        body = json.dumps(kwargs["json"], separators=(",", ":")).encode("utf-8")
        headers.setdefault("Content-Type", "application/json")
        return body, headers
    if "files" in kwargs:
        body, content_type = encode_multipart(kwargs.get("data", {}), kwargs["files"])
        headers["Content-Type"] = content_type
        return body, headers
    return None, headers


def timed_request(method, url, expected_statuses, **kwargs):
    body, headers = prepare_request_body(kwargs)
    request = urllib.request.Request(url, data=body, headers=headers, method=method)
    started = time.perf_counter()
    try:
        with urllib.request.urlopen(request, timeout=REQUEST_TIMEOUT_SECONDS) as raw_response:
            response = HTTPResponse(raw_response.status, raw_response.headers, raw_response.read())
    except urllib.error.HTTPError as error:
        response = HTTPResponse(error.code, error.headers, error.read())
    except (urllib.error.URLError, TimeoutError) as error:
        return None, None, str(error)
    duration_ms = (time.perf_counter() - started) * 1000
    if response.status_code not in expected_statuses:
        message = f"HTTP {response.status_code}: {response.text[:500]}"
        return response, None, message
    return response, duration_ms, None


class BenchmarkRunner:
    def __init__(self, args):
        self.args = args
        self.base_url = args.base_url.rstrip("/")
        self.results = []

    def run(self):
        self.assert_service_is_ready()
        for element_count in self.args.element_counts:
            self.run_model_scenario(element_count)
        for size_mib in self.args.binary_sizes_mib:
            self.run_binary_scenario(size_mib)
        if self.args.concurrency > 1:
            self.run_concurrent_model_scenario()
            self.run_concurrent_binary_scenario()
        return self.report()

    def assert_service_is_ready(self):
        response, _, error = timed_request("GET", self.base_url + "/health", {200})
        if error:
            raise RuntimeError(f"service is not ready: {error}")
        response.close()

    def run_model_scenario(self, element_count):
        create_durations = []
        update_durations = []
        create_failures = []
        update_failures = []
        payload_bytes = 0
        update_payload_bytes = 0
        for iteration in range(self.args.iterations):
            identifier = f"urn:basyx:benchmark:model:{element_count}:{uuid.uuid4()}"
            payload = build_submodel(identifier, element_count)
            encoded_payload = json.dumps(payload, separators=(",", ":")).encode("utf-8")
            payload_bytes = len(encoded_payload)
            response, duration, error = timed_request(
                "POST",
                self.base_url + "/submodels",
                {201},
                json=payload,
                headers={"Content-Type": "application/json"},
            )
            self.collect(response, duration, error, create_durations, create_failures)
            if error:
                continue
            update_url = self.submodel_url(identifier) + "/submodel-elements/Property00000"
            updated_property = property_element(0, f"updated-{iteration}")
            update_payload_bytes = len(json.dumps(updated_property, separators=(",", ":")).encode("utf-8"))
            response, duration, error = timed_request(
                "PUT",
                update_url,
                {204},
                json=updated_property,
                headers={"Content-Type": "application/json"},
            )
            self.collect(response, duration, error, update_durations, update_failures)
        self.results.append(summarize(f"create_{element_count}_elements", create_durations, payload_bytes, create_failures))
        self.results.append(summarize(f"small_update_{element_count}_elements", update_durations, update_payload_bytes, update_failures))

    def run_binary_scenario(self, size_mib):
        first_durations = []
        duplicate_durations = []
        first_failures = []
        duplicate_failures = []
        size_bytes = size_mib * 1024 * 1024
        for iteration in range(self.args.iterations):
            identifier = f"urn:basyx:benchmark:binary:{size_mib}:{uuid.uuid4()}"
            self.create_binary_submodel(identifier)
            payload = deterministic_payload(size_bytes, (size_mib * 1000) + iteration)
            endpoint = self.attachment_url(identifier)
            first_path, first_duration, first_error = self.upload_and_read_path(endpoint, payload, f"benchmark-{size_mib}.bin")
            self.collect_duration(first_duration, first_error, first_durations, first_failures)
            second_path, second_duration, second_error = self.upload_and_read_path(endpoint, payload, f"benchmark-{size_mib}.bin")
            self.collect_duration(second_duration, second_error, duplicate_durations, duplicate_failures)
            if not first_error and not second_error and first_path == second_path:
                duplicate_failures.append("identical re-upload did not rotate the managed path")
            download_error = self.verify_download(endpoint, payload)
            if download_error:
                duplicate_failures.append(download_error)
        self.results.append(summarize(f"binary_first_{size_mib}_mib", first_durations, size_bytes, first_failures))
        self.results.append(summarize(f"binary_duplicate_{size_mib}_mib", duplicate_durations, size_bytes, duplicate_failures))

    def run_concurrent_model_scenario(self):
        identifiers = [f"urn:basyx:benchmark:concurrent:model:{uuid.uuid4()}" for _ in range(self.args.concurrency)]
        for identifier in identifiers:
            self.create_model(identifier, 100)

        def update(index):
            url = self.submodel_url(identifiers[index]) + "/submodel-elements/Property00000"
            response, duration, error = timed_request("PUT", url, {204}, json=property_element(0, f"parallel-{index}"))
            if response is not None:
                response.close()
            return duration, error

        durations, failures = self.run_parallel(update)
        self.results.append(summarize("concurrent_independent_model_updates", durations, 0, failures))

    def run_concurrent_binary_scenario(self):
        identifiers = [f"urn:basyx:benchmark:concurrent:binary:{uuid.uuid4()}" for _ in range(self.args.concurrency)]
        for identifier in identifiers:
            self.create_binary_submodel(identifier)
        payload = deterministic_payload(1024 * 1024, 97)

        def upload(index):
            _, duration, error = self.upload_and_read_path(self.attachment_url(identifiers[index]), payload, "shared.bin")
            return duration, error

        durations, failures = self.run_parallel(upload)
        for identifier in identifiers:
            download_error = self.verify_download(self.attachment_url(identifier), payload)
            if download_error:
                failures.append(download_error)
        self.results.append(summarize("concurrent_identical_binary_uploads", durations, len(payload), failures))

    def run_parallel(self, operation):
        durations = []
        failures = []
        with concurrent.futures.ThreadPoolExecutor(max_workers=self.args.concurrency) as executor:
            for duration, error in executor.map(operation, range(self.args.concurrency)):
                if error:
                    failures.append(error)
                elif duration is not None:
                    durations.append(duration)
        return durations, failures

    def create_model(self, identifier, element_count):
        response, _, error = timed_request("POST", self.base_url + "/submodels", {201}, json=build_submodel(identifier, element_count))
        if response is not None:
            response.close()
        if error:
            raise RuntimeError(f"failed to create benchmark submodel: {error}")

    def create_binary_submodel(self, identifier):
        payload = build_submodel(identifier, 1, include_file=True)
        response, _, error = timed_request("POST", self.base_url + "/submodels", {201}, json=payload)
        if response is not None:
            response.close()
        if error:
            raise RuntimeError(f"failed to create binary benchmark submodel: {error}")

    def upload_and_read_path(self, endpoint, payload, file_name):
        files = {"file": (file_name, payload, "application/octet-stream")}
        response, duration, error = timed_request("PUT", endpoint, {204}, files=files, data={"fileName": file_name})
        if response is not None:
            response.close()
        if error:
            return "", duration, error
        element_response, _, element_error = timed_request("GET", endpoint.removesuffix("/attachment"), {200})
        if element_error:
            return "", duration, element_error
        try:
            model_path = element_response.json().get("value", "")
        finally:
            element_response.close()
        if not model_path.startswith("/aasx/files/"):
            return model_path, duration, f"upload returned invalid managed path {model_path!r}"
        return model_path, duration, None

    def verify_download(self, endpoint, expected):
        response, _, error = timed_request("GET", endpoint, {200})
        if error:
            return f"failed to download benchmark attachment: {error}"
        try:
            if response.content != expected:
                return "downloaded benchmark attachment differs from uploaded bytes"
        finally:
            response.close()
        return None

    def submodel_url(self, identifier):
        return self.base_url + "/submodels/" + encode_identifier(identifier)

    def attachment_url(self, identifier):
        return self.submodel_url(identifier) + "/submodel-elements/BenchmarkFile/attachment"

    def collect(self, response, duration, error, durations, failures):
        if response is not None:
            response.close()
        if error:
            failures.append(error)
        elif duration is not None:
            durations.append(duration)

    def collect_duration(self, duration, error, durations, failures):
        if error:
            failures.append(error)
        elif duration is not None:
            durations.append(duration)

    def report(self):
        return {
            "benchmark_version": 1,
            "label": self.args.label,
            "configuration": {
                "base_url": self.base_url,
                "evidence_enabled": self.args.evidence_enabled,
                "full_snapshot_interval": self.args.snapshot_interval,
                "iterations": self.args.iterations,
                "concurrency": self.args.concurrency,
                "element_counts": self.args.element_counts,
                "binary_sizes_mib": self.args.binary_sizes_mib,
            },
            "generated_at_unix": int(time.time()),
            "results": self.results,
            "failure_count": sum(len(result["failures"]) for result in self.results),
        }


def deterministic_payload(size_bytes, seed):
    block = hashlib.sha256(f"basyx-evidence-benchmark-{seed}".encode("utf-8")).digest()
    repeats, remainder = divmod(size_bytes, len(block))
    return block * repeats + block[:remainder]


def write_report(report, destination):
    serialized = json.dumps(report, indent=2, sort_keys=True) + "\n"
    if destination == "-":
        sys.stdout.write(serialized)
        return
    with open(destination, "w", encoding="utf-8") as output_file:
        output_file.write(serialized)


def main():
    args = parse_arguments()
    report = BenchmarkRunner(args).run()
    write_report(report, args.output)
    return 1 if report["failure_count"] else 0


if __name__ == "__main__":
    try:
        sys.exit(main())
    except RuntimeError as error:
        sys.stderr.write(f"benchmark failed: {error}\n")
        sys.exit(1)
