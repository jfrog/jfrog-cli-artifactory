package model

import "github.com/jfrog/froggit-go/vcsclient"

type GitLogEntry struct {
	Commit            string `json:"commit,omitempty"`
	AbbreviatedCommit string `json:"abbreviated_commit,omitempty"`
	Tree              string `json:"tree,omitempty"`
	AbbreviatedTree   string `json:"abbreviated_tree,omitempty"`
	Parent            string `json:"parent,omitempty"`
	AbbreviatedParent string `json:"abbreviated_parent,omitempty"`
	Subject           string `json:"subject,omitempty"`
	SanitizedSubject  string `json:"sanitized_subject_line,omitempty"`
	Author            struct {
		Name  string `json:"name,omitempty"`
		Email string `json:"email,omitempty"`
		Date  string `json:"date,omitempty"`
	} `json:"author,omitempty"`
	Commiter struct {
		Name  string `json:"name,omitempty"`
		Email string `json:"email,omitempty"`
		Date  string `json:"date,omitempty"`
	} `json:"commiter,omitempty"`
	PRreviewer []vcsclient.PullRequestReviewDetails `json:"pr_reviewer,omitempty"`
}
