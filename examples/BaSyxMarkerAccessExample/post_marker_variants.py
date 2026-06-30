#!/usr/bin/env python3
import argparse
import base64
import hashlib
import http.client
import json
import threading
import time
import urllib.parse
import uuid
from concurrent.futures import FIRST_COMPLETED, ThreadPoolExecutor, wait


thread_local = threading.local()


def parse_args():
    parser = argparse.ArgumentParser(description="Post generated marker-access example variants.")
    parser.add_argument("--count", type=int, default=20)
    parser.add_argument("--start-index", type=int, default=1)
    parser.add_argument("--dtr-base-url", default="http://localhost:5004/api/v3")
    parser.add_argument("--smrepo-base-url", default="http://localhost:5005")
    parser.add_argument("--token-url", default="http://localhost:8080/realms/basyx/protocol/openid-connect/token")
    parser.add_argument("--client-id", default="basyx-ui")
    parser.add_argument("--username", default="admin")
    parser.add_argument("--password", default="pwd")
    parser.add_argument("--run-id", default=None, help="Seed for generated hashed IDs. Defaults to a new UUID per run.")
    parser.add_argument("--max-parallel", type=int, default=64)
    parser.add_argument("--request-timeout", type=float, default=60)
    parser.add_argument("--token-refresh-margin", type=float, default=30)
    args = parser.parse_args()

    if args.count < 1:
        parser.error("--count must be >= 1")
    if args.start_index < 1:
        parser.error("--start-index must be >= 1")
    if args.max_parallel < 1:
        parser.error("--max-parallel must be >= 1")
    if args.request_timeout <= 0:
        parser.error("--request-timeout must be > 0")
    if args.token_refresh_margin < 0:
        parser.error("--token-refresh-margin must be >= 0")
    if not args.run_id:
        args.run_id = uuid.uuid4().hex

    return args


def connection_key(parsed_url):
    port = parsed_url.port
    if port is None:
        port = 443 if parsed_url.scheme == "https" else 80
    return parsed_url.scheme, parsed_url.hostname, port


def get_connection(parsed_url, timeout):
    if not hasattr(thread_local, "connections"):
        thread_local.connections = {}

    key = connection_key(parsed_url)
    connection = thread_local.connections.get(key)
    if connection is not None:
        return connection

    if parsed_url.scheme == "https":
        connection = http.client.HTTPSConnection(parsed_url.hostname, parsed_url.port, timeout=timeout)
    else:
        connection = http.client.HTTPConnection(parsed_url.hostname, parsed_url.port, timeout=timeout)

    thread_local.connections[key] = connection
    return connection


def close_thread_connections():
    connections = getattr(thread_local, "connections", {})
    for connection in connections.values():
        connection.close()
    thread_local.connections = {}


def request_bytes(method, url, body, headers, timeout):
    parsed_url = urllib.parse.urlsplit(url)
    path = parsed_url.path or "/"
    if parsed_url.query:
        path = f"{path}?{parsed_url.query}"

    body_bytes = body.encode("utf-8") if isinstance(body, str) else body
    request_headers = dict(headers)
    request_headers.setdefault("Connection", "keep-alive")

    connection = get_connection(parsed_url, timeout)
    try:
        connection.request(method, path, body=body_bytes, headers=request_headers)
        response = connection.getresponse()
        response_body = response.read()
        return response.status, response_body
    except (http.client.HTTPException, OSError):
        connection.close()
        thread_local.connections.pop(connection_key(parsed_url), None)
        connection = get_connection(parsed_url, timeout)
        connection.request(method, path, body=body_bytes, headers=request_headers)
        response = connection.getresponse()
        response_body = response.read()
        return response.status, response_body


def get_access_token(args):
    form = urllib.parse.urlencode(
        {
            "grant_type": "password",
            "client_id": args.client_id,
            "username": args.username,
            "password": args.password,
        }
    )
    status, body = request_bytes(
        "POST",
        args.token_url,
        form,
        {"Content-Type": "application/x-www-form-urlencoded"},
        args.request_timeout,
    )
    if status != 200:
        raise RuntimeError(f"token request failed with status {status}: {body.decode('utf-8', errors='replace')}")

    payload = json.loads(body)
    return {
        "access_token": payload["access_token"],
        "expires_at": time.monotonic() + int(payload.get("expires_in", 300)),
    }


