import requests
import uuid
import copy  # <-- use deepcopy

URL = "http://127.0.0.1:5004/shell-descriptors"

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

session = requests.Session()  # reuse TCP connection for speed

for i in range(1000):
    payload = copy.deepcopy(payload_template)  # IMPORTANT: deepcopy for nested structures

    # Unique AAS ID
    payload["id"] = f"https://iese.fraunhofer.de/ids/aas/{uuid.uuid4()}"

    # Unique IDs for ALL submodel descriptors
    if "submodelDescriptors" in payload:
        for sm in payload["submodelDescriptors"]:
            sm["id"] = f"https://iese.fraunhofer.de/ids/submodel/{uuid.uuid4()}"

    response = session.post(URL, json=payload)

    if response.status_code in (200, 201):
        print(f"[{i+1}/1000] ✅ Success: AAS={payload['id']} | submodels={[sm['id'] for sm in payload['submodelDescriptors']]}")
    else:
        print(f"[{i+1}/1000] ❌ Failed ({response.status_code}): {response.text}")
