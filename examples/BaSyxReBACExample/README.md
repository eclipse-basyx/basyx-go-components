# BaSyx ReBAC Example (experimental)

This example shows how people share their own Asset Administration Shells
and Submodels with others in the BaSyx UI. It uses the experimental
**relationship-based access control (ReBAC)** of BaSyx Go.

With ReBAC, access follows relationships between people and resources:

- Whoever creates a shell or Submodel becomes its **owner**.
- Owners share their resources with **people or groups** as viewer, editor,
  executor or owner, or send them an **invitation link**.
- A central access policy (ABAC) is still in place, but nobody has to edit it
  for everyday sharing.

In about 15 minutes you will see all of this with a few example users. The
full reference is [docu/security/REBAC.md](../../docu/security/REBAC.md).

## Start the example

You need Docker with Docker Compose.

```bash
docker compose up -d
```

Then open the BaSyx UI at <http://localhost:3000>.

> **UI version.** Sharing needs a BaSyx UI with ReBAC support. Until
> [eclipse-basyx/basyx-aas-web-ui#1552](https://github.com/eclipse-basyx/basyx-aas-web-ui/pull/1552)
> is released, build the UI image yourself and start the example without
> pulling images:
>
> ```bash
> git clone -b feat/rebac https://github.com/FriedJannik/basyx-aas-web-ui.git
> docker build -t eclipsebasyx/aas-gui:SNAPSHOT basyx-aas-web-ui/aas-web-ui
> docker compose up -d --pull never
> ```

Sign-in runs through Keycloak at `http://keycloak.localhost:8080`. Most
browsers resolve `*.localhost` on their own. If yours does not (for example
Safari), add `127.0.0.1 keycloak.localhost` to your hosts file.

To start over with an empty database, run `docker compose down -v` and start
the example again.

## Users

All users have the password **`pwd`**. To sign in, click the lock icon at the
top right and choose **Login**.

| User | User ID | Role in this example |
| --- | --- | --- |
| `dave` | `2b42008b-5971-51bc-af2e-079a30b151ab` | ReBAC administrator (group `operators`). Decides who may create resources. |
| `alice` | `738ddab6-7bad-570c-ace9-9ad95213a92b` | Creates shells and Submodels and shares them. |
| `bob` | `054378f6-9a34-5c9c-bec1-77dd7676cc82` | Receives an invitation link. |
| `carol` | `29093c15-47b4-585c-9c55-d0564000b953` | Member of the group `engineering`. |
| `eve` | `53dfe593-99ef-5537-a22d-91e1cc0b7505` | Has no access at all. |
| `admin` | – | Full access through the ABAC policy. Not needed for the walkthrough. |

People are shared with by their **user ID**, the `sub` claim of their token.
Everyone finds their own ID in the user menu under **User ID**, with buttons
to show and copy it.

## Roles

| Role | What it allows |
| --- | --- |
| Viewer | View the resource. |
| Editor | View and edit the resource and add elements. |
| Executor | Run operations and read their results (shells, Submodels and elements only). |
| Owner | Everything above, plus deleting the resource and managing its access. |

Two more roles apply to a whole repository, for example to all Submodels:

| Repository role | What it allows |
| --- | --- |
| Creator | Create new resources of that kind. The creator becomes their owner. |
| Administrator | Full access to every resource of that kind and to the repository access. |

## Walkthrough

### 1. Let alice create resources (as `dave`)

Nobody can create shells or Submodels yet, except the ABAC `admin`. `dave`,
the ReBAC administrator, changes that.

1. Sign in as `dave`.
2. Open the main menu at the top (the button showing the current page, for
   example *AAS Viewer*), switch to the tab **Modules** and choose
   **Access Management**.
3. Open the tab **Repositories** and select the repository
   **Asset Administration Shells**.
4. Under *Add a creator or administrator*, enter alice's user ID
   `738ddab6-7bad-570c-ace9-9ad95213a92b`, keep the role **Creator** and
   click **Add**.
5. Select the repository **Submodels** and add alice as **Creator** again.
6. Sign out (user menu → **Logout**).

### 2. Create a shell with a Submodel (as `alice`)

1. Sign in as `alice` and choose **AAS Editor** in the main menu (tab
   **AAS**).
2. In the menu (**⋮**) above the AAS list, choose **Create AAS**. Enter for
   example the ID `urn:example:aas:compressor`, the IdShort `Compressor` and a
   *Global Asset Id*, then click **Save**.
3. With the new shell selected, click **Create Submodel** in the Submodel
   tree. Enter for example the ID `urn:example:sm:compressor-status` and the
   IdShort `Status`, then click **Save**. Add a property or two if you like.

alice is now the owner of both. Choose **Share** in the menu (**⋮**) of the
shell in the AAS list to see it: *Your access* lists everything alice may
do, and alice appears as **Owner** with the chip *You*.

### 3. Share the shell with a group (as `alice`)

1. In the Share dialog of the shell, stay on **People and groups**.
2. Switch from **Person** to **Group** and enter `engineering`.
3. Keep the role **Viewer** and click **Share**.

Every member of `engineering`, here `carol`, can now see the shell, but not
its Submodel yet. **Access is granted per resource.** Sharing a shell does
not automatically share the Submodels it references.

### 4. Pass the shell's access on to its Submodel (as `alice`)

1. In the Submodel tree, open the menu (**⋮**) of the Submodel `Status` and
   choose **Share**.
2. Open the tab **Shell links** and click
   **Link the selected shell urn:example:aas:compressor**.

Now everyone who may view, edit or run operations of the shell may do the
same with this Submodel. Owners of the shell do not become owners of the
Submodel. A link only counts as long as the shell references the Submodel.

To check, sign in as `carol` (for example in a private browser window).
carol sees the shell and its Submodel, but cannot edit them.

### 5. Invite someone with a link (as `alice`)

1. Open the Share dialog of the Submodel `Status` again and choose the tab
   **Invitation links**.
2. Select the role **Editor**, keep *Valid for* **1 day** and *Uses* **1**,
   and click **Create link**. To restrict the link to a single person, switch
   on **Only for one person** and enter their user ID.
3. Copy the link. It is shown only once. The list *Open invitations* shows
   how often a link was used and lets you revoke it.

Open the link in a private browser window:

1. Click **Sign in** and sign in as `bob`.
2. Click **Accept invitation**. The page confirms that bob is now editor of
   the Submodel.
3. Click **Open** to go directly to the Submodel. bob can view and edit it,
   but he does not see the shell, because only the Submodel was shared with
   him.

Back in alice's Share dialog, **People and groups** now lists bob as
**Editor**. An accepted invitation becomes a normal grant.

### 6. Share a single element (optional, as `alice`)

Choose **Share** in the menu of any element in the Submodel tree, for
example a property. People you add there see only this element and its
children, not the rest of the Submodel. The element does not appear in their
lists, so an invitation link is the easiest way to share it: **Open** on the
invitation page goes directly to the element.

### 7. Change and revoke access (as `alice`)

In **People and groups**, change a role with its drop-down or remove access
with the trash icon. Removing bob asks for confirmation and takes effect
right away; bob gets no access on his next request.

A Submodel always keeps at least one owner. Removing the last owner is
rejected.

### 8. People without access (as `eve`)

Sign in as `eve`. The AAS list and all other lists are empty. eve sees no
errors and cannot tell which resources exist.

### 9. Review all access changes (as `dave`)

1. Sign in as `dave` and open **Access Management** (main menu, tab
   **Modules**) → **Audit trail**.
2. The table lists every access change: grants, shell links, invitations
   and repository grants, each with time, object and actor.
3. Click **Verify** to check that no entry was changed or removed. The
   entries are chained with hashes, so the result *The audit trail is
   intact* confirms the history.

## How the policy and sharing work together

- The central ABAC policy
  ([security_env/access-rules.json](security_env/access-rules.json)) still
  applies. A user gets access if **either** the policy **or** a ReBAC
  relationship allows it. `admin` has full access through the policy alone.
- The policy lets every signed-in user call the list endpoints, but its list
  rules match no resource on their own. Lists therefore show exactly what
  was shared with the user, and users like `eve` get empty lists instead of
  errors.
- A grant on a resource is not limited by ABAC filters of that resource.
  If the policy hides an element of a Submodel, a person with whom the
  Submodel is shared still sees it. Keep sensitive content in separate
  Submodels.
- Shell and Submodel descriptors in the registries and discovery entries
  are created automatically for new shells and Submodels. They follow the
  access of their shell or Submodel, so sharing a shell also makes it
  findable.
- `dave` is a ReBAC administrator because the group `operators` is listed in
  `REBAC_ADMINISTRATORS` in [docker-compose.yml](docker-compose.yml).

## What runs in this example

| Service | Purpose |
| --- | --- |
| `aas-ui` | BaSyx UI on <http://localhost:3000>, configured by [basyx-infra.yml](basyx-infra.yml) |
| `aas-environment` | BaSyx AAS Environment on <http://localhost:8082> with repositories, registries and discovery, ABAC and ReBAC enabled |
| `keycloak` | Identity provider on <http://keycloak.localhost:8080> (realm `basyx`) |
| `db` | PostgreSQL. The ReBAC relationships are stored here, next to the data. |
| `basyx_configuration` | Applies the BaSyx database schema at startup |

## Using the API directly

The UI uses the `$access` endpoints of each resource. You can call them
directly, for example to automate sharing:

```bash
token() {
  curl -s -d "grant_type=password&client_id=basyx-ui&username=$1&password=pwd" \
    http://keycloak.localhost:8080/realms/basyx/protocol/openid-connect/token |
    python3 -c 'import json,sys; print(json.load(sys.stdin)["access_token"])'
}
ALICE=$(token alice)

# Grants and access revision (ETag) of a Submodel
curl -i -H "Authorization: Bearer $ALICE" \
  "http://localhost:8082/submodels/<base64url id>/\$access"

# Replace the grants; If-Match must carry the current ETag
curl -X PUT -H "Authorization: Bearer $ALICE" -H 'If-Match: "1"' \
  -H 'Content-Type: application/json' \
  "http://localhost:8082/submodels/<base64url id>/\$access/grants" \
  -d '{"grants":[
        {"relation":"owner","subjectType":"user","issuer":"http://keycloak.localhost:8080/realms/basyx","subject":"738ddab6-7bad-570c-ace9-9ad95213a92b"},
        {"relation":"viewer","subjectType":"group","issuer":"http://keycloak.localhost:8080/realms/basyx","subject":"engineering"}]}'
```

[smoke.sh](smoke.sh) runs a short scripted version of the walkthrough
against the API and is a quick way to check that the example works:

```bash
./smoke.sh
```

See [docu/security/REBAC.md](../../docu/security/REBAC.md) for all
endpoints, the evaluation order and the configuration options.

## Notes

- ReBAC is experimental. Its API and behavior may still change.
- All BaSyx services that share one PostgreSQL database share the same
  relationships. Enable ReBAC consistently in all of them.
- The Keycloak realm, users and passwords of this example are for
  demonstration only.