def ensure_token(args, token_state):
    if time.monotonic() + args.token_refresh_margin < token_state["expires_at"]:
        return token_state
    return get_access_token(args)


def base64_url(value):
    return base64.urlsafe_b64encode(value.encode("utf-8")).decode("ascii").rstrip("=")


def variant_hash(run_id, index, label, length=16):
    raw = f"{run_id}:{index}:{label}".encode("utf-8")
    return hashlib.sha256(raw).hexdigest()[:length]


def marker_at(position):
    if position % 5 == 0:
        return "PUBLIC_READABLE"
    return f"BPN_COMPANY_{((position - 1) % 20) + 1:03d}"


def marker_values(index, salt):
    target_count = 1 if (index + salt) % 3 == 0 else 2
    markers = []
    position = (index * 7) + (salt * 11)

    while len(markers) < target_count:
        marker = marker_at(position)
        if marker not in markers:
            markers.append(marker)
        position += 1

    return markers


def reference_keys_json(markers):
    return "[" + ",".join(f'{{"type":"GlobalReference","value":{json.dumps(marker)}}}' for marker in markers[:2]) + "]"


def variant_payloads(index, run_id, dtr_base_url, smrepo_base_url):
    suffix = variant_hash(run_id, index, "variant", 12)
    shell_id_part = variant_hash(run_id, index, "aas")
    asset_id_part = variant_hash(run_id, index, "asset")
    public_submodel_id_part = variant_hash(run_id, index, "nameplate")
    restricted_submodel_id_part = variant_hash(run_id, index, "serial-part-typization")
    public_markers = marker_values(index, 0)
    restricted_markers = marker_values(index, 1)
    public_keys = reference_keys_json(public_markers)
    restricted_keys = reference_keys_json(restricted_markers)

    shell_id = f"urn:example:aas:{shell_id_part}"
    asset_id = f"urn:example:asset:{asset_id_part}"
    public_submodel_id = f"urn:example:submodel:nameplate:{public_submodel_id_part}"
    restricted_submodel_id = f"urn:example:submodel:serial-part-typization:{restricted_submodel_id_part}"
    public_href = f"{smrepo_base_url}/submodels/{base64_url(public_submodel_id)}"
    restricted_href = f"{smrepo_base_url}/submodels/{base64_url(restricted_submodel_id)}"

    shell_json = (
        f'{{"idShort":"ProductTwin{suffix}","id":{json.dumps(shell_id)},'
        f'"description":[{{"language":"en","text":"Public and partner-restricted submodel descriptors."}}],'
        f'"globalAssetId":{json.dumps(asset_id)},'
        f'"specificAssetIds":[{{"name":"manufacturerPartId","value":"PART-{suffix}",'
        f'"externalSubjectId":{{"type":"ExternalReference","keys":{public_keys}}}}},'
        f'{{"name":"customerPartId","value":"CUSTOMER-PART-{suffix}",'
        f'"externalSubjectId":{{"type":"ExternalReference","keys":{restricted_keys}}}}}],'
        f'"submodelDescriptors":[{{"idShort":"PublicNameplate{suffix}","id":{json.dumps(public_submodel_id)},'
        f'"supplementalSemanticIds":[{{"type":"ExternalReference","keys":{public_keys}}}],'
        f'"endpoints":[{{"interface":"SUBMODEL-3.0","protocolInformation":{{"href":{json.dumps(public_href)},'
        f'"endpointProtocol":"HTTP"}}}}]}},'
        f'{{"idShort":"RestrictedSerialPartTypization{suffix}","id":{json.dumps(restricted_submodel_id)},'
        f'"supplementalSemanticIds":[{{"type":"ExternalReference","keys":{restricted_keys}}}],'
        f'"endpoints":[{{"interface":"SUBMODEL-3.0","protocolInformation":{{"href":{json.dumps(restricted_href)},'
        f'"endpointProtocol":"HTTP"}}}}]}}]}}'
    )
    public_submodel_json = (
        f'{{"id":{json.dumps(public_submodel_id)},"idShort":"PublicNameplate{suffix}",'
        f'"modelType":"Submodel","supplementalSemanticIds":[{{"type":"ExternalReference","keys":{public_keys}}}],'
        f'"submodelElements":[{{"idShort":"ManufacturerName","modelType":"Property","valueType":"xs:string",'
        f'"value":"Example Corp {suffix}"}}]}}'
    )
    restricted_submodel_json = (
        f'{{"id":{json.dumps(restricted_submodel_id)},"idShort":"RestrictedSerialPartTypization{suffix}",'
        f'"modelType":"Submodel","supplementalSemanticIds":[{{"type":"ExternalReference","keys":{restricted_keys}}}],'
        f'"submodelElements":[{{"idShort":"PublicPartType","modelType":"Property","valueType":"xs:string",'
        f'"value":"PUBLIC-TYPE-{suffix}"}},{{"idShort":"PartnerSerialNumber","modelType":"Property",'
        f'"valueType":"xs:string","value":"SECRET-{suffix}"}}]}}'
    )

    return {
        "suffix": suffix,
        "markers": [
            f"public={'+'.join(public_markers)}",
            f"restricted={'+'.join(restricted_markers)}",
        ],
        "posts": [
            ("shell", f"{dtr_base_url}/shell-descriptors", shell_json),
            ("publicSubmodel", f"{smrepo_base_url}/submodels", public_submodel_json),
            ("restrictedSubmodel", f"{smrepo_base_url}/submodels", restricted_submodel_json),
        ],
    }


