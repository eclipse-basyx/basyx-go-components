#!/bin/bash

# Wait for the Company Lookup to be healthy
echo "Waiting for Company Lookup to be ready..."
max_attempts=30
attempt=0

while [ $attempt -lt $max_attempts ]; do
    if curl -s -f http://localhost:5080/health > /dev/null 2>&1; then
        echo "Company Lookup is healthy!"
        break
    fi
    attempt=$((attempt + 1))
    echo "Attempt $attempt/$max_attempts - Service not ready yet, waiting..."
    sleep 2
done

if [ $attempt -eq $max_attempts ]; then
    echo "Error: Company Lookup did not become healthy in time"
    exit 1
fi

# Give it a moment to fully initialize
sleep 2

echo "Pre-filling Company Lookup with sample data..."

# Add Infrastructure descriptor
echo "Adding Fraunhofer IESE..."
curl --location --request POST 'http://localhost:5080/companies' \
--header 'Content-Type: application/json' \
--data '{
    "description": [
        {
            "language": "en",
            "text": "The Fraunhofer Institute for Experimental Software Engineering IESE in Kaiserslautern is a research institute specializing in efficient software and systems engineering."
        }
    ],
    "displayName": [
        {
            "language": "en",
            "text": "Fraunhofer Institute for Experimental Software Engineering IESE"
        }
    ],
    "administration": {
        "version": "1",
        "revision": "0",
        "creator": {
            "type": "ExternalReference",
            "keys": [
                {
                    "type": "GlobalReference",
                    "value": "https://iese.fraunhofer.de"
                }
            ]
        }
    },
    "endpoints": [
        {
            "protocolInformation": {
                "href": "https://demo.digital-twin.host/aas-registry",
                "endpointProtocol": "HTTPS"
            },
            "interface": "AAS-REGISTRY-3.0"
        },
        {
            "protocolInformation": {
                "href": "https://demo.digital-twin.host/aas-discovery",
                "endpointProtocol": "HTTPS"
            },
            "interface": "AAS-DISCOVERY-3.0"
        },
        {
            "protocolInformation": {
                "href": "https://demo.digital-twin.host/aas-repository",
                "endpointProtocol": "HTTPS"
            },
            "interface": "AAS-REPOSITORY-3.0"
        },
        {
            "protocolInformation": {
                "href": "https://demo.digital-twin.host/submodel-registry",
                "endpointProtocol": "HTTPS"
            },
            "interface": "SUBMODEL-REGISTRY-3.0"
        },
        {
            "protocolInformation": {
                "href": "https://demo.digital-twin.host/submodel-repository",
                "endpointProtocol": "HTTPS"
            },
            "interface": "SUBMODEL-REPOSITORY-3.0"
        },
        {
            "protocolInformation": {
                "href": "https://demo.digital-twin.host/aasx-file",
                "endpointProtocol": "HTTPS"
            },
            "interface": "AASX-FILE-3.0"
        }
    ],
    "idShort": "fraunhoferIese",
    "name": "Fraunhofer IESE",
    "domain": "iese.fraunhofer.de",
    "nameOptions": [
        "Fraunhofer-Institut für Experimentelles Software-Engineering",
        "IESE",
        "IESE Fraunhofer"
    ],
    "assetIdRegexPatterns": [
        "^https://i\\.iese\\.fraunhofer\\.de/assetId/",
        "^https://i\\.fraun-iese\\.de/assetId/"
    ]
}'

echo ""
echo "Adding Example Company..."
curl --location --request POST 'http://localhost:5080/companies' \
--header 'Content-Type: application/json' \
--data '{
    "description": [
        {
            "language": "en",
            "text": "An example company."
        }
    ],
    "displayName": [
        {
            "language": "en",
            "text": "Example Company"
        }
    ],
    "administration": {
        "version": "1",
        "revision": "0",
        "creator": {
            "type": "ExternalReference",
            "keys": [
                {
                    "type": "GlobalReference",
                    "value": "https://examplecompany.de"
                }
            ]
        }
    },
    "endpoints": [
        {
            "protocolInformation": {
                "href": "https://demo.digital-twin.host/aas-discovery",
                "endpointProtocol": "HTTPS"
            },
            "interface": "AAS-DISCOVERY-3.0"
        }
    ],
    "idShort": "exampleCompany1",
    "name": "ExCom1",
    "domain": "ex.com.1.de",
    "nameOptions": [
        "Example Company 1",
        "EC1",
        "Com Example 1"
    ],
    "assetIdRegexPatterns": [
        "^https://i\\.excom1\\.de/assetId/"
    ]
}'

echo ""
echo "All sample data has been successfully added to Company Lookup!"
echo "You can access the service at: http://localhost:5080"
