package helm

import "strings"

// helmTelemetryPrefix is the Call Home / Visibility feature_id prefix for jf helm.
const helmTelemetryPrefix = "rt_helm"

// canonicalHelmVerbs maps a helm CLI verb (or alias) to the telemetry suffix.
// Chart names and other argv must not become a feature_id.
var canonicalHelmVerbs = map[string]string{
	"completion": "completion", "create": "create", "dependency": "dependency",
	"dep": "dependency", "env": "env", "fetch": "pull", "get": "get",
	"hist": "history", "history": "history", "inspect": "show", "install": "install",
	"lint": "lint",
	"list": "list", "ls": "list", "package": "package", "plugin": "plugin",
	"pull": "pull", "push": "push", "registry": "registry", "repo": "repo",
	"rollback": "rollback",
	"search":   "search", "show": "show", "status": "status", "template": "template",
	"test": "test", "uninstall": "uninstall", "del": "uninstall", "delete": "uninstall",
	"rm": "uninstall", "upgrade": "upgrade", "verify": "verify", "version": "version",
}

// helmTelemetryCommandName is the feature_id sent to Call Home and Visibility.
// Exec still uses the raw cmdName as the helm subcommand.
func helmTelemetryCommandName(cmdName string) string {
	if verb := canonicalHelmVerb(cmdName); verb != "" {
		return helmTelemetryPrefix + "_" + verb
	}
	return helmTelemetryPrefix
}

func canonicalHelmVerb(cmdName string) string {
	verb := strings.ToLower(strings.TrimSpace(cmdName))
	if verb == "" || strings.ContainsAny(verb, ":/\\@ \t") {
		return ""
	}
	return canonicalHelmVerbs[verb]
}