def post_payload(kind, variant, markers, url, body, access_token, timeout):
    status, response_body = request_bytes(
        "POST",
        url,
        body,
        {
            "Authorization": f"Bearer {access_token}",
            "Content-Type": "application/json",
        },
        timeout,
    )
    return {
        "variant": variant,
        "kind": kind,
        "markers": markers,
        "status": status,
        "body": response_body,
    }


def record_result(variant_statuses, result):
    variant = result["variant"]
    status = variant_statuses.setdefault(
        variant,
        {
            "shell": None,
            "publicSubmodel": None,
            "restrictedSubmodel": None,
            "markers": result["markers"],
        },
    )
    status[result["kind"]] = result["status"]

    if status["shell"] is not None and status["publicSubmodel"] is not None and status["restrictedSubmodel"] is not None:
        markers = ", ".join(status["markers"])
        print(
            f"variant {variant}: shell={status['shell']} publicSubmodel={status['publicSubmodel']} "
            f"restrictedSubmodel={status['restrictedSubmodel']} markers=[{markers}]",
            flush=True,
        )
        del variant_statuses[variant]


def drain_one(pending, variant_statuses):
    done, pending = wait(pending, return_when=FIRST_COMPLETED)
    for future in done:
        record_result(variant_statuses, future.result())
    return pending


def main():
    global args
    args = parse_args()
    token_state = get_access_token(args)
    pending = set()
    variant_statuses = {}

    with ThreadPoolExecutor(max_workers=args.max_parallel) as executor:
        for index in range(args.start_index, args.start_index + args.count):
            token_state = ensure_token(args, token_state)
            variant = variant_payloads(index, args.run_id, args.dtr_base_url, args.smrepo_base_url)
            for kind, url, body in variant["posts"]:
                pending.add(
                    executor.submit(
                        post_payload,
                        kind,
                        variant["suffix"],
                        variant["markers"],
                        url,
                        body,
                        token_state["access_token"],
                        args.request_timeout,
                    )
                )
                if len(pending) >= args.max_parallel:
                    pending = drain_one(pending, variant_statuses)

        while pending:
            pending = drain_one(pending, variant_statuses)

    close_thread_connections()


if __name__ == "__main__":
    main()
