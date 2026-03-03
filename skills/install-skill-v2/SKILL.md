---
name: install-skill-v2
description: Install or upgrade skills from Artifactory. Use when the user wants to install, upgrade, update, pull, download, fetch, or search for a skill or package from Artifactory.
---

# Install or Upgrade Skill from Artifactory (v2)

Downloads, installs, or upgrades skills from an Artifactory skills repository. Verifies publish-attestation evidence via `jf evd verify`.

## Agent behavior

- At the start of execution, **state the plan** before proceeding:

  **Plan**
  1. **Resolve:** Confirm skill and version (search if needed); check existing installation.
  2. **Download:** Fetch zip from Artifactory and extract to temp.
  3. **Verify:** Verify signed publish-attestation evidence.
  4. **Install:** Copy to chosen location and report.

- In the **final report**, start the first line with **✅ Installed: {slug} {version}** -- `{install_path}{slug}/`. Start each Done item with **✅**.

## Prerequisites

- **JFrog CLI** (v2.65.0+) installed and in `PATH`. Configure at least one server with `jf config add`.
- **Skills repository** is auto-discovered from Artifactory. Set `RT_REPO_KEY` in `.env` to override.
- **No signing keys needed.** Evidence verification (Phase 3) uses `jf evd verify --use-artifactory-keys`, which pulls the publisher's public key from Artifactory automatically. The installer does not need any keypair.

## Phase 1: Resolve

### Load environment

```bash
[ -f .env ] && set -a && source .env && set +a
```

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

If no servers are configured, stop and tell the user to run `jf config add`. If multiple servers are configured, use `--server-id` on all `jf` commands.

**3. Skills repository:** Discover the skills repo automatically. If `RT_REPO_KEY` is set in `.env`, use it as an override. Otherwise, query Artifactory:

```bash
jf rt curl "/api/repositories?type=local&packageType=skills" --silent
```

- **One repo found:** Use it automatically.
- **Multiple repos:** Present them via **AskQuestion** -- "Which skills repo?"
- **None found:** Tell the user to create a skills repository in Artifactory.

### Parse user request

Extract from the user's message:
- **slug** (optional) -- lowercase alphanumeric with hyphens, e.g. `todoist-cli`
- **version** (optional) -- semver format, e.g. `1.2.0`
- **intent** -- installing fresh or upgrading?

### Search for skill (if no exact slug)

If the user gave a vague description or no slug, search:

```bash
jf rt curl "/api/skills/{repoKey}/api/v1/search?q={query}&limit=10" --silent
```

Response: `{ "skills": [{ "slug": "...", "displayName": "...", "summary": "..." }, ...] }`

Present results via **AskQuestion** formatted as `{slug} -- {summary}`. If no results, ask the user to refine.

### Detect existing installation

```bash
ls ~/.cursor/skills/{slug}/SKILL.md 2>/dev/null
ls .cursor/skills/{slug}/SKILL.md 2>/dev/null
```

If found: read the YAML frontmatter for the current `version`, note the install path (reused later). This is an **upgrade** flow. If not found: **fresh install** flow.

### Resolve version

If version was provided, proceed. Otherwise, list available versions:

```bash
jf rt curl "/api/skills/{repoKey}/api/v1/skills/{slug}/versions" --silent
```

Response: `{ "versions": [{ "version": "1.2.0", "createdAt": "...", "changelog": "..." }, ...] }`

**Upgrade-aware selection:**
- Annotate the currently installed version, e.g. `1.1.0 (installed)`
- For upgrades, only show versions newer than installed
- If already on latest, inform the user and stop

Present top 10 versions via **AskQuestion** formatted as `{version} - {changelog or createdAt}`.

## Phase 2: Download

Download and extract the skill zip:

```bash
jf rt curl "/api/skills/{repoKey}/api/v1/download?slug={slug}&version={version}" \
  --output /tmp/{slug}-{version}.zip --silent

mkdir -p /tmp/{slug}-{version}
unzip -o /tmp/{slug}-{version}.zip -d /tmp/{slug}-{version}/
```

## Phase 3: Verify

Verify the artifact's publish-attestation using the JFrog Evidence service. The CLI pulls public keys from Artifactory and verifies signatures locally -- no local keys needed. Unset any signing key env vars first, since publisher-side vars from `.env` (e.g. `JFROG_CLI_SIGNING_KEY`) interfere with `--use-artifactory-keys`.

```bash
unset JFROG_CLI_SIGNING_KEY EVD_SIGNING_KEY_PATH 2>/dev/null
jf evd verify \
  --subject-repo-path "{repoKey}/{slug}/{version}/{slug}-{version}.zip" \
  --use-artifactory-keys
```

Add `--server-id <id>` when targeting a non-default server.

**Exit code 0:** Evidence verified -- proceed to Phase 4.

**Non-zero exit:** Treat the skill as **unattested**. Inform the user. Use **AskQuestion** to let them decide whether to proceed or abort. If the user aborts, clean up (`rm -rf /tmp/{slug}-{version} /tmp/{slug}-{version}.zip`) and stop.

## Phase 4: Install

### Determine install location

**Upgrade:** Reuse the existing install path detected in Phase 1.

**Fresh install:** Ask via **AskQuestion**:
- **Personal**: `~/.cursor/skills/{slug}/`
- **Project**: `.cursor/skills/{slug}/`

### Copy and clean up

```bash
mkdir -p {install_path}/{slug}
cp -R /tmp/{slug}-{version}/* {install_path}/{slug}/
rm -rf /tmp/{slug}-{version} /tmp/{slug}-{version}.zip
```

### Report

Confirm `SKILL.md` exists at the install location:

```bash
ls {install_path}/{slug}/SKILL.md
```

**Final report format:**

1. **✅ Installed: {slug} {version}** -- `{install_path}{slug}/`
2. **Done** heading, then exactly four items:
   - **1. Resolve:** ✅ Skill and version confirmed (or searched and selected); existing install noted if upgrade.
   - **2. Download:** ✅ Zip fetched from Artifactory and extracted.
   - **3. Verify:** ✅ Evidence verified; publish-attestation signed. (If unattested or skipped, say so without ✅.)
   - **4. Install:** ✅ Content copied to the path above; SKILL.md verified.
