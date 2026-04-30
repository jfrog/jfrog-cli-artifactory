package pnpm

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/jfrog/build-info-go/build"
	"github.com/jfrog/build-info-go/entities"
	buildUtils "github.com/jfrog/jfrog-cli-core/v2/common/build"
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/jfrog/jfrog-cli-core/v2/utils/coreutils"
	"github.com/jfrog/jfrog-client-go/utils/errorutils"
	"github.com/jfrog/jfrog-client-go/utils/log"
)

type PnpmInstallCommand struct {
	pnpmArgs           []string
	workingDirectory   string
	buildConfiguration *buildUtils.BuildConfiguration
	serverDetails      *config.ServerDetails
}

func NewPnpmInstallCommand() *PnpmInstallCommand {
	return &PnpmInstallCommand{}
}

func (pic *PnpmInstallCommand) SetArgs(args []string) *PnpmInstallCommand {
	pic.pnpmArgs = args
	return pic
}

func (pic *PnpmInstallCommand) SetBuildConfiguration(buildConfiguration *buildUtils.BuildConfiguration) *PnpmInstallCommand {
	pic.buildConfiguration = buildConfiguration
	return pic
}

func (pic *PnpmInstallCommand) SetServerDetails(serverDetails *config.ServerDetails) *PnpmInstallCommand {
	pic.serverDetails = serverDetails
	return pic
}

func (pic *PnpmInstallCommand) CommandName() string {
	return "rt_pnpm_install"
}

func (pic *PnpmInstallCommand) ServerDetails() (*config.ServerDetails, error) {
	return pic.serverDetails, nil
}

func (pic *PnpmInstallCommand) Run() error {
	log.Info("Running pnpm install...")
	var err error
	pic.workingDirectory, err = coreutils.GetWorkingDirectory()
	if err != nil {
		return err
	}
	log.Debug("Working directory set to:", pic.workingDirectory)

	collectBuildInfo, err := pic.buildConfiguration.IsCollectBuildInfo()
	if err != nil {
		return err
	}
	log.Debug("Collect build info:", collectBuildInfo)

	if err = pic.runPnpmInstall(); err != nil {
		return err
	}

	if collectBuildInfo {
		if err = pic.collectAndSaveBuildInfo(); err != nil {
			log.Warn("pnpm install completed successfully, but build info collection failed:", err.Error())
			return nil
		}
	}

	log.Info("pnpm install finished successfully.")
	return nil
}

func (pic *PnpmInstallCommand) runPnpmInstall() error {
	args := append([]string{"install"}, pic.pnpmArgs...)
	log.Debug("Running command: pnpm", strings.Join(args, " "))
	cmd := exec.Command("pnpm", args...)
	cmd.Dir = pic.workingDirectory
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return errorutils.CheckError(cmd.Run())
}

func (pic *PnpmInstallCommand) collectAndSaveBuildInfo() error {
	log.Info("Preparing for dependencies information collection... For the first run of the build, the dependencies collection may take a few minutes. Subsequent runs should be faster.")

	if pic.serverDetails == nil {
		return errorutils.CheckErrorf("no server configuration. Use 'jfrog config add' or specify --server-id")
	}
	log.Debug("Server details. Artifactory URL:", pic.serverDetails.ArtifactoryUrl)

	scopeToCurrentPackage := isPnpmWorkspaceSubPackage(pic.workingDirectory)
	if scopeToCurrentPackage && userRequestedWorkspaceRoot(pic.pnpmArgs) {
		log.Debug("--workspace-root/-w specified; collecting build-info for the entire workspace despite sub-package cwd.")
		scopeToCurrentPackage = false
	}
	log.Debug("Running pnpm ls to collect dependency tree...")
	projects, err := runPnpmLs(pic.workingDirectory, extractLsForwardFlags(pic.pnpmArgs), scopeToCurrentPackage)
	if err != nil {
		return err
	}
	modules := parsePnpmLsProjects(projects)

	if err = resolveChecksumsForModules(modules, pic.serverDetails, pic.buildConfiguration, pic.workingDirectory); err != nil {
		return err
	}

	return saveBuildInfo(modules, pic.buildConfiguration)
}

