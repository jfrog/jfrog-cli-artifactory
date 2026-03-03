---
name: publish-skill
description: Publish a skill to Artifactory. Use when the user wants to publish, upload, push, or release a skill or package to Artifactory or an OpenClaw repository.
---

# Publish OpenClaw Skill to Artifactory

Zips a local skill folder and uploads it to an Artifactory OpenClaw repository.

**Chat wording:** In Cursor chat, say **skill-scanning** (not "JPD verification") and do not mention "pre-built zip"; say "zip" or "from zip/..." when the source is a zip file.

**Emoji in chat (required):** Always include the literal characters in your reply so the user sees them. On **skill-scanning success** (Step 8), when `scan_result` is `safe`, you **must** write a line in the chat containing the **exact** phrase **🟢 Skill-scanning complete — result: SAFE.** (use the green circle emoji 🟢; do not paraphrase, omit, or only mention it in tool output — the user must see 🟢 in your message). On **skill-scanning failure** (Step 8), write **🔴** followed by "Attention required:" and the failure details. In the **final report**, the first line must start with **✅** and the published skill: **✅ Published: {slug} {version}** — path: `...` (green check so the user sees publish success). In the Done list, start each completed step line with **✅**; when the scan result was safe, include **🟢 SAFE** in that line. Do not omit these emoji from the chat output.

**At start:** After identifying the skill and version (and before running Step 4), **state the plan** using the same format as the Done summary: a heading then a numbered list with **bold** step names. Use this structure:

**Plan**
1. **Skill-scanning:** Check if the skill version exists in Artifactory; upload zip for scanning.
2. **Sign (cosign attest-blob):** Sign the zip with cosign attest-blob (Sigstore, DSSE).
3. **Artifactory:** Upload the skill to Artifactory.
4. **Evidence:** Generate evidence from the skill-scan results and attach (skill-scanning predicate and Sigstore bundle).

## Step 0: Load workspace `.env` (optional)

When running from a workspace that has a `.env` file (e.g. repo root), load it so `RT_REPO_KEY`, `GLOBAL_JPD_USER`, `GLOBAL_JPD_TOKEN`, `JPD_TOKEN`, `VERIFICATION_TOKEN`, `JPD_UPLOAD_URL`, `JPD_UPLOAD_BASE`, `JPD_VERIFY_URL`, `JPD_VERIFY_ENABLED`, `COSIGN_ATTEST_ENABLED`, `COSIGN_PRIVATE_KEY_PATH`, `COSIGN_PASSWORD`, and **evidence signing** vars are available to subsequent steps. For signing evidence (Step 10), pass `--key` from `EVD_SIGNING_KEY_PATH` if set, else the CLI uses `JFROG_CLI_SIGNING_KEY`; **always** pass `--key-alias` from `EVD_KEY_ALIAS` when set (or `JFROG_CLI_KEY_ALIAS` if the CLI supports it), so the evidence alias is never skipped.

```bash
# From workspace root; skip if .env is not present
[ -f .env ] && set -a && source .env && set +a
# JPD verify (Bearer): use a token valid for the scan_status API. Do NOT use GLOBAL_JPD_TOKEN
# here — that token is for Basic auth on upload only; the verify endpoint often requires a
# separate JWT or scanning-service token. Prefer JPD_TOKEN; fall back to VERIFICATION_TOKEN if set.
[ -z "${JPD_TOKEN}" ] && [ -n "${VERIFICATION_TOKEN}" ] && export JPD_TOKEN="${VERIFICATION_TOKEN}"
# JPD upload (Basic): Step 6 uses GLOBAL_JPD_USER and GLOBAL_JPD_TOKEN only
```

If the skill is not run from the repo that contains `.env`, the user must export the required variables in their environment (e.g. in the terminal before starting Cursor, or in shell profile) so the agent's shell sees them.

## Step 1: Identify the Skill Folder

Determine the skill folder from the user's message or current context. The folder must contain a `SKILL.md` file.

If unclear, ask the user with **AskQuestion** — scan for candidate folders:
```bash
find ~/.cursor/skills .cursor/skills -name "SKILL.md" -maxdepth 3 2>/dev/null
```

## Step 2: Read Skill Metadata

Parse the YAML frontmatter from `SKILL.md` to extract **name** (slug) and **version**:

```bash
head -20 {skill_folder}/SKILL.md
```

The frontmatter is between `---` markers:
```yaml
---
name: my-skill
version: 1.0.0
description: ...
---
```

