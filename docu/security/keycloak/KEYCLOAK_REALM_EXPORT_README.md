# Exporting Keycloak Realm Configuration

This guide explains how to export the Keycloak realm configuration from a running Keycloak container and copy it to your host machine.

## Overview

Recommended path from the repository root:

```bash
bash docu/security/keycloak/export_keycloak_realm.sh
```

The script supports Docker and Podman, auto-detects a single running Keycloak container, and writes the export to `./export` by default. Use `--runtime`, `--container`, and `--output` when auto-detection is not enough:

```bash
bash docu/security/keycloak/export_keycloak_realm.sh --runtime podman --container keycloak --output ./realm-export
```

Manual fallback:

1. Find the Keycloak container ID.
2. Execute commands **inside** the Keycloak container to authenticate and export the realm config.
3. Copy the exported files **from the container to your host** using `docker cp`.

---

## 1. Find the Keycloak container ID

On your host machine, list running containers and locate the Keycloak container:

```bash
docker ps
```

Note the **CONTAINER ID** (or name) of your Keycloak container, for example:

```text
CONTAINER ID   IMAGE              COMMAND        ...
60303f841b34   quay.io/keycloak   ...
```

In this example, the container ID is `60303f841b34`.

---

## 2. Open a shell inside the Keycloak container

Still on your host machine, start a shell in the Keycloak container (replace `<container_id>` with your actual ID or name):

```bash
docker exec -it <container_id> /bin/bash
```

You are now **inside** the Keycloak container.

---

## 3. Authenticate the admin CLI inside the container

Inside the container, run the following command to configure the Keycloak admin CLI (`kcadm.sh`) credentials:

```bash
~/bin/kcadm.sh config credentials --server http://localhost:8080 --realm master --user admin --password admin
```

- `--server` points to the Keycloak server URL **inside** the container.
- `--realm master` is the realm used for admin authentication.
- `--user` / `--password` are your admin credentials (adjust if they differ).

If this succeeds, `kcadm.sh` is now authenticated and can perform admin operations.

---

## 4. Export the realm configuration inside the container

Still inside the container, run:

```bash
~/bin/kc.sh export --dir /tmp/export --users realm_file
```

This will:

- Export the realm configuration (and users) to the directory `/tmp/export` **inside the container**.
- Create JSON files representing your realms and users.

> Note: You can adjust the export options as needed (e.g., to target a specific realm), but the above matches the given command.

After this step, your realm export exists only inside the container at `/tmp/export`.

You can now exit the container shell:

```bash
exit
```

---

## 5. Copy the exported realm config to your host

On your **host machine**, go to the directory where you want to store the exported realm config:

```bash
cd /path/where/you/want/the/export
```

Then copy the export folder from the container to the current directory:

```bash
docker cp <container_id>:/tmp/export .
```

- Replace `<container_id>` with the actual ID or name, e.g. `60303f841b34`.
- The `.` means “copy to the current directory”.

After this command finishes, you should see an `export/` directory (or similar) in your current folder, containing the exported Keycloak realm configuration.

---

## Result

You now have your Keycloak realm configuration exported from the container and available on your host machine. You can:

- Check the JSON files into version control.
- Use them for backups or migration to another Keycloak instance.
- Inspect or modify the configuration as needed.

---

## Keep committed realm imports minimal

The realm files under `examples/`, `sql_examples/` and the integration tests are not full exports. Keycloak creates everything that has a default when it imports a realm: built-in clients, client scopes, authentication flows, required actions, default roles and signing keys. A committed realm file therefore only contains what differs from those defaults:

- realm settings that differ from the defaults, such as `sslRequired`
- custom realm and client roles and groups
- custom clients with their protocol mappers, secrets and authorization settings
- users with a stable `id`, attributes, role and group mappings and a plain-text demo password
- the declarative user profile component when custom user attributes such as `role` or `clear` are used

Before committing a new export, reduce it to these sections and drop ids, timestamps, hashed credentials and key providers. Two Keycloak import rules matter here:

- Do not add a partial `clientScopes` list. When the list is present, Keycloak skips creating the built-in client scopes such as `profile`, `email` and `roles`. Add claims through client protocol mappers instead.
- Keep `frontchannelLogout` explicit on clients. The import default is `false`, while clients created in the admin console default to `true`.

Minimal realm files also pick up the defaults of newer Keycloak versions instead of pinning the defaults of the version that produced the export.