// isPnpmWorkspaceSubPackage returns true when workingDir is inside a pnpm workspace
// but is NOT the workspace root (i.e. the user invoked the command from a single
// workspace package). When true, build-info should be scoped to that package only.
//
// Detection is delegated to pnpm itself via `pnpm root -w`, which prints the
// workspace root's node_modules path when inside a workspace and errors with a
// non-zero exit code otherwise. Any error (not a workspace, pnpm missing, etc.)
// is logged at debug level and treated as "not a sub-package" — in that case
// the caller keeps the existing recursive behavior.
func isPnpmWorkspaceSubPackage(workingDir string) bool {
	cmd := exec.Command("pnpm", "root", "-w")
	cmd.Dir = workingDir
	// Split stdout/stderr so future pnpm warnings/deprecations on stderr can't
	// corrupt the path we parse from stdout. Both streams are surfaced in the
	// debug log on failure to preserve diagnostic coverage when the real error
	// reason can be on either stream.
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		log.Debug(fmt.Sprintf("pnpm workspace detection skipped (%s; stdout: %q; stderr: %q); falling back to recursive pnpm ls.",
			err.Error(),
			strings.TrimSpace(stdout.String()),
			strings.TrimSpace(stderr.String())))
		return false
	}
	workspaceRoot := filepath.Dir(strings.TrimSpace(stdout.String()))
	// filepath.Dir collapses "" / single-segment input to ".", so "." is the
	// only "no usable path" sentinel we need to guard against.
	if workspaceRoot == "." {
		log.Debug("pnpm root -w returned no usable path; falling back to recursive pnpm ls.")
		return false
	}
	// pnpm resolves symlinks in its output (e.g. on macOS /var → /private/var), so
	// compare canonicalized paths to avoid false "sub-package" detection when the
	// working directory happens to contain a symlink.
	if samePath(workspaceRoot, workingDir) {
		log.Debug("Invoked at pnpm workspace root; collecting build-info for all workspace packages.")
		return false
	}
	log.Debug(fmt.Sprintf("Invoked inside workspace sub-package (workspace root: %s); scoping build-info to current package.", workspaceRoot))
	return true
}

// samePath reports whether a and b refer to the same directory after resolving
// symlinks. If symlink resolution fails on either side (e.g. one path no longer
// exists on disk), it falls back to direct string equality — i.e. it returns
// false unless a == b verbatim. No further inference is attempted.
func samePath(a, b string) bool {
	if a == b {
		return true
	}
	resolvedA, errA := filepath.EvalSymlinks(a)
	resolvedB, errB := filepath.EvalSymlinks(b)
	if errA != nil || errB != nil {
		return false
	}
	return resolvedA == resolvedB
}

// lsForwardFlags maps `pnpm install` flags that affect the resolved dependency
// tree to whether the flag carries a value. Boolean flags (false) are forwarded
// as-is. Value-bearing flags (true) accept either `--flag value` or `--flag=value`.
// Anything not in this map is dropped — install-only flags (e.g. --frozen-lockfile)
// would just confuse `pnpm ls`.
//
// Notably absent: --no-optional. The build-info parser currently does not read
// pnpm ls's `optionalDependencies` JSON key (only `dependencies` and
// `devDependencies`), so forwarding --no-optional has no observable effect on
// build-info. Tracked as a follow-up to extend the parser; once that lands,
// --no-optional should be added back here.
var lsForwardFlags = map[string]bool{
	"--ignore-workspace": false,
	"--prod":             false,
	"-P":                 false,
	"--production":       false,
	"--dev":              false,
	"-D":                 false,
	"--workspace-root":   false,
	"-w":                 false,
	"--filter":           true,
	"-F":                 true,
}

// userRequestedWorkspaceRoot reports whether the user asked pnpm install to
// operate at the workspace root (via --workspace-root or its alias -w). When
// true, build-info must reflect the entire workspace even if invoked from a
// sub-package, so the cwd-based scope heuristic must be overridden. Conflict
// resolution between this and other flags (e.g. --ignore-workspace) is left
// to pnpm itself via the forwarded flags.
func userRequestedWorkspaceRoot(installArgs []string) bool {
	for _, arg := range installArgs {
		if arg == "--workspace-root" || arg == "-w" {
			return true
		}
	}
	return false
}

