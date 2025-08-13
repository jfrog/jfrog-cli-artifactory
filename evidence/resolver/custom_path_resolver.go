package resolver

import "fmt"

type CustomPathResolver struct {
	SubjectRepoPath string
}

func NewCustomPathResolver(subjectRepoPath string) *CustomPathResolver {
	return &CustomPathResolver{
		SubjectRepoPath: subjectRepoPath,
	}
}

func (c *CustomPathResolver) ResolveSubjectRepoPath() (string, error) {
	if c.SubjectRepoPath == "" {
		return "", fmt.Errorf("subject repository path is required but not provided")
	}
	return c.SubjectRepoPath, nil
}
