---
name: install-skill
description: Install or upgrade skills from Artifactory. Use when the user wants to install, upgrade, update, pull, download, fetch, or search for a skill or package from Artifactory.
---

# Install or Upgrade Skill from Artifactory

Downloads, installs, or upgrades skills from an Artifactory Skill repository.

**At start:** At the start of execution, **state the plan** using the same format as the Done summary: a heading then a numbered list with **bold** step names. Use this structure:

**Plan**
1. **Resolve:** Confirm skill and version (search if needed); check existing installation.
2. **Download:** Fetch zip from Artifactory and extract to temp.
3. **Verify:** Fetch evidence; if signature exists, download bundle and verify with cosign.
4. **Install:** Copy to chosen location and report.

## Step 0: Load workspace `.env` (optional)

When running from a workspace that has a `.env` file (e.g. repo root), load it so `RT_REPO_KEY`, `ARTIFACTORY_URL`, `JFROG_ACCESS_TOKEN`, `COSIGN_PUBLIC_KEY_PATH`, and similar vars are available to subsequent steps:

```bash
# From workspace root; skip if .env is not present
[ -f .env ] && set -a && source .env && set +a
```

If the skill is not run from the repo that contains `.env`, the user must export the required variables in their environment so the agent's shell sees them.

## Step 1: Parse User Request

Extract from the user's message:
- **slug**: the skill name (optional) — lowercase alphanumeric with hyphens, e.g. `todoist-cli`
- **version**: the version to install (optional) — semver format, e.g. `1.2.0`
- **intent**: is the user installing fresh or upgrading an existing skill?

If the user provided an **exact slug**, proceed to Step 2.
If the user gave a **vague description** or **no slug at all**, skip to Step 2 then go to Step 3 (Search).

## Step 2: Get Artifactory Configuration

### Option A: JFrog CLI (preferred)

Check if `jf` is available and configured:

```bash
jf config show 2>/dev/null
```

If available, use `jf rt curl` for all API calls — it handles authentication automatically. If multiple servers are configured (check `jf config show`), use the `--server-id` flag on all `jf` commands to target the correct server.

### Option B: Environment Variables (fallback)

If JFrog CLI is not configured, require these environment variables:
- `ARTIFACTORY_URL` — Artifactory base URL (e.g. `https://mycompany.jfrog.io/artifactory`)
- `JFROG_ACCESS_TOKEN` — access token for authentication

If missing, tell the user to either:
1. Install and configure JFrog CLI (`jf config add`)
2. Set the environment variables

### Repo Key

The Artifactory repository key is always needed. Check `RT_REPO_KEY` env var first. If not set, ask the user with AskQuestion or let them provide it in their message.

## Step 3: Resolve Skill

### If exact slug was provided

Verify the skill exists by fetching its versions (Step 4). If the API returns a 404 or empty result, fall back to search below.

### If no slug, vague description, or slug not found

Search for matching skills:

**With JFrog CLI:**
```bash
jf rt curl "/api/openclaw/{repoKey}/api/v1/search?q={query}&limit=10" --silent
```

**With curl fallback:**
```bash
curl -s -H "Authorization: Bearer ${JFROG_ACCESS_TOKEN}" \
  "${ARTIFACTORY_URL}/api/openclaw/${RT_REPO_KEY}/api/v1/search?q={query}&limit=10"
```

Use the user's description or partial name as the `q` parameter.

Response JSON structure:
```json
{
  "skills": [
    { "slug": "todoist-cli", "displayName": "Todoist CLI", "summary": "...", "tags": [...] },
    { "slug": "github-helper", "displayName": "GitHub Helper", "summary": "...", "tags": [...] }
  ]
}
```

Present the results to the user using **AskQuestion**. Format each option as `{slug} — {summary}`. Once the user picks a skill, use that slug and proceed to Step 4.

If no results are found, tell the user and ask them to refine their search.

## Step 4: Detect Existing Installation

Once the slug is known, check if the skill is already installed locally:

```bash
ls ~/.cursor/skills/{slug}/SKILL.md 2>/dev/null
ls .cursor/skills/{slug}/SKILL.md 2>/dev/null
```

If found in **either** location:
1. Read the YAML frontmatter from the installed `SKILL.md` to extract the current `version`
2. Note the **install path** (personal or project) — this will be reused later
3. This is now an **upgrade** flow

If not found in either location, this is a **fresh install** flow.

## Step 5: Resolve Version

### If version was provided

Skip to Step 6.

### If version was NOT provided

List available versions:

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

### Upgrade-aware version selection

- If Step 4 detected an existing installation, annotate the currently installed version in the list, e.g. `1.1.0 (installed)`
- If the user's intent is to **upgrade**, only show versions **newer** than the currently installed one
- If no newer versions exist, inform the user they are already on the latest version and **stop**

Present the top 10 applicable versions to the user using the **AskQuestion** tool. Format each option as `{version} - {changelog or createdAt}`. Once selected, proceed to Step 6.

## Step 6: Download

**With JFrog CLI:**
```bash
jf rt curl "/api/openclaw/{repoKey}/api/v1/download?slug={slug}&version={version}" \
  --output /tmp/{slug}-{version}.zip --silent
```

**With curl fallback:**
```bash
curl -s -H "Authorization: Bearer ${JFROG_ACCESS_TOKEN}" \
  "${ARTIFACTORY_URL}/api/openclaw/${RT_REPO_KEY}/api/v1/download?slug={slug}&version={version}" \
  -o /tmp/{slug}-{version}.zip
```

## Step 7: Extract to Temp Directory

Extract the zip to a temporary directory for inspection and signature verification:
```bash
mkdir -p /tmp/{slug}-{version}
unzip -o /tmp/{slug}-{version}.zip -d /tmp/{slug}-{version}/
```