// extractLsForwardFlags returns flags from the user's pnpm install args that must
// be forwarded to `pnpm ls` so the dependency tree respects the same scope and
// dep-group filtering as the install command. Both `--flag=value` and
// `--flag value` forms are recognized for value-bearing flags.
func extractLsForwardFlags(installArgs []string) []string {
	var forwarded []string
	for i := 0; i < len(installArgs); i++ {
		arg := installArgs[i]

		if takesValue, ok := lsForwardFlags[arg]; ok {
			if !takesValue {
				forwarded = append(forwarded, arg)
				continue
			}
			// `--flag value` form — capture the next arg as the value when present.
			if i+1 < len(installArgs) {
				forwarded = append(forwarded, arg, installArgs[i+1])
				i++
			}
			continue
		}

		// `--flag=value` form — split on the first '=' and check the prefix.
		if eq := strings.IndexByte(arg, '='); eq > 0 {
			key, val := arg[:eq], arg[eq+1:]
			if takesValue, ok := lsForwardFlags[key]; ok {
				// Value-bearing flags (e.g. --filter=foo) always forward as-is.
				if takesValue {
					forwarded = append(forwarded, arg)
					continue
				}
				// Boolean flags with =value (e.g. --prod=true, --ignore-workspace=0):
				// drop only when strconv.ParseBool recognizes a falsy value
				// (false / 0 / F). Anything unparseable is forwarded as-is so pnpm
				// can decide. This matches jfrog-cli-core's FindBooleanFlag semantics.
				if parsed, err := strconv.ParseBool(val); err != nil || parsed {
					forwarded = append(forwarded, arg)
				}
			}
		}
	}
	return forwarded
}

// buildPnpmLsArgs assembles the argument list for `pnpm ls`. The `-r` flag is omitted
// when either (a) we are scoping output to the current working directory's package, or
// (b) the user forwarded --ignore-workspace — in that case pnpm emits one JSON array
// per project concatenated on stdout, which is not parseable as a single array.
func buildPnpmLsArgs(extraArgs []string, scopeToCurrentPackage bool) []string {
	args := []string{"ls"}
	if !scopeToCurrentPackage && !hasIgnoreWorkspace(extraArgs) {
		args = append(args, "-r")
	}
	args = append(args, "--depth", "Infinity", "--json")
	args = append(args, extraArgs...)
	return args
}

// hasIgnoreWorkspace reports whether `--ignore-workspace` is present in args,
// matching both the bare flag and the `--ignore-workspace=<value>` form. The
// value is parsed via strconv.ParseBool to match jfrog-cli-core's bool flag
// semantics; an unparseable value is conservatively treated as enabled and
// left to pnpm to validate.
func hasIgnoreWorkspace(args []string) bool {
	const prefix = "--ignore-workspace="
	for _, a := range args {
		if a == "--ignore-workspace" {
			return true
		}
		if strings.HasPrefix(a, prefix) {
			if parsed, err := strconv.ParseBool(a[len(prefix):]); err != nil || parsed {
				return true
			}
		}
	}
	return false
}

func runPnpmLs(workingDir string, extraArgs []string, scopeToCurrentPackage bool) ([]pnpmLsProject, error) {
	pnpmLsArgs := buildPnpmLsArgs(extraArgs, scopeToCurrentPackage)
	log.Info("Collecting dependency tree information. This may take a few minutes for large projects...")
	log.Debug("Running command: pnpm", strings.Join(pnpmLsArgs, " "))
	command := exec.Command("pnpm", pnpmLsArgs...)
	command.Dir = workingDir
	outBuffer := bytes.NewBuffer([]byte{})
	command.Stdout = outBuffer
	errBuffer := bytes.NewBuffer([]byte{})
	command.Stderr = errBuffer
	err := command.Run()
	errResult := errBuffer.String()
	if err != nil {
		return nil, errorutils.CheckErrorf("running pnpm ls: %s\n%s", err.Error(), strings.TrimSpace(errResult))
	}
	if errResult != "" {
		log.Debug("pnpm ls stderr:", errResult)
	}

	out := outBuffer.Bytes()

	var projects []pnpmLsProject
	if err = json.Unmarshal(out, &projects); err != nil {
		return nil, errorutils.CheckErrorf("parsing pnpm ls output: %s", err.Error())
	}
	return projects, nil
}

