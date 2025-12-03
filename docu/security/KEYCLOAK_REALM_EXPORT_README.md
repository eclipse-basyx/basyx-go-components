# Exporting Keycloak Realm Configuration

This guide explains how to export the Keycloak realm configuration from a running Keycloak container and copy it to your host machine.

## Overview

You will:

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
