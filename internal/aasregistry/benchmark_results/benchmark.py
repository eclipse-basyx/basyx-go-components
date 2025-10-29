import subprocess
import time
import json
import os
import requests
import uuid
import copy
import random
import base64
from datetime import datetime
import signal
import sys

# ──────────────────────────────────────────────
# CONFIG
# ──────────────────────────────────────────────
COMPOSE_FILE = "docker_compose/docker_compose.yml"
DB_CONTAINER = "postgres_db"  # optional: used if you have a healthcheck on DB
DISCOVERY_URL = "http://localhost:5004/shell-descriptors"

TOTAL_ITERS = 100000
SEED = 42

# How many iterations should be POST-only (prewarm). Set to 0 to disable.
PREWARM_ITERS = 1000

OP_WEIGHTS = {
    "post": 0.4,
    "get": 0.2,
    "search_all": 0.2,
    "search_limit100": 0.2
}

LOG_FILE = "runtime_results.json"

# When True, each log entry includes request_url, request_body, and response_body
LOG_REQUEST_DETAILS = False

# ──────────────────────────────────────────────
# Helper functions
# ──────────────────────────────────────────────
def run(cmd: str):
    print(f"▶ {cmd}")
    return subprocess.run(cmd, shell=True, check=False)

def wait_for_container_health(container: str, timeout: int = 120):
    """
    Waits for a container to be healthy via 'podman inspect'.
    Only useful if the container defines a HEALTHCHECK in your compose.
    """
    print(f"⏳ Waiting for '{container}' to be healthy...")
    start = time.time()
    while time.time() - start < timeout:
        result = subprocess.run(
            f"podman inspect --format='{{{{json .State.Health}}}}' {container}",
            shell=True, capture_output=True, text=True
        )
        if '"Status":"healthy"' in (result.stdout or ""):
            print(f"✅ {container} is healthy")
            return True
        time.sleep(3)
    print(f"❌ Timeout waiting for '{container}' health.")
    return False

def wait_for_http(url: str, timeout: int = 180):
    print(f"⏳ Waiting for service at {url}")
    start = time.time()
    while time.time() - start < timeout:
        try:
            r = requests.get(url, timeout=2)
            if r.status_code < 500:
                print(f"✅ Service responding: HTTP {r.status_code}")
                return True
        except Exception:
            pass
        time.sleep(2)
    print("❌ Timeout waiting for HTTP readiness.")
    return False

def url_b64_encode(s: str):
    return base64.urlsafe_b64encode(s.encode()).decode().rstrip("=")

def ts():
    return datetime.now().strftime("%H:%M:%S")

