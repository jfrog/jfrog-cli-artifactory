---
name: publish-skill-v2
description: Publish a skill to Artifactory. Use when the user wants to publish, upload, push, or release a skill or package to Artifactory.
---

# Publish Skill to Artifactory (v2)

Zips a local skill folder and uploads it to an Artifactory skills repository, then attaches a signed publish-attestation as evidence using `jf evd create`.

## Agent behavior

- After identifying the skill and version, **state the plan** before proceeding:

  **Plan**
  1. **Resolve:** Confirm skill and version; check for existing versions in Artifactory.
  2. **Artifactory:** Zip and upload the skill to Artifactory.
  3. **Evidence:** Create and attach a signed publish-attestation predicate.

- In the **final report**, start the first line with **✅ Published: {slug} {version}** — path: `...`. Start each Done item with **✅**.
- When the source is a zip in `zip/`, say "from zip/..." — never say "pre-built zip".
- Do not mention `generated/` cleanup or deletion in the final report.

## Prerequisites

- **JFrog CLI** (v2.65.0+) installed and in `PATH`. Configure at least one server with `jf config add`.
- **Evidence signing keypair** (required for the Evidence phase). Generate an ECDSA, RSA, or Ed25519 keypair and upload the **public key** to the target Artifactory instance:

  ```bash
  openssl ecparam -genkey -name prime256v1 -noout -out evidence.key
  openssl ec -in evidence.key -pubout -o evidence.pub

  jf rt curl -XPOST "/api/security/keys/trusted" \
    -H "Content-Type: application/json" \
    -d '{"alias":"my-evd-key","public_key":"'"$(cat evidence.pub)"'"}' \
    --server-id <server-id>
  ```

  Store the private key path in `JFROG_CLI_SIGNING_KEY` (or `EVD_SIGNING_KEY_PATH`) and the alias in `EVD_KEY_ALIAS` in your `.env`. Without the public key on the instance, `jf evd create` will fail with "Subject not found".

## Phase 1: Resolve

### Load environment

```bash
[ -f .env ] && set -a && source .env && set +a
```

For evidence signing: `JFROG_CLI_SIGNING_KEY` or `EVD_SIGNING_KEY_PATH` (private key path), `EVD_KEY_ALIAS` (key alias).

### Check prerequisites

After loading `.env`, verify each prerequisite and assist the user if anything is missing.

**1. JFrog CLI:**

```bash
command -v jf &>/dev/null && jf --version
```

If missing, stop and tell the user to install it: `curl -fL https://install-cli.jfrog.io | sh`

**2. JFrog CLI server configured:**

```bash
jf config show 2>/dev/null
```

If no servers are configured, stop and tell the user to run `jf config add`.

**3. Skills repository:** Discover the skills repo automatically. If `RT_REPO_KEY` is set in `.env`, use it as an override. Otherwise, query Artifactory:

```bash
jf rt curl "/api/repositories?type=local&packageType=skills" --silent
```

- **One repo found:** Use it automatically.
- **Multiple repos:** Present them via **AskQuestion** -- "Which skills repo?"
- **None found:** Tell the user to create a skills repository in Artifactory.

**4. Evidence signing key:** Check that a private key is available and the key file exists:

```bash
key_path="${EVD_SIGNING_KEY_PATH:-${JFROG_CLI_SIGNING_KEY:-}}"
test -n "$key_path" && test -f "$key_path" && echo "key ok" || echo "key missing"
```

If the key is missing, offer to generate a new keypair:

```bash
mkdir -p keys/evd
openssl ecparam -genkey -name prime256v1 -noout -out keys/evd/evidence.key
openssl ec -in keys/evd/evidence.key -pubout -o keys/evd/evidence.pub
```

Then prompt the user to add `JFROG_CLI_SIGNING_KEY` and `EVD_KEY_ALIAS` to `.env`.

**5. `EVD_KEY_ALIAS`:** If unset, generate a default alias (e.g. `evd-key-$(date +%Y%m%d-%H%M%S)`) and offer it to the user via **AskQuestion**.

**6. Public key uploaded to Artifactory:** Check if the alias is registered on the target server:

```bash
jf rt curl "/api/security/keys/trusted" --silent | jq -r '.[].alias' | grep -q "^${EVD_KEY_ALIAS}$"
```

If the alias is not found, upload the public key automatically (deriving the `.pub` path from the private key path by replacing `.key` with `.pub`, or looking for a `.pub` next to the private key):

```bash
pub_path="${key_path%.key}.pub"
jf rt curl -XPOST "/api/security/keys/trusted" \
  -H "Content-Type: application/json" \
  -d '{"alias":"'"${EVD_KEY_ALIAS}"'","public_key":"'"$(cat "$pub_path")"'"}'
```

If the `.pub` file doesn't exist, ask the user to provide it.

If any prerequisite cannot be resolved, warn the user clearly before proceeding. Missing evidence keys do not block the publish (Phase 2 still runs); the agent skips Phase 3 with a warning.

### Identify skill and metadata

