#!/usr/bin/env bash
#
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

set -euo pipefail

prepare_data_directory() {
  mkdir -p "${PGDATA}"
  chown -R postgres:postgres "${PGDATA}"
  find "${PGDATA}" -mindepth 1 -maxdepth 1 -exec rm -rf -- {} +
}

clone_primary() {
  local connection_string
  connection_string="host=${PRIMARY_HOST} port=${PRIMARY_PORT} user=${REPLICATION_USER} password=${REPLICATION_PASSWORD}"

  for attempt in $(seq 1 30); do
    prepare_data_directory
    if gosu postgres pg_basebackup \
      --dbname="${connection_string}" \
      --pgdata="${PGDATA}" \
      --checkpoint=fast \
      --wal-method=stream \
      --write-recovery-conf; then
      chmod 0700 "${PGDATA}"
      return 0
    fi
    echo "Primary clone attempt ${attempt} failed; retrying" >&2
    sleep 2
  done

  echo "Unable to clone PostgreSQL primary" >&2
  return 1
}

if [[ ! -s "${PGDATA}/PG_VERSION" ]]; then
  clone_primary
fi

exec docker-entrypoint.sh postgres -c hot_standby=on