- **slug**: from the `name` field. Must match `^[a-z0-9][a-z0-9-]*$`. If missing, use the folder name.
- **version**: from the `version` field. If the skill uses a **pre-built zip** (see Step 5), version (and slug) may be derived from the zip filename instead: `{slug}_{version}.zip` (e.g. `4chan-reader_1.0.0.zip` → slug=`4chan-reader`, version=`1.0.0`). If missing and not using a pre-built zip, ask the user to provide one.

Slug is required. Version is required unless it is taken from a pre-built zip filename.

## Step 3: Get Artifactory Configuration

Same as install skill — try JFrog CLI first, fall back to env vars.

### Option A: JFrog CLI (preferred)
```bash
jf config show 2>/dev/null
```

If multiple servers are configured, use the `--server-id` flag on all `jf` commands to target the correct server.

### Option B: Environment Variables (fallback)
- `ARTIFACTORY_URL` — Artifactory base URL
- `JFROG_ACCESS_TOKEN` — access token

### Repo Key
Check `RT_REPO_KEY` env var. If not set, ask the user.

### JPD Configuration (optional)

If a Global JPD Server is available for content verification (skill certification), check for these environment variables:

**Upload (Step 6) — use curl with Basic auth:**
- `GLOBAL_JPD_USER` and `GLOBAL_JPD_TOKEN` — username and token for **Basic auth** when uploading the zip (required for JPD upload)
- `JPD_UPLOAD_URL` — full URL to upload the skill zip (e.g. `https://z0skilltest.jfrog.info/artifactory/skillscan-skills/{slug}-{version}.zip`; replace `{slug}` and `{version}` before the request), or a base URL ending with `/` (then the agent appends `{slug}-{version}.zip`)
- Alternatively `JPD_UPLOAD_BASE` — base URL without filename (e.g. `https://z0skilltest.jfrog.info/artifactory/skillscan-skills`); the agent uploads to `{JPD_UPLOAD_BASE}/{slug}-{version}.zip`

**Verify (Steps 7–8) — Bearer token:**
- `JPD_TOKEN` — Bearer token for polling the verify endpoint (e.g. a JWT for the scanning service). Must be valid for the verify API; it is **not** the same as `GLOBAL_JPD_TOKEN` (which is for Basic auth on upload). If unset, Step 0 can set it from `VERIFICATION_TOKEN` when present.
- `VERIFICATION_TOKEN` — (optional) Alternative env var for the verify Bearer token; used in Step 0 when `JPD_TOKEN` is not set.
- `JPD_VERIFY_URL` — URL to poll for scan status, with a `{sha256}` placeholder replaced by the uploaded zip's SHA256. Example: `https://jfs-skill-scanning.jfrog.info/api/v1/skill_analysis/scan_status?sha256={sha256}`

**Toggle (optional):**
- `JPD_VERIFY_ENABLED` — whether to run the global JPD verification flow (Steps 6–8). When set to a **disabled** value (`0`, `false`, `no`, or `off`, case-insensitive), skip Steps 6–8 and proceed directly from Step 5 to Step 9 (upload to Artifactory only). When unset or any other value (e.g. `1`, `true`, `yes`), use JPD verification when configured. To check in shell: treat as disabled when `$(echo "${JPD_VERIFY_ENABLED}" | tr '[:upper:]' '[:lower:]')` is `0`, `false`, `no`, or `off`; otherwise JPD verification is enabled (if configured).

**Sign with cosign attest-blob (Step 9) — optional toggle:**
- `COSIGN_ATTEST_ENABLED` — whether to sign the skill zip with **cosign attest-blob**. When set to a **disabled** value (`0`, `false`, `no`, or `off`, case-insensitive), skip Step 9 (publish unsigned). When unset or any other value (e.g. `1`, `true`, `yes`), run cosign attest-blob if cosign is installed. Cosign attest-blob produces a Sigstore bundle with a **DSSE envelope**, which the JFrog Evidence API accepts. To check in shell: treat as disabled when `$(echo "${COSIGN_ATTEST_ENABLED}" | tr '[:upper:]' '[:lower:]')` is `0`, `false`, `no`, or `off`.

- `COSIGN_PRIVATE_KEY_PATH` — (optional) path to a **private key** file for key-based signing. When set, the agent uses `cosign attest-blob ... --key "${COSIGN_PRIVATE_KEY_PATH}"` so signing is **non-interactive** (no OIDC browser flow). Use this in CI or when you cannot use Sigstore keyless. When **unset**, cosign uses the default **Sigstore keyless** flow (OIDC in the browser). The key **must** be generated with **`cosign generate-key-pair`**; cosign (e.g. v3) does not accept keys from other tools (e.g. `jf evd` or `openssl`) in standard PEM formats. Do not commit the key file; add it to `.gitignore` or use a secrets manager.

