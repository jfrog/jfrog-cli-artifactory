package resolver

import (
	"fmt"

	"github.com/jfrog/jfrog-cli-artifactory/evidence/utils"
)

type ReleaseBundlePathResolver struct {
	project              string
	releaseBundle        string
	releaseBundleVersion string
}

func NewReleaseBundlePathResolver(project, releaseBundle, releaseBundleVersion string) *ReleaseBundlePathResolver {
	return &ReleaseBundlePathResolver{
		project:              project,
		releaseBundle:        releaseBundle,
		releaseBundleVersion: releaseBundleVersion,
	}
}

func (r *ReleaseBundlePathResolver) ResolveSubjectRepoPath() (string, error) {
	repoKey := utils.BuildReleaseBundleRepoKey(r.project)

	path := fmt.Sprintf("%s/%s", r.releaseBundle, r.releaseBundleVersion)

	subjectPath := fmt.Sprintf("%s/%s/%s", repoKey, path, "release-bundle.json.evd")

	return subjectPath, nil
}
