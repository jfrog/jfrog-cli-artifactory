package helm

import (
	"fmt"
	"github.com/jfrog/build-info-go/entities"
	"github.com/jfrog/jfrog-client-go/utils/log"
	"helm.sh/helm/v3/pkg/chart/loader"
	"helm.sh/helm/v3/pkg/registry"
	orasregistry "oras.land/oras-go/v2/registry"
	"os/exec"
	"strings"
)

func needBuildInfo(cmdName string) bool {
	buildInfoNeededCommands := map[string]bool{
		"dependency": true,
		"package":    true,
		"push":       true,
	}
	return buildInfoNeededCommands[cmdName]
}

func appendModuleAndBuildAgentIfAbsent(buildInfo *entities.BuildInfo, chartName string, chartVersion string) {
	if buildInfo == nil {
		log.Debug("No build info collected, skipping further processing")
		return
	}
	if len(buildInfo.Modules) == 0 {
		module := entities.Module{
			Id:   fmt.Sprintf("%s:%s", chartName, chartVersion),
			Type: "helm",
		}
		buildInfo.Modules = append(buildInfo.Modules, module)
	}
	if buildInfo.BuildAgent == nil || buildInfo.BuildAgent.Version == "" {
		if buildInfo.BuildAgent == nil {
			buildInfo.BuildAgent = &entities.Agent{}
		}
		buildInfo.BuildAgent.Name = "Helm"
		buildInfo.BuildAgent.Version = getHelmVersion()
	}
}

func getHelmVersion() string {
	cmd := exec.Command("helm", "version", "--short")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "unknown"
	}
	versionStr := strings.TrimSpace(string(output))
	// Remove 'v' prefix if present
	versionStr = strings.TrimPrefix(versionStr, "v")
	// Remove any build metadata after '+'
	if idx := strings.Index(versionStr, "+"); idx != -1 {
		versionStr = versionStr[:idx]
	}
	return versionStr
}

func getChartDetails(filePath string) (string, string, error) {
	chart, err := loader.Load(filePath)
	if err != nil {
		return "", "", err
	}
	name := chart.Metadata.Name
	version := chart.Metadata.Version
	return name, version, nil
}

// getUploadedFileDeploymentPath extracts the deployment path from the OCI registry URL argument
func getUploadedFileDeploymentPath(registryURL string) string {
	if registryURL == "" {
		return ""
	}
	raw := strings.TrimPrefix(registryURL, registry.OCIScheme+"://")
	ref, err := parseOCIReference(raw)
	if err != nil {
		log.Debug("Failed to parse OCI reference ", registryURL, " : ", err)
		return ""
	}
	return ref.Repository
}

func getPushChartPathAndRegistryURL(helmArgs []string) (chartPath, registryURL string) {
	var positionalArgs []string
	for _, arg := range helmArgs {
		if arg == "push" || strings.HasPrefix(arg, "-") {
			continue
		}
		positionalArgs = append(positionalArgs, arg)
	}
	if len(positionalArgs) > 0 {
		chartPath = positionalArgs[0]
	}
	if len(positionalArgs) > 1 {
		registryURL = positionalArgs[1]
	}
	return
}

// parseOCIReference parses an OCI reference using the same approach as Helm SDK
func parseOCIReference(raw string) (*ociReference, error) {
	orasRef, err := orasregistry.ParseReference(raw)
	if err != nil {
		return nil, err
	}
	return &ociReference{
		Registry:   orasRef.Registry,
		Repository: orasRef.Repository,
		Reference:  orasRef.Reference,
	}, nil
}

// ociReference represents a parsed OCI reference (similar to Helm SDK's reference struct)
type ociReference struct {
	Registry   string
	Repository string
	Reference  string
}

func getPaths(helmArgs []string) []string {
	var paths []string
	for _, arg := range helmArgs {
		if !strings.HasPrefix(arg, "-") {
			paths = append(paths, arg)
		}
	}
	return paths
}

func removeDuplicateDependencies(buildInfo *entities.BuildInfo) {
	if buildInfo == nil {
		return
	}
	for moduleIdx, module := range buildInfo.Modules {
		dependenciesMap := make(map[string]entities.Dependency)
		var dependencies []entities.Dependency
		for _, dependency := range module.Dependencies {
			sha256 := dependency.Sha256
			if sha256 == "" {
				log.Debug("Missing Sha256 for dependency: ", dependency.Id, "so, skipping it from adding into build info.")
			}
			_, exist := dependenciesMap[sha256]
			if sha256 != "" && !exist {
				dependenciesMap[sha256] = dependency
				dependencies = append(dependencies, dependency)
			}
		}
		module.Dependencies = dependencies
		buildInfo.Modules[moduleIdx] = module
	}
}

func addArtifactsInBuildInfo(buildInfo *entities.BuildInfo, artifacts []entities.Artifact, chartName, chartVersion string) {
	if buildInfo == nil {
		return
	}
	moduleId := fmt.Sprintf("%s:%s", chartName, chartVersion)
	for moduleIdx, module := range buildInfo.Modules {
		if module.Id == moduleId {
			module.Artifacts = append(module.Artifacts, artifacts...)
			buildInfo.Modules[moduleIdx] = module
		}
	}
}

func removeDuplicateArtifacts(buildInfo *entities.BuildInfo) {
	if buildInfo == nil {
		return
	}
	for moduleIdx, module := range buildInfo.Modules {
		artifactsMap := make(map[string]entities.Artifact)
		var artifacts []entities.Artifact
		for _, artifact := range module.Artifacts {
			sha256 := artifact.Sha256
			if sha256 == "" {
				log.Debug("Missing Sha256 for artifact: ", artifact.Name, "so, skipping it from adding into build info.")
			}
			_, exist := artifactsMap[sha256]
			if sha256 != "" && !exist {
				artifactsMap[sha256] = artifact
				artifacts = append(artifacts, artifact)
			}
		}
		module.Artifacts = artifacts
		buildInfo.Modules[moduleIdx] = module
	}
}

func appendModuleInExistingBuildInfo(buildInfo *entities.BuildInfo, moduleToAdd *entities.Module) {
	if buildInfo == nil || moduleToAdd == nil {
		return
	}
	for moduleIdx, module := range buildInfo.Modules {
		if module.Id == moduleToAdd.Id {
			dependencies := moduleToAdd.Dependencies
			if len(dependencies) > 0 {
				buildInfo.Modules[moduleIdx].Dependencies = append(buildInfo.Modules[moduleIdx].Dependencies, dependencies...)
			}
			artifacts := moduleToAdd.Artifacts
			if len(artifacts) > 0 {
				buildInfo.Modules[moduleIdx].Artifacts = artifacts
			}
			return
		}
	}
	buildInfo.Modules = append(buildInfo.Modules, *moduleToAdd)
}
