package container_test

import (
	buildCreate "github.com/jfrog/jfrog-cli-artifactory/artifactory/commands/container"
	container "github.com/jfrog/jfrog-cli-artifactory/artifactory/commands/ocicontainer"
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

	t.Run("Docker Buildx Format - No Spaces", func(t *testing.T) {
		// This is the exact format from docker buildx metadata file (issue #197)
		img := container.NewImage("myorg.jfrog.io/myrepo/jfrog/myimage:tag1,myorg.jfrog.io/myrepo/jfrog/myimage:tag2")
		images := buildCreate.SplitMultiTagDockerImageStringWithComma(img)

		assert.Equal(t, 2, len(images))
		assert.Equal(t, "myorg.jfrog.io/myrepo/jfrog/myimage:tag1", images[0].Name())
		assert.Equal(t, "myorg.jfrog.io/myrepo/jfrog/myimage:tag2", images[1].Name())
	})

	t.Run("Multiple Tags With Registry", func(t *testing.T) {
		img := container.NewImage("registry.example.com:5000/repo/image:v1,registry.example.com:5000/repo/image:v2,registry.example.com:5000/repo/image:latest")
		images := buildCreate.SplitMultiTagDockerImageStringWithComma(img)

		assert.Equal(t, 3, len(images))
		assert.Equal(t, "registry.example.com:5000/repo/image:v1", images[0].Name())
		assert.Equal(t, "registry.example.com:5000/repo/image:v2", images[1].Name())
		assert.Equal(t, "registry.example.com:5000/repo/image:latest", images[2].Name())
	})

	t.Run("Mixed Spaces Format", func(t *testing.T) {
		// Some tags with spaces, some without
		img := container.NewImage("repo/image:tag1, repo/image:tag2,repo/image:tag3")
		images := buildCreate.SplitMultiTagDockerImageStringWithComma(img)

		assert.Equal(t, 3, len(images))
		assert.Equal(t, "repo/image:tag1", images[0].Name())
		assert.Equal(t, "repo/image:tag2", images[1].Name())
		assert.Equal(t, "repo/image:tag3", images[2].Name())
	})

	t.Run("Tags In Different Repositories", func(t *testing.T) {
		// Test case where tags are in different repositories
		// This tests the scenario where repo from image should take precedence
		img := container.NewImage("myorg.jfrog.io/repo1/image:tag1,myorg.jfrog.io/repo2/image:tag2,myorg.jfrog.io/repo1/image:tag3")
		images := buildCreate.SplitMultiTagDockerImageStringWithComma(img)

		assert.Equal(t, 3, len(images))
		assert.Equal(t, "myorg.jfrog.io/repo1/image:tag1", images[0].Name())
		assert.Equal(t, "myorg.jfrog.io/repo2/image:tag2", images[1].Name())
		assert.Equal(t, "myorg.jfrog.io/repo1/image:tag3", images[2].Name())
	})
}