# ──────────────────────────────────────────────
# Benchmark payload (template)
# ──────────────────────────────────────────────
data_big = {
    "description": [
        {"language": "en", "text": "Machine consisting of multiple parts which are assets provided by different companies"},
        {"language": "de", "text": "Maschine bestehend aus mehreren Assets, welche von verschiedenen anderen Firmen bereitgestellt werden"}
    ],
    "displayName": [
        {"language": "en", "text": "Composite Machine"},
        {"language": "de", "text": "Verbundmaschine"}
    ],
    "specificAssetIds": [
        {
            "name": "h1",
            "value": "value",
            "semanticId": {"type": "ModelReference"},
            "supplementalSemanticIds": [
                {
                    "type": "ModelReference",
                    "keys": [{"type": "AnnotatedRelationshipElement", "value": "ddsfkjn"}]
                }
            ]
        }
    ],
    "administration": {
        "version": "1",
        "revision": "1",
        "creator": {
            "type": "ModelReference",
            "keys": [{"type": "AnnotatedRelationshipElement", "value": "dfok"}],
            "referredSemanticId": {
                "type": "ExternalReference",
                "keys": [{"type": "AnnotatedRelationshipElement", "value": "fkvkfk"}]
            }
        }
    },
    "assetKind": "Instance",
    "assetType": "machine",
    "endpoints": [
        {
            "interface": "AAS-3.0",
            "protocolInformation": {
                "href": "https://demo3.digital-twin.host/aas-environment/shells/aHR0cHM6Ly9pZXNlLmZyYXVuaG9mZXIuZGUvaWRzL2Fhcy83NjA5XzQxMTZfMTYyMF8yN1Tk5",
                "endpointProtocol": "https",
                "subprotocol": "lol",
                "subprotocolBody": "lil",
                "subprotocolBodyEncoding": "dsoinf",
                "securityAttributes": [
                    {"type": "NONE", "key": "dofijs", "value": "NONE"}
                ]
            }
        }
    ],
    "globalAssetId": "https://iese.fraunhofer.de/ids/asset/5079_8944_8914_9414",
    "idShort": "machine",
    # "id" will be overwritten with a unique value per POST
    "id": "https://iese.fraunhofer.de/ids/aas/sdfgbfdb",
    "extensions": [
        {
            "name": "help",
            "valueType": "xs:string",
            "value": "test",
            "semanticId": {
                "type": "ExternalReference",
                "keys": [{"type": "AnnotatedRelationshipElement", "value": "ddsfkjn"}],
                "referredSemanticId": {
                    "type": "ExternalReference",
                    "keys": [{"type": "AnnotatedRelationshipElement", "value": "ddsfkjn"}]
                }
            },
            "supplementalSemanticIds": [
                {
                    "type": "ExternalReference",
                    "keys": [{"type": "AnnotatedRelationshipElement", "value": "ddsfkjn"}],
                    "referredSemanticId": {
                        "type": "ExternalReference",
                        "keys": [{"type": "AnnotatedRelationshipElement", "value": "ddsfkjn"}]
                    }
                }
            ]
        }
    ],
    "submodelDescriptors": [
        {
            # "id" will be overwritten with a unique value per POST
            "id": "https://iese.fraunhofer.de/ids/aas/yxcyxc",
            "endpoints": [
                {
                    "interface": "AAS-3.0",
                    "protocolInformation": {
                        "href": "https://demo3.digital-twin.host/aas-environment/shells/aHR0cHM6Ly9pZXNlLmZyYXVuaG9mZXIuZGUvaWRzL2Fhcy83NjA5XzQxMTZfMTYyMF8yNTk15",
                        "endpointProtocol": "https"
                    }
                }
            ],
            "semanticId": {
                "type": "ExternalReference",
                "keys": [{"type": "AnnotatedRelationshipElement", "value": "ddsfkjn"}],
                "referredSemanticId": {
                    "type": "ExternalReference",
                    "keys": [{"type": "AnnotatedRelationshipElement", "value": "ddsfkjn"}]
                }
            },
            "supplementalSemanticIds": [
                {
                    "type": "ExternalReference",
                    "keys": [{"type": "AnnotatedRelationshipElement", "value": "ddsfkjn"}],
                    "referredSemanticId": {
                        "type": "ExternalReference",
                        "keys": [{"type": "AnnotatedRelationshipElement", "value": "ddsfkjn"}]
                    }
                }
            ],
            "extensions": [
                {"name": "help", "valueType": "xs:string", "value": "test"}
            ]
        }
    ]
}

data = {
    "idShort": "InduCoreIC5000",
    "id": "https://id.idta-showcase.net/namespace/InduCoreIC5000/v1",
    "endpoints": [
        {
            "protocolInformation": {
                "href": "https://idta.basyx-enterprise.net/controller/datasource/unprotected-repository-aas/shells/aHR0cHM6Ly9pZC5pZHRhLXNob3djYXNlLm5ldC9uYW1lc3BhY2UvSW5kdUNvcmVJQzUwMDAvdjE",
                "endpointProtocol": "http"
            },
            "interface": "AAS-3.0"
        }
    ]
}
payload_template = data
# ──────────────────────────────────────────────
# Cleanup handler
# ──────────────────────────────────────────────
def cleanup():
    print("\n🧹 Stopping Docker stack...")
    run(f"podman compose -f {COMPOSE_FILE} down -v")

def sig_handler(sig, frame):
    print("\n⚠ Interrupted by user.")
    cleanup()
    sys.exit(0)

signal.signal(signal.SIGINT, sig_handler)
signal.signal(signal.SIGTERM, sig_handler)

