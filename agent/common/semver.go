package common

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
		versionParts, err := parseSemver(version)
		if err != nil {
			continue
		}
		parsed = append(parsed, versionParts)
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

// CompareSemver compares two semver strings using the same parsing rules as LatestVersion.
// Returns negative if firstVersion < secondVersion, zero if equal, positive if firstVersion > secondVersion.
// Non-parseable values return an error.
func CompareSemver(firstVersion, secondVersion string) (int, error) {
	firstVersionParts, err := parseSemver(strings.TrimSpace(firstVersion))
	if err != nil {
		return 0, fmt.Errorf("compare semver: invalid first version %q: %w", firstVersion, err)
	}
	secondVersionParts, err := parseSemver(strings.TrimSpace(secondVersion))
	if err != nil {
		return 0, fmt.Errorf("compare semver: invalid second version %q: %w", secondVersion, err)
	}
	if firstVersionParts.Major != secondVersionParts.Major {
		return firstVersionParts.Major - secondVersionParts.Major, nil
	}
	if firstVersionParts.Minor != secondVersionParts.Minor {
		return firstVersionParts.Minor - secondVersionParts.Minor, nil
	}
	if firstVersionParts.Patch != secondVersionParts.Patch {
		return firstVersionParts.Patch - secondVersionParts.Patch, nil
	}
	return 0, nil
}

// NextMinorVersion takes a semver string and returns the next minor version
// with patch reset to 0 (e.g. "1.2.3" -> "1.3.0").
func NextMinorVersion(version string) (string, error) {
	versionParts, err := parseSemver(version)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%d.%d.0", versionParts.Major, versionParts.Minor+1), nil
}

// ValidateSemver checks that version is a valid semantic version (e.g. 1.2.0, 1.2.3-rc.1).
// It uses the same parsing rules as LatestVersion and CompareSemver, and rejects path-unsafe values.
func ValidateSemver(version string) error {
	if version == "" {
		return fmt.Errorf("version must not be empty")
	}
	if strings.Contains(version, "..") {
		return fmt.Errorf("invalid version %q: must not contain '..'", version)
	}
	if strings.ContainsAny(version, "/\\") {
		return fmt.Errorf("invalid version %q: must not contain path separators", version)
	}
	if _, err := parseSemver(version); err != nil {
		return fmt.Errorf("invalid semver version %q: %w", version, err)
	}
	return nil
}

func parseSemver(version string) (semverParts, error) {
	versionWithoutPrefix := strings.TrimPrefix(version, "v")
	versionSegments := strings.SplitN(versionWithoutPrefix, ".", 3)
	if len(versionSegments) != 3 {
		return semverParts{}, fmt.Errorf("invalid semver: %s", version)
	}

	major, err := strconv.Atoi(versionSegments[0])
	if err != nil {
		return semverParts{}, fmt.Errorf("invalid major version in %s: %w", version, err)
	}
	minor, err := strconv.Atoi(versionSegments[1])
	if err != nil {
		return semverParts{}, fmt.Errorf("invalid minor version in %s: %w", version, err)
	}

	// Patch may contain pre-release or build metadata; take numeric part only for comparison
	patchSegment := strings.SplitN(versionSegments[2], "-", 2)[0]
	patchSegment = strings.SplitN(patchSegment, "+", 2)[0]
	patch, err := strconv.Atoi(patchSegment)
	if err != nil {
		return semverParts{}, fmt.Errorf("invalid patch version in %s: %w", version, err)
	}

	return semverParts{Major: major, Minor: minor, Patch: patch, Raw: version}, nil
}