func saveBuildInfo(modules []*moduleInfo, buildConfiguration *buildUtils.BuildConfiguration) error {
	buildName, err := buildConfiguration.GetBuildName()
	if err != nil {
		return err
	}
	buildNumber, err := buildConfiguration.GetBuildNumber()
	if err != nil {
		return err
	}
	log.Debug(fmt.Sprintf("Saving build info for build: %s/%s", buildName, buildNumber))

	pnpmBuild, err := newBuild(buildConfiguration)
	if err != nil {
		return errorutils.CheckError(err)
	}

	customModule := buildConfiguration.GetModule()
	totalDeps := 0
	for _, mod := range modules {
		moduleID := mod.id
		if customModule != "" && len(modules) == 1 {
			moduleID = customModule
		}
		log.Debug(fmt.Sprintf("Saving module '%s' with %d dependencies.", moduleID, len(mod.dependencies)))
		partial := &entities.Partial{
			ModuleId:     moduleID,
			ModuleType:   entities.Npm,
			Dependencies: mod.dependencies,
		}
		if err = pnpmBuild.SavePartialBuildInfo(partial); err != nil {
			return err
		}
		totalDeps += len(mod.dependencies)
	}

	log.Info(fmt.Sprintf("Build info collected: %d module(s), %d total dependencies.", len(modules), totalDeps))
	return nil
}

func resolveChecksumsForModules(modules []*moduleInfo, serverDetails *config.ServerDetails, buildConfig *buildUtils.BuildConfiguration, workingDir string) error {
	allDeps := collectAllDepsFromModules(modules)
	if len(allDeps) == 0 {
		log.Debug("No dependencies found to resolve checksums for.")
		return nil
	}

	var parsedDeps []parsedDep
	skipped := 0
	for _, dep := range allDeps {
		resolved := dep.resolvedURL
		if resolved == "" || strings.HasPrefix(dep.version, "link:") {
			skipped++
			continue
		}
		parts, err := parseTarballURL(resolved)
		if err != nil {
			log.Debug(fmt.Sprintf("Could not parse resolved URL for %s:%s (%s), falling back to name-based path.", dep.name, dep.version, resolved))
			parts = buildTarballPartsFromName(dep.name, dep.version)
		}
		parsedDeps = append(parsedDeps, parsedDep{dep: dep, parts: parts})
	}

	if skipped > 0 {
		log.Debug(fmt.Sprintf("Skipped %d dependencies (no resolved URL or workspace link).", skipped))
	}

	if len(parsedDeps) == 0 {
		log.Debug("No dependencies with resolved URLs to fetch checksums for.")
		return nil
	}

	log.Debug(fmt.Sprintf("Fetching checksums for %d dependencies...", len(parsedDeps)))
	checksumMap, err := fetchChecksums(parsedDeps, serverDetails, buildConfig, workingDir)
	if err != nil {
		return err
	}

	resolved := 0
	for _, cs := range checksumMap {
		if !cs.IsEmpty() {
			resolved++
		}
	}
	log.Debug(fmt.Sprintf("Checksums resolved: %d/%d dependencies.", resolved, len(parsedDeps)))

	applyChecksumsToModules(modules, checksumMap)
	return nil
}

func collectAllDepsFromModules(modules []*moduleInfo) []depInfo {
	seen := make(map[string]struct{})
	var all []depInfo
	for _, mod := range modules {
		for _, dep := range mod.rawDeps {
			key := dep.name + ":" + dep.version
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			all = append(all, dep)
		}
	}
	return all
}

func applyChecksumsToModules(modules []*moduleInfo, checksumMap map[string]entities.Checksum) {
	for _, mod := range modules {
		for i, dep := range mod.dependencies {
			if cs, ok := checksumMap[dep.Id]; ok {
				mod.dependencies[i].Checksum = cs
			}
		}
	}
}

func newBuild(buildConfiguration *buildUtils.BuildConfiguration) (*build.Build, error) {
	pnpmBuild, err := buildUtils.PrepareBuildPrerequisites(buildConfiguration)
	if err != nil {
		return nil, err
	}
	if pnpmBuild == nil {
		return nil, errorutils.CheckErrorf("build info collection is not enabled")
	}
	return pnpmBuild, nil
}
