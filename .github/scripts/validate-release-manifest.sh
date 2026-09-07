#!/usr/bin/env bash
# /*******************************************************************************
# * Copyright (C) 2026 the Eclipse BaSyx Authors and Fraunhofer IESE
# *
# * Permission is hereby granted, free of charge, to any person obtaining
# * a copy of this software and associated documentation files (the
# * "Software"), to deal in the Software without restriction, including
# * without limitation the rights to use, copy, modify, merge, publish,
# * distribute, sublicense, and/or sell copies of the Software, and to
# * permit persons to whom the Software is furnished to do so, subject to
# * the following conditions:
# *
# * The above copyright notice and this permission notice shall be
# * included in all copies or substantial portions of the Software.
# *
# * THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND,
# * EXPRESS OR IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF
# * MERCHANTABILITY, FITNESS FOR A PARTICULAR PURPOSE AND
# * NONINFRINGEMENT. IN NO EVENT SHALL THE AUTHORS OR COPYRIGHT HOLDERS BE
# * LIABLE FOR ANY CLAIM, DAMAGES OR OTHER LIABILITY, WHETHER IN AN ACTION
# * OF CONTRACT, TORT OR OTHERWISE, ARISING FROM, OUT OF OR IN CONNECTION
# * WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE SOFTWARE.
# *
# * SPDX-License-Identifier: MIT
# ******************************************************************************/

set -euo pipefail

manifest=${1:?"ERROR [DOCKER-RELEASE-VALIDATEMANIFEST] Manifest path is required."}
release_tag=${2:?"ERROR [DOCKER-RELEASE-VALIDATEMANIFEST] Release tag is required."}
version=${release_tag#v}
signature_identity="https://github.com/eclipse-basyx/basyx-go-components/.github/workflows/docker-release.yml@refs/heads/main"

expected_services='[
  {"service":"aasenvironmentservice","image":"eclipsebasyx/aasenvironment-go"},
  {"service":"aasregistryservice","image":"eclipsebasyx/aasregistry-go"},
  {"service":"discoveryservice","image":"eclipsebasyx/aasdiscovery-go"},
  {"service":"digitaltwinregistryservice","image":"eclipsebasyx/digitaltwinregistry-go"},
  {"service":"companylookupservice","image":"eclipsebasyx/companylookup-go"},
  {"service":"submodelrepositoryservice","image":"eclipsebasyx/submodelrepository-go"},
  {"service":"submodelregistryservice","image":"eclipsebasyx/submodelregistry-go"},
  {"service":"conceptdescriptionrepositoryservice","image":"eclipsebasyx/conceptdescriptionrepository-go"},
  {"service":"aasrepositoryservice","image":"eclipsebasyx/aasrepository-go"},
  {"service":"aasxfileserverservice","image":"eclipsebasyx/aasxfileserver-go"},
  {"service":"basyxconfigurationservice","image":"eclipsebasyx/basyxconfigurationservice-go"},
  {"service":"dppapiservice","image":"eclipsebasyx/dppapi-go"}
]'

if ! jq -e \
  --arg release "${release_tag}" \
  --arg version "${version}" \
  --arg identity "${signature_identity}" \
  --argjson expected "${expected_services}" \
  '
    type == "object" and
    (.release == $release) and
    (.generatedAt | type == "string" and length > 0) and
    (.services | type == "array" and length == ($expected | length)) and
    ((.services | map({service, image}) | sort_by(.service)) == ($expected | sort_by(.service))) and
    (all(.services[];
      (.digest | test("^sha256:[0-9a-f]{64}$")) and
      .version == $version and
      .tags == [(.image + ":" + $version)] and
      .signatureIdentity == $identity and
      (.provenancePredicateType == "https://slsa.dev/provenance/v1" or
       .provenancePredicateType == "https://slsa.dev/provenance/v0.2") and
      .sbomFiles.spdxJson == (.service + "-" + $version + ".spdx.json") and
      .sbomFiles.cyclonedxJson == (.service + "-" + $version + ".cdx.json")
    ))
  ' "${manifest}" >/dev/null; then
  echo "ERROR [DOCKER-RELEASE-VALIDATEMANIFEST] ${manifest} does not match the release manifest policy." >&2
  exit 1
fi