- `COSIGN_PASSWORD` — (optional) password for the key at `COSIGN_PRIVATE_KEY_PATH`. When the key is password-protected, set this in `.env` or the environment so cosign does not prompt (required for non-interactive/CI). Use an empty value for an unencrypted key or one created with an empty passphrase. Cosign reads this automatically when present. **When invoking cosign with `--key`, always set `COSIGN_PASSWORD` in the environment:** if the user has set it in `.env`, it will be used after loading; if not set, use `export COSIGN_PASSWORD="${COSIGN_PASSWORD:-}"` so that keys with no passphrase work without "decryption failed" or interactive prompt.

If **upload credentials** (`GLOBAL_JPD_USER` + `GLOBAL_JPD_TOKEN`), **upload URL** (from `JPD_UPLOAD_URL` or `JPD_UPLOAD_BASE`), and **verify** (`JPD_VERIFY_URL` and `JPD_TOKEN`) are all set, **and** `JPD_VERIFY_ENABLED` is not disabled, the JPD flow (Steps 6–8) is enabled. If any of those are missing or JPD verification is disabled, skip Steps 6–8 and proceed directly from Step 5 to Step 9. **When JPD is enabled, verification is mandatory:** if the Step 6 upload fails or Step 8 reports failure, stop the process — do not proceed to Step 9.

## Step 4: Check for Existing Versions (Overwrite Protection)

Before publishing, fetch the list of existing versions for this skill from Artifactory.

**With JFrog CLI:**
```bash
jf rt curl "/api/openclaw/{repoKey}/api/v1/skills/{slug}/versions" --silent
```

**With curl fallback:**
```bash
curl -s -H "Authorization: Bearer ${JFROG_ACCESS_TOKEN}" \
  "${ARTIFACTORY_URL}/api/openclaw/${RT_REPO_KEY}/api/v1/skills/{slug}/versions"
```

Response JSON structure:
```json
{
  "versions": [
    { "version": "1.2.0", "createdAt": "2025-01-15T...", "changelog": "..." },
    { "version": "1.1.0", "createdAt": "2025-01-10T...", "changelog": "..." }
  ]
}
```

### If no version was provided (not in frontmatter, not stated by user)

Do **NOT** offer to overwrite. Instead, use the existing versions list to **suggest the next logical version**. Use **AskQuestion** with smart suggestions based on the latest published version (e.g. if `1.0.0` exists, suggest `1.0.1` as a patch bump and `1.1.0` as a minor bump). Let the user pick or type their own.

### If the user explicitly provided a version that already exists

The user intentionally chose a version that collides. Use **AskQuestion** to warn them:

- Show the existing versions so the user can see what's already published.
- Clearly state that version `{version}` already exists and uploading will **overwrite** it.
- Offer options: **Overwrite** (proceed anyway), **Use a different version** (suggest next patch/minor bumps as above), or **Cancel**.
- If the user chooses a different version, update `{version}` accordingly and re-check against the existing list.

### If the target version does NOT exist

Proceed to Step 5.

### If no versions exist yet (new skill)

Default to `1.0.0` if no version was provided, or use the version the user specified. Proceed to Step 5.

## Step 5: Place Skill in Generated Folder & Zip

