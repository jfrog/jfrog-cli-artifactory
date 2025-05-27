package container_test

import (
	buildCreate "github.com/jfrog/jfrog-cli-artifactory/artifactory/commands/container"
	"github.com/jfrog/jfrog-cli-core/v2/artifactory/utils/container"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestSplitMultiTagDockerImageStringWithComma(t *testing.T) {
	t.Run("Multiple Tags", func(t *testing.T) {
		img := container.NewImage("repo/image:tag1, repo/image:tag2")
		images := buildCreate.SplitMultiTagDockerImageStringWithComma(img)

		assert.Equal(t, 2, len(images))
		assert.Equal(t, "repo/image:tag1", images[0].Name())
		assert.Equal(t, "repo/image:tag2", images[1].Name())
	})

	t.Run("Single Tag", func(t *testing.T) {
		img := container.NewImage("repo/image:tag1")
		images := buildCreate.SplitMultiTagDockerImageStringWithComma(img)

		assert.Equal(t, 1, len(images))
		assert.Equal(t, "repo/image:tag1", images[0].Name())
	})

	t.Run("Empty Tag", func(t *testing.T) {
		img := container.NewImage("repo/image:tag1, , repo/image:tag2")
		images := buildCreate.SplitMultiTagDockerImageStringWithComma(img)

		assert.Equal(t, 2, len(images))
		assert.Equal(t, "repo/image:tag1", images[0].Name())
		assert.Equal(t, "repo/image:tag2", images[1].Name())
	})

	t.Run("All Empty Tags", func(t *testing.T) {
		img := container.NewImage(", , ")
		images := buildCreate.SplitMultiTagDockerImageStringWithComma(img)

		assert.Equal(t, 0, len(images))
	})
}
