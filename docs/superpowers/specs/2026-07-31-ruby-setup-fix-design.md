# `jf setup ruby` — Fix Design

**Date:** 2026-07-31
**Scope:** Ruby-only. Does not touch cargo/alpine/apt/nuget setup implementations.

## Problem

`configureRuby()` (`artifactory/commands/setup/setup.go`) has two confirmed bugs, found via manual testing against a live Artifactory instance:

1. **Broken credential write.** It shells out to `bundle config set <host> <user:pass>`. Bundler ≥ 2.0 supports the `set` subcommand; Bundler 1.x (still the default on stock macOS system Ruby, and any Ruby install that predates 2.0's bundled Bundler) does not — its CLI is `bundle config NAME [VALUE]`, no subcommand. On 1.x, the command doesn't error; it silently misparses `set` as the config key name and the rest as a single value, writing a garbage `BUNDLE_SET` entry to `~/.bundle/config`. `jf setup ruby` reports success regardless (`"Bundler configured: credentials set for host '...'"`), so there is no signal anything went wrong. Even when the subprocess itself fails, the current code only logs a warning (`"Failed to configure Bundler credentials (bundle may not be installed)"`) and continues — a misdiagnosis, since the real cause is unrelated to whether `bundle` is installed.

2. **Fragile `~/.gemrc` write.** `rubyAddSourceToGemrc()` uses raw string concatenation with a `strings.Contains(content, sourceURL)` substring check to avoid duplicates. This only recognizes a byte-identical repeat of the same URL. Configuring a *different* repo on a later run doesn't get deduplicated against the first — it just appends. Reproduced live: running `jf setup ruby` twice against two different repos on the same Artifactory host left both source lines in `~/.gemrc`, with no way to tell which was most recently configured.

## Non-goals

- Auto-editing the project's `Gemfile`. Bundler has no global source-redirect mechanism (unlike Cargo's `[source.crates-io] replace-with`), so the Gemfile edit remains an unavoidable manual step. Keeping this manual is also consistent with the "never write Gemfile" principle already in effect elsewhere in this feature (build-info collection, dependency discovery).
- Any change to cargo, alpine, apt, or nuget setup commands.
- A `--remove`/cleanup command (APT's setup has one; ruby's doesn't, and isn't gaining one here). Worth a future ask, not bundled into this fix.
- Bundler version detection/branching on the CLI syntax. Rejected in favor of not shelling out to `bundle config` at all (see below) — this avoids the whole class of "does this CLI syntax exist on this version" problem permanently, including against *future* Bundler CLI changes, not just the current 1.x/2.x split.

## Design

Both bugs are fixed the same way: stop shelling out to native CLIs for config writes, and read-modify-write the actual YAML files directly, the way `cargo/setup.go`'s `ConfigureNativeRegistry` already does for TOML. `gopkg.in/yaml.v3` is already a direct dependency of `jfrog-cli-core` (which `jfrog-cli-artifactory` already depends on), so this adds no new external dependency.

### `writeBundleConfig(host, user, password string) error`

Replaces the `exec.Command("bundle", "config", "set", ...)` call in `configureRuby()`.

1. Read `~/.bundle/config`. Missing file → treat as empty. File exists but fails to parse as YAML → return an error (do not silently overwrite a file that may be hand-edited and load-bearing — matches Cargo's `mergeTomlFile` behavior: `err != nil && !os.IsNotExist(err) → return err`).
2. Compute the config key via the **existing** `bundleEnvKeyForHost(host)` function (already used by `jf ruby bundle install`'s runtime auth injection in `native_ruby.go`). Reusing it — rather than reimplementing host normalization — guarantees setup-time and runtime-injection key formats can never drift apart.
3. Set/overwrite that key to `"user:password"` in the parsed map. All other existing keys (`BUNDLE_PATH`, other hosts' credentials, anything else already in the file) are preserved untouched.
4. Marshal back to YAML and write, with file mode `0600` (this file now holds a real credential — the current code doesn't set this at all).
5. A write failure at any step is returned as a real error, which `configureRuby()` propagates up as a command failure (non-zero exit). This is a deliberate change from today's behavior: silently continuing after a failed credential write is the exact misdiagnosis this fix exists to eliminate. `jf setup ruby` should never report success when the credential wasn't actually written.

### `addGemrcSource(sourceURL string) error`

Replaces `rubyAddSourceToGemrc()`.

1. Read `~/.gemrc`. Missing file → treat as empty. Parse failure on an existing file → error, same reasoning as above.
2. Preserve all unrelated top-level keys (e.g. `:ssl_ca_cert:`).
3. `:sources:` is a YAML list of strings.
   - If the list doesn't exist yet, create it seeded with `https://rubygems.org` (matches current behavior).
   - If `sourceURL` is already present (exact match) → no duplicate insert; move it to the front of the list (excluding `rubygems.org`, which stays first).
   - If `sourceURL` is not present → prepend it (front of the list, after `rubygems.org`).
   - Rationale for "append, don't replace, when different": `~/.gemrc`'s sources list is natively a multi-source mechanism — bare `gem install` already searches every listed source. Treating a second, different repo as something to *replace* the first with would fight that native behavior. Moving the most-recent one to the front makes it the one `gem` naturally tries first, and the one reflected in the printed "add this source to your Gemfile" suggestion.
4. Marshal back to YAML and write.

### Error handling summary

| Situation | Current behavior | New behavior |
|---|---|---|
| Bundler is 1.x | Silently writes garbage key, reports success | Writes a correct key directly; no dependency on Bundler CLI syntax at all |
| `~/.bundle/config` write fails | Logged as warning, command continues, reports success | Real error surfaced |
| `~/.gemrc` write fails | Logged at debug level only (`log.Debug`), essentially invisible | Real error surfaced |
| Existing file has unrelated keys | Preserved (string concat happens to not clobber them) | Preserved (explicit, via parse-modify-write) |
| Existing file is malformed/hand-edited | Silently appended to (string concat doesn't care about validity) | Clear error, no data loss risk |
| Re-run with the same repo | Skipped (works today) | Skipped, and moved to front |
| Re-run with a different repo | Appended without dedup guarantee, no ordering signal | Appended (intentional — see rationale above), moved to front |

## Testing

- Unit tests for `writeBundleConfig`, mirroring the existing `TestRubyWriteTempGemCredentials*` pattern (from the earlier `gem push` credentials fix in this same file): empty file, file with unrelated existing keys (preserved), overwrite of an existing same-host key, malformed existing file (errors, doesn't clobber).
- Unit tests for `addGemrcSource`: empty file, file with unrelated keys, append of a new source, re-add of an identical source (no duplicate, moved to front), append of a second different source (both present, most recent first).
- Manual/integration verification: the exact repro from manual testing — run `jf setup ruby` twice against two different repos on the same Artifactory host. Confirm `~/.gemrc` has both, most-recent first; confirm `~/.bundle/config` has exactly one correct entry for the shared host, using the same key `jf ruby bundle install` would inject at runtime.
