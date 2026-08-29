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

changie_version="1.26.0"
archive="changie_${changie_version}_linux_amd64.tar.gz"
checksum="eab168c8287a6e91912e1c02e5260911232d945bfd3c89d8a0e1ace6bb7b6161"
download_url="https://github.com/miniscruff/changie/releases/download/v${changie_version}/${archive}"
temporary_directory=$(mktemp -d)
trap 'rm -rf "${temporary_directory}"' EXIT

curl --fail --silent --show-error --location \
  --output "${temporary_directory}/${archive}" \
  "${download_url}"
if ! printf '%s  %s\n' "${checksum}" "${temporary_directory}/${archive}" | sha256sum -c - >/dev/null; then
  echo "ERROR [CHLOG-INSTALL-VERIFYCHECKSUM] Changie archive checksum verification failed." >&2
  exit 1
fi

tar --extract --gzip --file "${temporary_directory}/${archive}" --directory "${temporary_directory}" changie
install_directory="${RUNNER_TEMP}/changie-${changie_version}/bin"
mkdir -p "${install_directory}"
install -m 0755 "${temporary_directory}/changie" "${install_directory}/changie"
echo "${install_directory}" >> "${GITHUB_PATH}"
