package agentcommon

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

type semverParts struct {
	Major int
	Minor int
	Patch int
	Raw   string
}

// LatestVersion returns the greatest semver from a list of version strings.
func LatestVersion(versions []string) (string, error) {
	if len(versions) == 0 {
		return "", fmt.Errorf("no versions available")
	}
	parsed := make([]semverParts, 0, len(versions))
	for _, version := range versions {
		parsedSemver, err := parseSemver(version)
		if err != nil {
			continue
		}
		parsed = append(parsed, parsedSemver)
	}
	if len(parsed) == 0 {
		return "", fmt.Errorf("no valid semver versions found")
	}
	sort.Slice(parsed, func(i, j int) bool {
		if parsed[i].Major != parsed[j].Major {
			return parsed[i].Major < parsed[j].Major
		}
		if parsed[i].Minor != parsed[j].Minor {
			return parsed[i].Minor < parsed[j].Minor
		}
		return parsed[i].Patch < parsed[j].Patch
	})
	return parsed[len(parsed)-1].Raw, nil
}

// NextMinorVersion returns the next minor version with patch reset to 0 (e.g. "1.2.3" -> "1.3.0").
func NextMinorVersion(version string) (string, error) {
	parsedSemver, err := parseSemver(version)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%d.%d.0", parsedSemver.Major, parsedSemver.Minor+1), nil
}

func parseSemver(version string) (semverParts, error) {
	versionWithoutPrefix := strings.TrimPrefix(version, "v")
	parts := strings.SplitN(versionWithoutPrefix, ".", 3)
	if len(parts) != 3 {
		return semverParts{}, fmt.Errorf("invalid semver: %s", version)
	}
	major, err := strconv.Atoi(parts[0])
	if err != nil {
		return semverParts{}, fmt.Errorf("invalid major version in %s: %w", version, err)
	}
	minor, err := strconv.Atoi(parts[1])
	if err != nil {
		return semverParts{}, fmt.Errorf("invalid minor version in %s: %w", version, err)
	}
	patchStr := strings.SplitN(parts[2], "-", 2)[0]
	patch, err := strconv.Atoi(patchStr)
	if err != nil {
		return semverParts{}, fmt.Errorf("invalid patch version in %s: %w", version, err)
	}
	return semverParts{Major: major, Minor: minor, Patch: patch, Raw: version}, nil
}
