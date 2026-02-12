import requests
import uuid
import copy
import json

# --- Config ---
URL = "http://localhost:5004/shell-descriptors"
JSON_FILE = "bodies/data.json"  # path to your file
INSERT_COUNT = 30000  # Change number if you want more inserts
INSERT_LIST_ONCE = False  # If JSON file is a list, insert each element once

# --- Load JSON file ---
with open(JSON_FILE, "r", encoding="utf-8") as f:
    payload_template = json.load(f)

# Allow the JSON file to contain a single object or a list of objects.
if isinstance(payload_template, list):
    payload_templates = payload_template
else:
    payload_templates = [payload_template]


# --- POST loop ---
if INSERT_LIST_ONCE and len(payload_templates) > 1:
    total_inserts = len(payload_templates)
else:
    total_inserts = INSERT_COUNT

for i in range(total_inserts):
    template = payload_templates[i % len(payload_templates)]
    payload = copy.deepcopy(template)

    # Unique Descriptor ID
    payload["id"] = f"https://example.org/aas/{uuid.uuid4()}"

    # Unique Submodel IDs
    for sm in payload.get("submodelDescriptors", []):
        sm["id"] = f"https://example.org/submodel/{uuid.uuid4()}"
        sm.pop("createdAt", None)

    # Remove createdAt fields before POST
    payload.pop("createdAt", None)


    # Post it
    resp = requests.post(URL, json=payload)

    print(f"POST {i}: HTTP {resp.status_code}")
