package npm

import "strings"

// npmTelemetryPrefix is the Call Home / Visibility feature_id prefix for jf npm.
const npmTelemetryPrefix = "rt_npm"

// canonicalNpmVerbs maps an npm CLI verb (or alias) to the telemetry suffix.
// Unknown tokens and host-like strings are dropped so argv (URLs, repo keys)
// does not become a feature_id.
var canonicalNpmVerbs = map[string]string{
	"access": "access", "add": "install", "adduser": "adduser", "audit": "audit",
	"bugs": "bugs", "c": "config", "cache": "cache", "ci": "ci",
	"cit": "install-ci-test", "clean-install": "ci", "completion": "completion",
	"config": "config", "dedupe": "dedupe", "deprecate": "deprecate", "diff": "diff",
	"dist-tag": "dist-tag", "docs": "docs", "doctor": "doctor", "exec": "exec",
	"explain": "explain", "explore": "explore", "find-dupes": "find-dupes",
	"fund": "fund", "get": "get", "help": "help", "help-search": "help-search",
	"hook": "hook", "i": "install", "info": "view", "init": "init",
	"install": "install", "install-ci-test": "install-ci-test",
	"install-test": "install-test", "isntall": "install", "it": "install-test",
	"link": "link", "list": "ls", "ll": "ll", "login": "login", "logout": "logout",
	"ls": "ls", "org": "org", "outdated": "outdated", "owner": "owner",
	"p": "publish", "pack": "pack", "ping": "ping", "pkg": "pkg",
	"prefix": "prefix", "profile": "profile", "prune": "prune", "publish": "publish",
	"query": "query", "rb": "rebuild", "rebuild": "rebuild", "repo": "repo",
	"restart": "restart", "root": "root", "run": "run", "run-script": "run",
	"sbom": "sbom", "search": "search", "set": "set", "show": "view",
	"shrinkwrap": "shrinkwrap", "star": "star", "stars": "stars", "start": "start",
	"stop": "stop", "t": "test", "team": "team", "test": "test", "token": "token",
	"tst": "test", "un": "uninstall", "undeprecate": "undeprecate",
	"uninstall": "uninstall", "unpublish": "unpublish", "unstar": "unstar",
	"up": "update", "update": "update", "version": "version", "view": "view",
	"whoami": "whoami",
}

// npmTelemetryCommandName is the feature_id sent to Call Home and Visibility.
// Exec still uses the raw cmdName; only the usage report name is sanitized.
func npmTelemetryCommandName(cmdName string) string {
	if verb := canonicalNpmVerb(cmdName); verb != "" {
		return npmTelemetryPrefix + "_" + verb
	}
	return npmTelemetryPrefix
}

func canonicalNpmVerb(cmdName string) string {
	verb := strings.ToLower(strings.TrimSpace(cmdName))
	if verb == "" || strings.ContainsAny(verb, ":/\\@ \t") {
		return ""
	}
	return canonicalNpmVerbs[verb]
}
