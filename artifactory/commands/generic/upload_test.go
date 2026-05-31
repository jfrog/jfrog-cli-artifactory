package generic

import (
	"testing"

	"github.com/jfrog/jfrog-cli-artifactory/artifactory/utils/civcs"
	"github.com/jfrog/jfrog-cli-core/v2/common/spec"
	"github.com/stretchr/testify/assert"
)

func TestEnrichUploadFileProps_MergesDetectedVcsProps(t *testing.T) {
	file := &spec.File{Pattern: "dist/*.zip", Props: "team=payments", TargetProps: "tier=backend"}

	calledPattern := ""
	mergeVcsPropsForUpload = func(userProps, sourcePattern string) string {
		calledPattern = sourcePattern
		return userProps + ";vcs.revision=abc123"
	}
	defer func() { mergeVcsPropsForUpload = civcs.MergeWithUserAndDetectedProps }()

	enrichUploadFileProps(file, "")
	assert.Equal(t, "dist/*.zip", calledPattern)
	assert.Contains(t, file.TargetProps, "team=payments")
	assert.Contains(t, file.TargetProps, "vcs.revision=abc123")
}

func TestEnrichUploadFileProps_AddsSyncDeletesProp(t *testing.T) {
	file := &spec.File{Pattern: "dist/*.zip", Props: "team=payments", TargetProps: "tier=backend"}
	mergeVcsPropsForUpload = func(userProps, _ string) string {
		return userProps
	}
	defer func() { mergeVcsPropsForUpload = civcs.MergeWithUserAndDetectedProps }()

	enrichUploadFileProps(file, "sync.deletes.timestamp=123")
	assert.Contains(t, file.TargetProps, "sync.deletes.timestamp=123")
	assert.Contains(t, file.Props, "sync.deletes.timestamp=123")
}