# ──────────────────────────────────────────────
# MAIN
# ──────────────────────────────────────────────
def main():
    print("🚀 Starting Docker Compose...")
    up_rc = run(f"podman compose -f {COMPOSE_FILE} up -d --build").returncode
    if up_rc != 0:
        print("❌ Failed to start the compose stack (check build paths and context).")
        sys.exit(1)

    # If you want to wait for DB health (only meaningful if healthcheck is defined on DB):
    # wait_for_container_health(DB_CONTAINER)  # uncomment if desired

    if not wait_for_http(DISCOVERY_URL):
        cleanup()
        return

    # Normalize prewarm iterations if they exceed total
    effective_prewarm = max(0, min(PREWARM_ITERS, TOTAL_ITERS))

    print("🔥 Starting benchmark...")
    if effective_prewarm > 0:
        print(f"♨️  Prewarm enabled: first {effective_prewarm} ops will be POST only.")

    session = requests.Session()
    rng = random.Random(SEED)

    logs = []
    descriptor_pool = []  # store descriptor ids (payload["id"]) to use for GET
    submodel_pool = []    # optional storage of submodel ids (not used now)

    ops = list(OP_WEIGHTS.keys())
    weights = list(OP_WEIGHTS.values())

    for i in range(TOTAL_ITERS):
        # Force POST during prewarm phase
        if i < effective_prewarm:
            op = "post"
        else:
            op = rng.choices(ops, weights)[0]
            # If GET chosen but we have nothing to GET yet, fall back to searches
            if op == "get" and not descriptor_pool:
                op = rng.choice(["search_all", "search_limit100"])

        req_url = None
        req_body = None
        resp_body = None

        t0 = time.perf_counter()
        ok = False
        code = None

        try:
            if op == "post":
                payload = copy.deepcopy(payload_template)

                # Unique descriptor ID (AAS descriptor id)
                descriptor_id = f"https://example.org/aas/{uuid.uuid4()}"
                payload["id"] = descriptor_id

                # Unique submodel descriptor IDs
                if "submodelDescriptors" in payload and payload["submodelDescriptors"]:
                    for sm in payload["submodelDescriptors"]:
                        sm["id"] = f"https://example.org/submodel/{uuid.uuid4()}"

                req_url = DISCOVERY_URL
                req_body = payload

                r = session.post(req_url, json=payload)
                ok = r.ok
                code = r.status_code
                if LOG_REQUEST_DETAILS:
                    try:
                        resp_body = r.json()
                    except ValueError:
                        resp_body = r.text

                if ok:
                    descriptor_pool.append(descriptor_id)
                    for sm in payload.get("submodelDescriptors", []):
                        submodel_pool.append(sm["id"])

            elif op == "get":
                # Use the stored descriptor id for GET-by-id, base64url-encoded
                descriptor_id = rng.choice(descriptor_pool)
                encoded = url_b64_encode(descriptor_id)
                req_url = f"{DISCOVERY_URL}/{encoded}"

                r = session.get(req_url)
                ok = r.ok
                code = r.status_code
                if LOG_REQUEST_DETAILS:
                    try:
                        resp_body = r.json()
                    except ValueError:
                        resp_body = r.text

            elif op == "search_all":
                req_url = DISCOVERY_URL

                r = session.get(req_url)
                ok = r.ok
                code = r.status_code
                if LOG_REQUEST_DETAILS:
                    try:
                        resp_body = r.json()
                    except ValueError:
                        resp_body = r.text

            elif op == "search_limit100":
                req_url = DISCOVERY_URL + "?limit=100"

                r = session.get(DISCOVERY_URL, params={"limit": 100})
                ok = r.ok
                code = r.status_code
                if LOG_REQUEST_DETAILS:
                    try:
                        resp_body = r.json()
                    except ValueError:
                        resp_body = r.text

        except Exception as e:
            print(f"[{ts()}] ❌ Error: {e}")

        t1 = time.perf_counter()
        entry = {
            "iter": i,
            "op": op,
            "code": code,
            "ok": ok,
            "duration_ms": int((t1 - t0) * 1000)
        }

        if LOG_REQUEST_DETAILS:
            entry["request_url"] = req_url
            entry["request_body"] = req_body
            entry["response_body"] = resp_body

        logs.append(entry)

        if i % 100 == 0:
            print(f"[{ts()}] {i}/{TOTAL_ITERS} ops done | descriptors={len(descriptor_pool)} | submodels={len(submodel_pool)}")

    print("✅ Benchmark finished.")
    print(f"💾 Saving results → {LOG_FILE}")
    with open(LOG_FILE, "w") as f:
        json.dump(logs, f, indent=2)

    cleanup()
    print("🏁 Done!")


if __name__ == "__main__":
    main()