Determine the skill from the user's message. Two sources:

1. **Skill folder** — must contain a `SKILL.md` with YAML frontmatter (`name` for slug, `version`). If unclear, ask with **AskQuestion**.
2. **Zip file** — from `zip/{slug}_{version}.zip`. Derive slug and version from the filename.

Slug must match `^[a-z0-9][a-z0-9-]*$`. If missing, use the folder name. Version is required.

### Check for existing versions

```bash
jf rt curl "/api/skills/{repoKey}/api/v1/skills/{slug}/versions" --silent
```

Response: `{ "versions": [{ "version": "1.2.0", "createdAt": "..." }, ...] }`

- **No version provided:** Suggest the next logical version (patch/minor bump) via **AskQuestion**. Do not offer to overwrite.
- **Version exists:** Warn with **AskQuestion** — offer Overwrite, Use different version, or Cancel.
- **Version does not exist:** Proceed.
- **New skill (no versions):** Default to `1.0.0` if no version was provided.

## Phase 2: Artifactory

All generated files go under `generated/{slug}-{version}/` at the workspace root.

### Prepare the zip

Check for a zip in the workspace `zip/` folder:

```bash
test -f zip/{slug}_{version}.zip && echo "exists" || echo "missing"
```

**If zip exists:** Copy it as-is (do not unpack):

```bash
mkdir -p generated/{slug}-{version}
cp zip/{slug}_{version}.zip generated/{slug}-{version}/{slug}-{version}.zip
```

**If zip is missing:** Build from the skill folder:

```bash
mkdir -p generated/{slug}-{version}/build
cp -R {skill_folder}/* generated/{slug}-{version}/build/
rm -rf generated/{slug}-{version}/build/.* generated/{slug}-{version}/build/__pycache__ 2>/dev/null || true
cd generated/{slug}-{version}/build && zip -r ../{slug}-{version}.zip . -x ".*" "__pycache__/*" "*.pyc"
```

### Upload to Artifactory

Compute SHA256 (needed for evidence), then upload with `--flat`:

```bash
sha256=$(shasum -a 256 generated/{slug}-{version}/{slug}-{version}.zip | awk '{print $1}')

jf rt upload generated/{slug}-{version}/{slug}-{version}.zip "{repoKey}/{slug}/{version}/" --flat
```

The Artifactory path is `{repoKey}/{slug}/{version}/{slug}-{version}.zip`. Add `--server-id <id>` when targeting a specific server.

If the upload fails, stop and alert the user.

## Phase 3: Evidence

Generate a publish-attestation predicate and attach it to the uploaded artifact.

### Generate predicate and markdown

```bash
publishedAt=$(date -u +%Y-%m-%dT%H:%M:%SZ)

jq -n --arg slug "{slug}" --arg version "{version}" --arg at "$publishedAt" \
  '{skill: $slug, version: $version, publishedAt: $at}' \
  > generated/{slug}-{version}/predicate.json

jq -r '"# Publish Attestation\n\n| Field | Value |\n|-------|-------|\n| Skill | \(.skill) |\n| Version | \(.version) |\n| Published at | \(.publishedAt) |"' \
  generated/{slug}-{version}/predicate.json \
  > generated/{slug}-{version}/{slug}-{version}-attestation.md
```

### Attach evidence

Pass `--key` from `EVD_SIGNING_KEY_PATH` if set (otherwise the CLI uses `JFROG_CLI_SIGNING_KEY`). Always pass `--key-alias` when `EVD_KEY_ALIAS` is set.

```bash
jf evd create \
  --subject-repo-path "{repoKey}/{slug}/{version}/{slug}-{version}.zip" \
  --subject-sha256 "${sha256}" \
  --predicate generated/{slug}-{version}/predicate.json \
  --predicate-type "https://jfrog.com/evidence/publish-attestation/v1" \
  --markdown generated/{slug}-{version}/{slug}-{version}-attestation.md \
  ${EVD_SIGNING_KEY_PATH:+--key "${EVD_SIGNING_KEY_PATH}"} \
  ${EVD_KEY_ALIAS:+--key-alias} ${EVD_KEY_ALIAS:+$EVD_KEY_ALIAS}
```

If the subject "does not exist", verify the filename uses a hyphen (`{slug}-{version}.zip`) and that the upload succeeded. If evidence fails and no signing key was configured, warn the user but do not block the publish.

## Report

If the skill folder flow was used, clean the staging dir: `rm -rf generated/{slug}-{version}/build`. Verify the upload:

```bash
jf rt search "{repoKey}/{slug}/{version}/"
```

**Final report format:**

1. **✅ Published: {slug} {version}** — path: `{repoKey}/{slug}/{version}/{slug}-{version}.zip`
2. **Done** heading, then exactly three items:
   - **1. Resolve:** ✅ Skill and version confirmed; no conflicts (or overwrite confirmed).
   - **2. Artifactory:** ✅ Zip uploaded to the path above.
   - **3. Evidence:** ✅ Publish-attestation predicate attached and signed. (If skipped/failed, say so without ✅.)