## Step 8: Verify Signature (Evidence)

Signature verification uses the JFrog Evidence service: fetch the **evidence index** for the artifact (`jf evd get` → `{slug}-{version}-evd.json`); use it only to get the `downloadPath` for the Sigstore bundle. Then download the **Sigstore bundle** (`.sigstore.json`) and verify the zip with cosign. The evd.json is not verified; the only verification is **cosign** on the zip and the Sigstore bundle. **Evidence-based verification requires JFrog CLI** (Option A in Step 2). If you are using the curl fallback only, skip to "No signature evidence" below and treat the skill as unsigned.

### Fetch evidence index for the artifact

Subject path in the repository: `{repoKey}/{slug}/{version}/{slug}-{version}.zip`.

**With JFrog CLI:**
```bash
jf evd get --subject-repo-path "{repoKey}/{slug}/{version}/{slug}-{version}.zip" \
  --include-predicate --output /tmp/{slug}-{version}-evd.json
# Add --server-id {serverId} if targeting a non-default server
```

**Parse for signature evidence:** Look for an evidence entry whose `predicateSlug` is `cosign` or `model_signing-signature` (or similar Sigstore bundle). Extract its `downloadPath` (repository path to the `.sigstore.json` file).

Example with `jq` (try `cosign` first, then `model_signing-signature`):
```bash
sig_path=$(jq -r '.result.evidence[] | select(.predicateSlug == "cosign" or .predicateSlug == "model_signing-signature") | .downloadPath' /tmp/{slug}-{version}-evd.json | head -1)
```

### Signature evidence found — download and verify

If `sig_path` is non-empty, download the bundle to a path outside the extracted content (so it is not copied into the install path), then verify the zip. Use **jf rt curl** with the repo path (the same as `downloadPath` from evidence); this returns the artifact content. Do **not** use `/api/storage/` in the path — that returns metadata only, not the file.

```bash
jf rt curl "${sig_path}" -o /tmp/{slug}-{version}.sigstore.json
# Add --server-id {serverId} if needed
```

Then verify with **cosign** (if installed):

- **If `COSIGN_PUBLIC_KEY_PATH` is set** (key-based signing): verify using the publisher's public key. This avoids transparency-log certificate mismatches when the skill was signed with `cosign attest-blob --key`:
  ```bash
  cosign verify-blob /tmp/{slug}-{version}.zip \
    --bundle /tmp/{slug}-{version}.sigstore.json \
    --key "${COSIGN_PUBLIC_KEY_PATH}"
  ```
- **Otherwise** (keyless / Fulcio): verify using certificate regexps:
  ```bash
  cosign verify-blob /tmp/{slug}-{version}.zip \
    --bundle /tmp/{slug}-{version}.sigstore.json \
    --certificate-identity-regexp='.*' \
    --certificate-oidc-issuer-regexp='.*'
  ```

> For stricter keyless verification, replace the regexps with exact values matching the publisher's identity (e.g. `--certificate-identity=user@company.com --certificate-oidc-issuer=https://accounts.google.com`).

- If verification **succeeds**, proceed to Step 9.
- If verification **fails**, warn the user that the signature is invalid. Delete the temp directory and zip, then **stop**. Do NOT proceed with installation.

If cosign is **not** installed, warn the user that signature verification was skipped. Recommend installing cosign for supply chain security, then proceed.

### No signature evidence

If `jf evd get` was not run (curl-only flow), the evidence file is missing, or no evidence entry has a Sigstore `downloadPath`, treat the skill as **unsigned**. Inform the user. Use **AskQuestion** to let them decide whether to proceed or abort. If the user aborts, clean up and stop.

## Step 9: Determine Install Location

Only reached if signature is valid, or the user chose to proceed with an unsigned skill.

### Upgrade (skill already installed)

Skip this step — reuse the existing install path detected in Step 4.

### Fresh install

Use the **AskQuestion** tool:
- **Personal**: `~/.cursor/skills/{slug}/`
- **Project**: `.cursor/skills/{slug}/`

## Step 10: Install

Move the extracted content from the temp directory to the chosen install location:
```bash
mkdir -p {install_path}/{slug}
cp -R /tmp/{slug}-{version}/* {install_path}/{slug}/
rm -f {install_path}/{slug}/*.sigstore.json
```

Clean up temporary files (extract dir, zip, evidence JSON, and downloaded signature bundle if any):
```bash
rm -rf /tmp/{slug}-{version} /tmp/{slug}-{version}.zip /tmp/{slug}-{version}-evd.json /tmp/{slug}-{version}.sigstore.json
```

## Step 11: Verify and Report

Confirm `SKILL.md` exists at the install location:
```bash
ls {install_path}/{slug}/SKILL.md
```

When the install completed successfully (copy and verification above succeeded), state in chat: **✅ Installed successfully** — {slug} {version} at `{install_path}{slug}/`.

**Final report format:** Use this structure so the chat matches the preferred "Done" summary:

1. In one short line, state the installed skill name, version, and location with a **success emoji**: **✅ Installed: {slug} {version}** — `{install_path}{slug}/`.
2. Then a **Done** heading.
3. Then a numbered list of exactly four items (use **bold** for the step name in each); use **✅** for successful steps:
   - **1. Resolve:** ✅ Skill and version confirmed (or searched and selected); existing install noted if upgrade.
   - **2. Download:** ✅ Zip fetched from Artifactory and extracted.
   - **3. Verify:** ✅ Evidence fetched; signature downloaded and verified with cosign. (If no signature or cosign skipped, say "Unsigned" or "Signature verification skipped" without ✅.)
   - **4. Install:** ✅ Content copied to the path above; SKILL.md verified.

Keep the report concise.