**Generated artifacts:** All generated files (zip, cosign attest-blob bundle `.sig`, verification JSON/MD) are written under **generated/{slug}-{version}/** at the workspace root (e.g. `generated/testskill-1.0.0/testskill-1.0.0.zip`, `generated/testskill-1.0.0/testskill-1.0.0.sig`). Create that folder at the start of this step: `mkdir -p generated/{slug}-{version}`. **Do not delete any files under `generated/` during cleanup** (Step 11); they are kept for the user.

### Pre-built zip (always upload the zip as-is)

When publishing a skill that has a **pre-built zip**, use that zip **as-is** for uploads. Do **not** unpack it. After JPD verification success (Step 8), save the verification response to a file for evidence only; in Step 10 upload this same zip to Artifactory and attach the verification file as evidence via `jf evd`.

**Where to find pre-built zips:** In the workspace `zip/` folder, with filename `{slug}_{version}.zip` (e.g. `4chan-reader_1.0.0.zip`). You can derive **slug** and **version** from the filename when using a pre-built zip. Optional table for known skills:

| Skill (folder or slug) | Pre-built zip path (relative to workspace root) |
|------------------------|--------------------------------------------------|
| **4chan-reader**       | `zip/4chan-reader_1.0.0.zip`                     |
| **yahoo-finance-b5p** (slug `yahoo-finance`) | `zip/yahoo-finance_1.0.0.zip`        |

**Check for the pre-built zip:** Once you have the slug (and version, or default e.g. `1.0.0`), **always** check by calling the **Read** tool on the path `{workspace}/zip/{slug}_{version}.zip`. Do **not** use Glob or directory listing to find `.zip` files — many tools exclude or omit zip files, so they will not be found. Only the Read-on-path check is reliable.

1. **Required:** Use the Read tool on that path (e.g. `{workspace}/zip/4chan-reader_1.0.0.zip`). If the tool returns an error **"Cannot read binary files"**, the file **exists** — treat as pre-built zip present. If the tool returns **"File not found"** (or the path does not exist), treat as missing.
2. **Optional shell fallback:** If using a shell check, write the result to a file in the workspace so the agent can read it; do not rely on echoed output alone:
   ```bash
   # From workspace root; path: zip/{slug}_{version}.zip
   test -f zip/4chan-reader_1.0.0.zip && echo EXISTS > .prebuilt-check || echo MISSING > .prebuilt-check
   # Then read .prebuilt-check to decide. Remove after: rm -f .prebuilt-check
   ```

If the pre-built zip exists:
1. Resolve its path from workspace root (e.g. `{workspace}/zip/4chan-reader_1.0.0.zip`). If version was not set in Step 2, derive **slug** and **version** from the filename: `{slug}_{version}.zip`.
2. Create the output folder and copy the zip: `mkdir -p generated/{slug}-{version}` then copy the zip to `generated/{slug}-{version}/{slug}-{version}.zip`. **Do not unpack yet.**
3. Skip the "Copy the skill files" and "Create the zip" below. Proceed to Step 6 and **upload this zip as-is** to JPD (the exact pre-built bytes). After Step 8 (verification success), save the verification response to `generated/{slug}-{version}/{slug}-{version}-verification.json` for evidence only; do **not** unpack or modify the zip. In Step 10 this same zip is uploaded to Artifactory, then verification is attached as evidence via `jf evd`.

If the pre-built zip is **missing**, fall back to the normal flow below. For any other skill, follow the normal flow below.

### Normal flow: copy skill folder and create zip

Copy the skill files into a **staging directory** under `generated/{slug}-{version}/build/`, create the zip there, and write the zip to `generated/{slug}-{version}/{slug}-{version}.zip`. Do **not** use `/tmp`; all build artifacts live under `generated/`.

```bash
mkdir -p generated/{slug}-{version}/build
cp -R {skill_folder}/* generated/{slug}-{version}/build/
rm -rf generated/{slug}-{version}/build/.* generated/{slug}-{version}/build/__pycache__ 2>/dev/null || true
cd generated/{slug}-{version}/build && zip -r ../{slug}-{version}.zip . -x ".*" "__pycache__/*" "*.pyc"
```

The zip used for the rest of the flow is `generated/{slug}-{version}/{slug}-{version}.zip`.

## Steps 6–8: JPD Verification (only if JPD is configured and enabled)

> Skip to Step 9 if JPD is not fully configured (missing `GLOBAL_JPD_USER`, `GLOBAL_JPD_TOKEN`, upload URL, or verify URL/token), **or** if `JPD_VERIFY_ENABLED` is set to a disabled value (`0`, `false`, `no`, `off`). **When JPD is enabled, verification is mandatory:** upload failure in Step 6 or verification failure in Step 8 must stop the process; do not continue to Step 9.

### Step 6: Upload to JPD for Scanning

Obtain the SHA256 of the skill zip (needed for the verify URL in Step 7). Compute it before upload:

```bash
sha256=$(shasum -a 256 generated/{slug}-{version}/{slug}-{version}.zip | awk '{print $1}')
# Or on Linux: sha256sum generated/{slug}-{version}/{slug}-{version}.zip | awk '{print $1}'
```

Build the upload URL: if `JPD_UPLOAD_BASE` is set, use `{JPD_UPLOAD_BASE}/{slug}-{version}.zip`; else if `JPD_UPLOAD_URL` ends with `/`, append `{slug}-{version}.zip`; else replace placeholders `{slug}` and `{version}` in `JPD_UPLOAD_URL`.

Upload the zip with **curl and Basic auth** using **GLOBAL_JPD_USER** and **GLOBAL_JPD_TOKEN**:

```bash
curl -L -u "${GLOBAL_JPD_USER}:${GLOBAL_JPD_TOKEN}" -T generated/{slug}-{version}/{slug}-{version}.zip "${UPLOAD_URL}"
```

If the upload fails (e.g. non-2xx response), **stop the process**: alert the user. If the normal flow was used in Step 5, remove only the staging build dir: `rm -rf generated/{slug}-{version}/build`. Do **not** delete the zip or other files under `generated/{slug}-{version}/`. Do not proceed to Step 9.

If the upload response returns a different SHA256 or scan identifier, use that for the verify step instead. Otherwise use the computed `sha256` from above.

When using the **normal** flow, the zip lives only in `generated/`; do **not** delete it. When using a **pre-built zip**, do **not** delete the zip in `generated/{slug}-{version}/`; it is the **original** zip that will be uploaded to Artifactory in Step 10 (unchanged; verification is attached as evidence only, not added into the zip).

### Step 7: Poll Verification Status

Poll the JPD verify URL until the scan completes. The verify endpoint is called with the skill's SHA256 (as a query parameter). Replace the placeholder in `JPD_VERIFY_URL` with the actual `sha256` value from Step 6, or build the URL as `"${JPD_VERIFY_URL_BASE}?sha256=${sha256}"` if you use a base URL.

Example (verify URL with placeholder replaced by the uploaded skill's SHA256):

```bash
# Replace placeholder {sha256} or <sha256> in JPD_VERIFY_URL with the actual sha256 from Step 6
verify_url=$(echo "${JPD_VERIFY_URL}" | sed "s/{sha256}/${sha256}/g" | sed "s/<sha256>/${sha256}/g")
# Or if JPD_VERIFY_URL is the base (no query): verify_url="${JPD_VERIFY_URL}?sha256=${sha256}"

curl -s -H "Authorization: Bearer ${JPD_TOKEN}" \
  "${verify_url}"
```

Example verify URL form: `https://jfs-skill-scanning.jfrog.info/api/v1/skill_analysis/scan_status?sha256=<sha256>` (replace `<sha256>` with the uploaded skill's SHA256).

Expected response:
```json
{
  "status": "pending | success | failure",
  "message": "..."
}
```

**Polling logic (do not wait when the API already returned a terminal state):** Call the verify endpoint once and parse the response (e.g. with `jq`: read `status` or `scan_status` for the state, and `scan_result` if present). If the state is **success**, **completed**, or **failure**, stop immediately and proceed to Step 8 — **do not sleep**. Only if the state is **pending**, sleep 5 seconds and call the endpoint again; repeat until a terminal state. The response may use `"status"` or `"scan_status"` (values may be `"success"`, `"completed"`, `"pending"`, `"failure"`) and may include `"scan_result"` (e.g. `"safe"`, `"unsafe"`). In Step 8, treat **success or completed with scan_result safe** as pass; treat **failure** or **success/completed with scan_result not safe** as attention required (🔴).

### Step 8: Handle Verification Result

### Failure (status not success, or scan_result not safe)

**JPD verification is mandatory when JPD is configured.** Alert the user with a **red attention marker** so the outcome is obvious at a glance. In chat, state: **🔴 Attention required:** verification did not pass (include the `status`, `scan_result` if present, and `message` from the response). If the normal flow was used in Step 5, remove only the staging build dir: `rm -rf generated/{slug}-{version}/build`. Do **not** delete the zip or other files under `generated/{slug}-{version}/`. **Stop the process** — do not proceed to Step 9 or Step 10.

### Success (status success or completed, and scan result safe)

**Required chat output:** As soon as you determine the scan passed, you **MUST** write the following phrase in your reply to the user so they see it in the chat (the 🟢 must appear as a literal character in your message): **🟢 Skill-scanning complete — result: SAFE.** Do not skip this line or only mention it in tool output; the user must see the green circle in the chat. (The API may return `scan_status: "completed"` instead of `"success"`; when `scan_result` is `"safe"`, that counts as success — still output the phrase with 🟢. For non-safe do not use 🟢.)

The verification response includes a valid JSON payload. Save the **actual response body** from the curl call to two files under **generated/{slug}-{version}/** for use as **evidence only** (Step 10): a JSON file (predicate) and a Markdown file (human-readable, used with `--markdown`). Do **not** unpack the zip or add these files into the zip; the zip uploaded to Artifactory stays the **original** zip.

1. Save the response as JSON:
   ```bash
   echo "${response_body}" > generated/{slug}-{version}/{slug}-{version}-verification.json
   ```

2. Create a Markdown summary from the JSON (e.g. scan result, name, sha256, scanned_at, reason). Example using `jq`:
   ```bash
   jq -r '"# Verification Report\n\n| Field | Value |\n|-------|-------|\n| Name | \(.name) |\n| SHA256 | \(.sha256) |\n| Scan result | \(.scan_result) |\n| Scan status | \(.scan_status) |\n| Scanned at | \(.scanned_at) |\n| Reason | \(.reason) |"' generated/{slug}-{version}/{slug}-{version}-verification.json > generated/{slug}-{version}/{slug}-{version}-verification.md
   ```
   If `jq` is not available, build the markdown manually from the same fields (name, sha256, scan_result, scan_status, scanned_at, reason) and write it to `generated/{slug}-{version}/{slug}-{version}-verification.md`.

## Step 9: Sign Content with cosign attest-blob (optional)

Skip this step if `COSIGN_ATTEST_ENABLED` is set to a disabled value (`0`, `false`, `no`, or `off`, case-insensitive). Otherwise, check if **cosign** is available:

```bash
command -v cosign &>/dev/null
```

If cosign is available **and** `COSIGN_ATTEST_ENABLED` is not disabled, sign the **zip file** at `generated/{slug}-{version}/{slug}-{version}.zip` using **cosign attest-blob**. This produces a Sigstore bundle with a **DSSE envelope**, which the JFrog Evidence API accepts. Write the bundle to the generated folder so it can be attached as evidence and kept for the user. Do **not** add the `.sig` file into the zip.

Generate a **predicate** file with skill slug, version, and attestation time (cosign attest-blob requires `--predicate`). Then run attest-blob.

**With jq** (recommended):
```bash
attestedAt=$(date -u +%Y-%m-%dT%H:%M:%SZ)
jq -n --arg slug "{slug}" --arg version "{version}" --arg at "$attestedAt" '{skill: $slug, version: $version, attestedAt: $at}' > generated/{slug}-{version}/predicate.json
```

**Without jq** (fallback):
```bash
attestedAt=$(date -u +%Y-%m-%dT%H:%M:%SZ)
echo "{\"skill\":\"{slug}\",\"version\":\"{version}\",\"attestedAt\":\"$attestedAt\"}" > generated/{slug}-{version}/predicate.json
```

Then run attest-blob:
```bash
cosign attest-blob \
  --predicate generated/{slug}-{version}/predicate.json \
  --type https://cosign.sigstore.dev/attestation/v1 \
  --bundle generated/{slug}-{version}/{slug}-{version}.sig \
  -y \
  generated/{slug}-{version}/{slug}-{version}.zip
```

For **key-based** signing (e.g. CI or non-interactive), add `--key` and ensure `COSIGN_PASSWORD` is set so cosign does not prompt or fail with "decryption failed":

```bash
# Use COSIGN_PASSWORD from .env if set; otherwise empty (for unencrypted or empty-passphrase keys)
export COSIGN_PASSWORD="${COSIGN_PASSWORD:-}"
cosign attest-blob \
  --predicate generated/{slug}-{version}/predicate.json \
  --type https://cosign.sigstore.dev/attestation/v1 \
  --key "${COSIGN_PRIVATE_KEY_PATH}" \
  --bundle generated/{slug}-{version}/{slug}-{version}.sig \
  -y \
  generated/{slug}-{version}/{slug}-{version}.zip
```

(Optional env: `COSIGN_PRIVATE_KEY_PATH` — path to private key from `cosign generate-key-pair`; if set, add `--key` as above and set `COSIGN_PASSWORD` as shown so unencrypted keys work. `COSIGN_PASSWORD` — if the key is encrypted, set in `.env`; if unset, empty is used so keys with no passphrase do not trigger "decryption failed". Otherwise cosign uses Sigstore keyless, which may open an OIDC flow in the browser.)

Monitor the terminal output. With keyless, cosign may prompt for OIDC in the browser. Wait for the command to complete (exit code 0 confirms success).

If cosign is **not** installed, skip this step but inform the user that the skill will be published **unsigned**. Recommend installing cosign for supply chain security; the resulting `.sig` bundle is accepted by the JFrog Evidence API.

## Step 10: Upload Original Zip to Artifactory, Then Attach Verification as Evidence

Upload the **original** zip (local file at `generated/{slug}-{version}/{slug}-{version}.zip`) to Artifactory. Do **not** unpack it or add verification into it; the file is uploaded as-is.

**Artifactory path (repository path only):** The path in Artifactory must be **only** the repository path: `{repoKey}/{slug}/{version}/{slug}-{version}.zip`. Do **not** include any local filesystem path (e.g. no `/tmp`, `tmp`, or other local directory) in the Artifactory target; the first argument to `jf rt upload` is the local file, the second is the repository path only.

**With JFrog CLI:**  
Use the target as a **folder** in the repo (path ending with `/`): `{repoKey}/{slug}/{version}/`. Pass **`--flat`** so the local path (e.g. `generated/{slug}-{version}/`) is not recreated in the repository; only the filename `{slug}-{version}.zip` is used, giving `{repoKey}/{slug}/{version}/{slug}-{version}.zip`. Do **not** use `--silent` — it is not a valid option for `jf rt upload` and causes "Wrong number of arguments (3)".

When the user targets a specific server (e.g. openclawdemo, local-rt), add `--server-id <serverId>` to the command.

```bash
jf rt upload generated/{slug}-{version}/{slug}-{version}.zip "{repoKey}/{slug}/{version}/" --flat
# With a specific server: add --server-id {serverId}
# Omit --server-id if using the default server
```

After the upload command succeeds (exit 0), state in chat: **✅ Upload to Artifactory successful** — path: `{repoKey}/{slug}/{version}/{slug}-{version}.zip`.

**With curl fallback:**
```bash
curl -s -H "Authorization: Bearer ${JFROG_ACCESS_TOKEN}" \
  -T generated/{slug}-{version}/{slug}-{version}.zip \
  "${ARTIFACTORY_URL}/${RT_REPO_KEY}/{slug}/{version}/{slug}-{version}.zip"
```

### Attach verification as evidence (when JPD was used)

When JPD verification ran (Steps 6–8), attach the saved verification payload as evidence to the uploaded skill zip using the Evidence service. Use the predicate file saved in Step 8: `generated/{slug}-{version}/{slug}-{version}-verification.json`, and the markdown file: `generated/{slug}-{version}/{slug}-{version}-verification.md` (pass it with `--markdown` so the evidence includes a human-readable summary). The **subject path in Artifactory** must be the repository path only: `{repoKey}/{slug}/{version}/{slug}-{version}.zip` (hyphen between slug and version in the filename, not underscore). Do **not** use any local path (e.g. no `tmp`) in `--subject-repo-path`.

Run this **after** the zip is uploaded; both predicate and markdown files are available under `generated/{slug}-{version}/` from Step 8.

**With JFrog CLI (jf evd):**

1. The SHA256 of the zip we just uploaded is the same as the original zip (we did not repackage). Use the value from Step 6, or compute it again:
   ```bash
   sha256_final=$(shasum -a 256 generated/{slug}-{version}/{slug}-{version}.zip | awk '{print $1}')
   ```

2. Attach the verification payload as evidence. **Use the signing key and alias when available:** after loading `.env`, if `EVD_SIGNING_KEY_PATH` is set, pass `--key "${EVD_SIGNING_KEY_PATH}"`; otherwise the CLI uses `JFROG_CLI_SIGNING_KEY` if set. **Always pass the evidence key alias when set:** if `EVD_KEY_ALIAS` is set, pass `--key-alias "${EVD_KEY_ALIAS}"` so the created evidence is associated with that alias; if unset, `JFROG_CLI_KEY_ALIAS` may be used by the CLI. Use the same `--server-id` as for the upload if targeting a specific server.
   ```bash
   jf evd create \
     --subject-repo-path "{repoKey}/{slug}/{version}/{slug}-{version}.zip" \
     --subject-sha256 "${sha256_final}" \
     --predicate generated/{slug}-{version}/{slug}-{version}-verification.json \
     --predicate-type "https://jfrog.com/evidence/verification/v1" \
     --markdown generated/{slug}-{version}/{slug}-{version}-verification.md \
     ${EVD_SIGNING_KEY_PATH:+--key "${EVD_SIGNING_KEY_PATH}"} \
     ${EVD_KEY_ALIAS:+--key-alias} ${EVD_KEY_ALIAS:+$EVD_KEY_ALIAS}
   # With a specific server: add --server-id {serverId}
   # Pass --key-alias and its value as two separate arguments so the CLI receives two words (not one); same for --key above.
   ```

   If the CLI reports that the subject "does not exist", confirm the path uses a **hyphen** in the filename (`{slug}-{version}.zip`), not an underscore, and that the upload in the previous step succeeded. If it reports that a signing key is required, ensure `JFROG_CLI_SIGNING_KEY` or `EVD_SIGNING_KEY_PATH` (and alias) are set in `.env` or the environment and re-run.

If `jf evd` is not available or the evidence step fails (and no signing key was configured), warn the user but do not block the publish; the skill zip has been uploaded successfully.

### Attach Sigstore bundle as evidence (when cosign attest-blob was used)

When Step 9 ran and cosign attest-blob produced `generated/{slug}-{version}/{slug}-{version}.sig`, attach that Sigstore bundle to the uploaded skill zip. The cosign attest-blob bundle contains a **DSSE envelope**, which the JFrog Evidence API accepts. Run this **after** the zip is uploaded.

```bash
jf evd create \
  --subject-repo-path "{repoKey}/{slug}/{version}/{slug}-{version}.zip" \
  --sigstore-bundle generated/{slug}-{version}/{slug}-{version}.sig
# With a specific server: add --server-id {serverId}
```

If the CLI reports that the subject "does not exist", confirm the upload succeeded and the path uses a hyphen in the filename. If it reports "sigstore bundle does not contain a DSSE envelope", the file was not produced by cosign attest-blob (e.g. it was a cosign sign-blob bundle); use **cosign attest-blob** (not sign-blob) to produce the `.sig`. If this step fails, warn the user but do not block the publish; the skill zip has been uploaded successfully.

> **Note:** If the user confirmed an overwrite in Step 4, this upload will replace the existing artifact.

## Step 11: Clean Up and Report

**Chat output (Cursor):** When summarizing in the Cursor chat, use this wording so the user sees consistent, friendly messages:
- **Do not** mention "pre-built zip" or "pre-built zip flow". If the skill was published from a zip in `zip/`, say only that you used the zip from `zip/{slug}_{version}.zip` (or "published from zip") without using the term "pre-built".
- **Do not** say "JPD verification" or "JPD flow" to the user. Use **skill-scanning** instead (e.g. "skill-scanning passed", "skill-scanning result: safe", "uploaded for skill-scanning", "skill-scanning evidence attached").
- **Do not** say "Nothing under generated/ was deleted" or "were not deleted" in the final report.

**Do not delete the zip, cosign attest-blob `.sig` bundle, or verification files under `generated/{slug}-{version}/`.** They are kept for the user. If the **normal** flow was used in Step 5, remove only the staging build directory (used to create the zip):

```bash
rm -rf generated/{slug}-{version}/build
```

(If the pre-built zip flow was used, there is no `build` staging dir to remove.)

Verify the upload:

**With JFrog CLI:**
```bash
jf rt search "{repoKey}/{slug}/{version}/"
```

**With curl fallback:**
```bash
curl -s -H "Authorization: Bearer ${JFROG_ACCESS_TOKEN}" \
  "${ARTIFACTORY_URL}/api/storage/${RT_REPO_KEY}/{slug}/{version}/{slug}-{version}.zip"
```

**Final report format:** Use this exact structure so the chat looks like the preferred "Done" summary:

1. In one short line, start with **✅** then state the published skill name, version, and Artifactory path: **✅ Published: {slug} {version}** — path: `{repoKey}/{slug}/{version}/{slug}-{version}.zip` (the ✅ is required so the user sees a green check on publish success).
2. Then a **Done** heading.
3. Then a numbered list of exactly four items (use **bold** for the step name in each):
   - **1. Skill-scanning:** ✅ Zip uploaded for scanning; result **🟢 SAFE** (or the actual scan_result) (completed). (If verification failed earlier, the user would have seen 🔴 Attention required in Step 8.)
   - **2. Sign (cosign attest-blob):** ✅ Zip signed with cosign attest-blob; bundle saved as `{slug}-{version}.sig`. (If signing was skipped, say "Skipped" or "Not signed" without ✅.)
   - **3. Artifactory:** ✅ Zip uploaded to the path above.
   - **4. Evidence:** ✅ Skill-scanning predicate and Sigstore bundle attached and verified. (If only one type was attached, say which; if evidence was skipped or failed, say so without ✅.)

Keep the report concise. Do not add a table, "Generated artifacts" paragraph, or deletion wording. Use **skill-scanning** (not "JPD verification"). Do not mention "pre-built zip".
