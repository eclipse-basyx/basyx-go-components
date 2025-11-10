import requests
import uuid
import copy
import json

# --- Config ---
URL = "http://localhost:5004/shell-descriptors"
JSON_FILE = "bodies/simple.json"  # path to your file

# --- Load JSON file ---
with open(JSON_FILE, "r", encoding="utf-8") as f:
    payload_template = json.load(f)

# --- POST loop ---
for i in range(90000):  # Change number if you want more inserts
    payload = copy.deepcopy(payload_template)

    # Unique Descriptor ID
    payload["id"] = f"https://example.org/aas/{uuid.uuid4()}"

    # Unique Submodel IDs
    for sm in payload.get("submodelDescriptors", []):
        sm["id"] = f"https://example.org/submodel/{uuid.uuid4()}"

    # Post it
    resp = requests.post(URL, json=payload)

    print(f"POST {i}: HTTP {resp.status_code}")
