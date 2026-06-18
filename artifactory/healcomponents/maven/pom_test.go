package maven

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParsePom_Aggregator(t *testing.T) {
	data := []byte(`<?xml version="1.0"?>
<project>
  <packaging>pom</packaging>
  <modules>
    <module>mod-a</module>
    <module>services/api</module>
  </modules>
</project>`)
	p, err := parsePom(data)
	require.NoError(t, err)
	assert.True(t, p.isAggregator())
	assert.Equal(t, []string{"mod-a", "services/api"}, p.modulePaths())
}

func TestParsePom_SingleModule(t *testing.T) {
	data := []byte(`<project><artifactId>app</artifactId></project>`)
	p, err := parsePom(data)
	require.NoError(t, err)
	assert.False(t, p.isAggregator())
	assert.Empty(t, p.modulePaths())
}

func TestParsePom_IgnoresCommentModules(t *testing.T) {
	data := []byte(`<project><!-- <modules>fake</modules> --><packaging>jar</packaging></project>`)
	p, err := parsePom(data)
	require.NoError(t, err)
	assert.False(t, p.isAggregator())
}
