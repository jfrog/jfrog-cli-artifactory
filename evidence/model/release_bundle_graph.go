package model

import "time"

type GraphResponse struct {
	Root Root `json:"root,omitempty"`
}

// Root corresponds to the "root" key.
// All fields are pointers to allow for null values.
type Root struct {
	Type          string `json:"type,omitempty"`
	Name          string `json:"name,omitempty"`
	Version       string `json:"version,omitempty"`
	Repository    string `json:"repository,omitempty"`
	CreatedMillis int64  `json:"created_millis,omitempty"`
	Nodes         []Node `json:"nodes,omitempty"`
	Evidence      []any  `json:"evidence,omitempty"`
}

// Node represents items in the "nodes" array (recursively).
// Again, all fields are pointers to allow them to be nil.
type Node struct {
	Type          string `json:"type,omitempty"`
	Name          string `json:"name,omitempty"`
	Version       string `json:"version,omitempty"`
	Repository    string `json:"repository,omitempty"`
	CreatedMillis int64  `json:"created_millis,omitempty"`
	Nodes         []Node `json:"nodes,omitempty"`
	Evidence      []any  `json:"evidence,omitempty"`
	PackageID     string `json:"package_id,omitempty"`
}

type ReleaseBundle struct {
	Status               string    `json:"status"`
	RepositoryKey        string    `json:"repository_key"`
	ReleaseBundleName    string    `json:"release_bundle_name"`
	ReleaseBundleVersion string    `json:"release_bundle_version"`
	ServiceID            string    `json:"service_id"`
	CreatedBy            string    `json:"created_by"`
	Created              time.Time `json:"created"`
}

type ReleaseBundlesResponse struct {
	ReleaseBundles []ReleaseBundle `json:"release_bundles"`
	Total          int             `json:"total"`
	Limit          int             `json:"limit"`
	Offset         int             `json:"offset"`
}
