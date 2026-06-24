package healcomponents

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

type fakeTool struct {
	name     string
	commands []string
}

func (f fakeTool) ToolName() string                       { return f.name }
func (f fakeTool) RelevantCommands() []string             { return f.commands }
func (f fakeTool) ProjectRoot(dir string) (string, error) { return dir, nil }
func (f fakeTool) EnsureLockfiles(_ context.Context, _, _ string, _ CommandRunner, _ ...string) ([]string, error) {
	return nil, nil
}
func (f fakeTool) DiscoverLockfiles(_ string) ([]Lockfile, error) {
	return []Lockfile{{Path: "lock.json", Content: []byte(`{}`)}}, nil
}

func TestIsRelevantCommand(t *testing.T) {
	tool := fakeTool{name: "fake", commands: []string{"install", "ci"}}
	assert.True(t, IsRelevantCommand(tool, "install"))
	assert.True(t, IsRelevantCommand(tool, "ci"))
	assert.False(t, IsRelevantCommand(tool, "publish"))
}
