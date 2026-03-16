package pnpm

import (
	"fmt"
	"strings"

	"github.com/jfrog/build-info-go/entities"
	"github.com/jfrog/jfrog-client-go/utils/log"
)

type pnpmLsProject struct {
	Name            string                       `json:"name"`
	Version         string                       `json:"version"`
	Path            string                       `json:"path"`
	Dependencies    map[string]pnpmLsDependency  `json:"dependencies"`
	DevDependencies map[string]pnpmLsDependency  `json:"devDependencies"`
}

type pnpmLsDependency struct {
	From         string                       `json:"from"`
	Version      string                       `json:"version"`
	Resolved     string                       `json:"resolved"`
	Path         string                       `json:"path"`
	Dependencies map[string]pnpmLsDependency  `json:"dependencies"`
}

func parsePnpmLsProjects(projects []pnpmLsProject) []*moduleInfo {
	var modules []*moduleInfo
	for _, proj := range projects {
		log.Debug(fmt.Sprintf("Parsing project: %s@%s (path: %s)", proj.Name, proj.Version, proj.Path))
		mod := parseSingleProject(proj)
		if mod != nil {
			log.Debug(fmt.Sprintf("Module '%s': %d dependencies, %d raw deps.", mod.id, len(mod.dependencies), len(mod.rawDeps)))
			modules = append(modules, mod)
		} else {
			log.Debug(fmt.Sprintf("Project '%s' has no dependencies, skipping.", proj.Name))
		}
	}
	return modules
}

// formatModuleId produces a module ID consistent with npm's BuildInfoModuleId:
//   - unscoped: name:version  (e.g. "lodash:4.17.21")
//   - scoped:   scope:name:version  (e.g. "jscope:pkg:1.0.0")
//
// Leading "v" or "=" prefixes on the version are stripped.
func formatModuleId(name, version string) string {
	if name == "" || version == "" {
		return name
	}
	version = strings.TrimPrefix(version, "v")
	version = strings.TrimPrefix(version, "=")
	if strings.HasPrefix(name, "@") {
		parts := strings.SplitN(name, "/", 2)
		if len(parts) == 2 {
			return strings.TrimPrefix(parts[0], "@") + ":" + parts[1] + ":" + version
		}
	}
	return name + ":" + version
}

func parseSingleProject(proj pnpmLsProject) *moduleInfo {
	moduleID := formatModuleId(proj.Name, proj.Version)
	if moduleID == "" {
		moduleID = "pnpm-project"
	}

	depMap := make(map[string]*depInfo)
	rootRequestor := moduleID

	walkDependencies(proj.Dependencies, "prod", rootRequestor, depMap)
	walkDependencies(proj.DevDependencies, "dev", rootRequestor, depMap)

	if len(depMap) == 0 {
		return nil
	}

	mod := &moduleInfo{id: moduleID}
	for _, dep := range depMap {
		mod.rawDeps = append(mod.rawDeps, *dep)
		mod.dependencies = append(mod.dependencies, entities.Dependency{
			Id:          dep.name + ":" + dep.version,
			Type:        "tgz",
			Scopes:      dep.scopes,
			RequestedBy: dep.requestedBy,
		})
	}
	return mod
}

func walkDependencies(deps map[string]pnpmLsDependency, scope, parentID string, depMap map[string]*depInfo) {
	for name, info := range deps {
		walkSingleDep(name, info, scope, parentID, depMap)
	}
}

func walkSingleDep(name string, info pnpmLsDependency, scope, parentID string, depMap map[string]*depInfo) {
	if strings.HasPrefix(info.Version, "link:") {
		log.Debug(fmt.Sprintf("Skipping workspace link dependency: %s (version: %s)", name, info.Version))
		return
	}

	key := name + ":" + info.Version
	requestedByPath := []string{parentID}

	if existing, ok := depMap[key]; ok {
		addRequestedBy(existing, requestedByPath)
		addScope(existing, scope)
		return // already walked children for this dep
	}

	dep := &depInfo{
		name:        name,
		version:     info.Version,
		resolvedURL: info.Resolved,
		scopes:      []string{scope},
		requestedBy: [][]string{requestedByPath},
	}
	depMap[key] = dep

	for childName, childInfo := range info.Dependencies {
		walkSingleDep(childName, childInfo, "transitive", name+"@"+info.Version, depMap)
	}
}

func addRequestedBy(dep *depInfo, path []string) {
	for _, existing := range dep.requestedBy {
		if slicesEqual(existing, path) {
			return
		}
	}
	dep.requestedBy = append(dep.requestedBy, path)
}

func addScope(dep *depInfo, scope string) {
	for _, s := range dep.scopes {
		if s == scope {
			return
		}
	}
	dep.scopes = append(dep.scopes, scope)
}

func slicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
