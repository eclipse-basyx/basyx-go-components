import requests
import uuid
import copy

URL = "http://localhost:5004/shell-descriptors"

# ---- Your Full Template (unchanged except IDs updated later) ----
payload_template = {
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
                "href": "https://demo3.digital-twin.host/aas-environment/...",
                "endpointProtocol": "https"
            }
        }
    ],
    "globalAssetId": "https://iese.fraunhofer.de/ids/asset/5079_8944_8914_9414",
    "idShort": "machine",
    "id": "https://iese.fraunhofer.de/ids/aas/sdfgbfdb",
    "extensions": [
        {
            "name": "help",
            "valueType": "xs:string",
            "value": "test"
        }
    ],
    "submodelDescriptors": [
        {
            "id": "https://iese.fraunhofer.de/ids/aas/yxcyxc",
            "endpoints": [
                {
                    "interface": "AAS-3.0",
                    "protocolInformation": {
                        "href": "https://demo3.digital-twin.host/aas-environment/...",
                        "endpointProtocol": "https"
                    }
                }
            ]
        }
    ]
}

# ---- POST loop ----
for i in range(1000):  # Change number if you want more inserts
    payload = copy.deepcopy(payload_template)

    # Unique Descriptor ID
    payload["id"] = f"https://example.org/aas/{uuid.uuid4()}"

    # Unique Submodel IDs
    for sm in payload.get("submodelDescriptors", []):
        sm["id"] = f"https://example.org/submodel/{uuid.uuid4()}"

    # Post it
    resp = requests.post(URL, json=payload)

    print(f"POST {i}: HTTP {resp.status_code}")
