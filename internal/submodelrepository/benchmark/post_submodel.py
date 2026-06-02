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

import copy
import json
import random
import uuid
import requests


URL = "http://localhost:5004/submodels"


payload = {
    "id": "YERPENDERPENDERP",
    "idShort": "Identification",
    "kind": "Instance",
    "modelType": "Submodel",
    "semanticId": {
        "keys": [
            {
                "type": "Submodel",
                "value": "http://acplt.org/SubmodelTemplates/AssetIdentification"
            },
            {
                "type": "Submodel",
                "value": "http://acplt.org/SubmodelTemplates/AssetIdentification"
            }
        ],
        "type": "ExternalReference"
    },
    "submodelElements": [
        {
            "category": "VARIABLE",
            "idShort": "InstanceId",
            "modelType": "Property",
            "semanticId": {
                "keys": [
                    {
                        "type": "GlobalReference",
                        "value": "http://opcfoundation.org/UA/DI/1.1/DeviceType/Serialnumber"
                    }
                ],
                "type": "ExternalReference"
            },
            "supplementalSemanticIds": [
                {
                    "keys": [
                        {
                            "type": "GlobalReference",
                            "value": "something_random_e14ad770"
                        }
                    ],
                    "type": "ExternalReference"
                },
                {
                    "keys": [
                        {
                            "type": "GlobalReference",
                            "value": "something_random_bd061acd"
                        }
                    ],
                    "type": "ExternalReference"
                }
            ],
            "value": "978-8234-234-342",
            "valueId": {
                "keys": [
                    {
                        "type": "GlobalReference",
                        "value": "978-8234-234-342"
                    }
                ],
                "type": "ExternalReference"
            },
            "valueType": "xs:string"
        },
        {
            "description": [
                {
                    "language": "en-us",
                    "text": "Legally valid designation of the natural or judicial person which is directly responsible for the design, production, packaging and labeling of a product in respect to its being brought into circulation."
                },
                {
                    "language": "de",
                    "text": "Bezeichnung fuer eine natuerliche oder juristische Person, die fuer die Auslegung, Herstellung und Verpackung sowie die Etikettierung eines Produkts im Hinblick auf das 'Inverkehrbringen' im eigenen Namen verantwortlich ist"
                }
            ],
            "displayName": [
                {
                    "language": "en-us",
                    "text": "Manufacturer Name"
                }
            ],
            "idShort": "ManufacturerName",
            "modelType": "Property",
            "qualifiers": [
                {
                    "kind": "ConceptQualifier",
                    "type": "http://acplt.org/Qualifier/ExampleQualifier",
                    "value": "100",
                    "valueType": "xs:int"
                },
                {
                    "kind": "ConceptQualifier",
                    "type": "http://acplt.org/Qualifier/ExampleQualifier2",
                    "value": "50",
                    "valueType": "xs:int"
                }
            ],
            "semanticId": {
                "keys": [
                    {
                        "type": "GlobalReference",
                        "value": "0173-1#02-AAO677#002"
                    }
                ],
                "type": "ExternalReference"
            },
            "value": "http://acplt.org/ValueId/ACPLT",
            "valueId": {
                "keys": [
                    {
                        "type": "GlobalReference",
                        "value": "http://acplt.org/ValueId/ACPLT"
                    }
                ],
                "type": "ExternalReference"
            },
            "valueType": "xs:string"
        },
        {
            "category": "PARAMETER",
            "description": [
                {
                    "language": "en-us",
                    "text": "New Test Collection"
                },
                {
                    "language": "de",
                    "text": "Neue Test Sammlung"
                }
            ],
            "idShort": "NewTestCollection",
            "modelType": "SubmodelElementCollection",
            "semanticId": {
                "keys": [
                    {
                        "type": "GlobalReference",
                        "value": "http://acplt.org/SubmodelElementCollections/NewTestCollection"
                    }
                ],
                "type": "ExternalReference"
            },
            "value": [
                {
                    "category": "CONSTANT",
                    "idShort": "CollectionProperty",
                    "modelType": "Property",
                    "value": "collectionValue",
                    "valueType": "xs:string"
                },
                {
                    "category": "CONSTANT",
                    "idShort": "CollectionProperty2",
                    "modelType": "Property",
                    "value": "collectionValue",
                    "valueType": "xs:string"
                }
            ]
        },
        {
            "category": "PARAMETER",
            "description": [
                {
                    "language": "en-us",
                    "text": "New Test List"
                },
                {
                    "language": "de",
                    "text": "Neue Test Liste"
                }
            ],
            "idShort": "NewTestList",
            "modelType": "SubmodelElementList",
            "orderRelevant": True,
            "semanticId": {
                "keys": [
                    {
                        "type": "GlobalReference",
                        "value": "http://acplt.org/SubmodelElementLists/NewTestList"
                    }
                ],
                "type": "ExternalReference"
            },
            "typeValueListElement": "SubmodelElement",
            "value": [
                {
                    "category": "CONSTANT",
                    "idShort": "ListProperty1",
                    "modelType": "Property",
                    "value": "listValue1",
                    "valueType": "xs:string"
                },
                {
                    "category": "CONSTANT",
                    "idShort": "ListProperty2",
                    "modelType": "Property",
                    "value": "2",
                    "valueType": "xs:double"
                }
            ]
        }
    ]
}


def add_random_to_value(value):
    number = random.randint(1, 10)

    if isinstance(value, int):
        return value + number

    if isinstance(value, float):
        return value + number

    if isinstance(value, str):
        try:
            if "." in value:
                return str(float(value) + number)
            return str(int(value) + number)
        except ValueError:
            return f"{value}_{number}"

    return value


def modify_all_values(obj):
    if isinstance(obj, dict):
        for key, val in obj.items():
            if key == "value":
                if isinstance(val, list):
                    modify_all_values(val)
                else:
                    obj[key] = add_random_to_value(val)
            else:
                modify_all_values(val)

    elif isinstance(obj, list):
        for item in obj:
            modify_all_values(item)


def create_payload():
    data = copy.deepcopy(payload)

    # Always change the top-level id
    data["id"] = f"submodel-{uuid.uuid4()}"

    # Add/append random numbers to all fields named "value"
    modify_all_values(data)

    return data


if __name__ == "__main__":
    for i in range(1):
        data = create_payload()

        response = requests.post(
            URL,
            json=data,
            headers={"Content-Type": "application/json"}
        )

        print("Status code:", response.status_code)
        print("Response:", response.text)
