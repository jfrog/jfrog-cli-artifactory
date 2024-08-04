package model

type GraphqlResponse struct {
	Data VersionsData `json:"data"`
}

type VersionsData struct {
	Versions Versions `json:"versions"`
}

type Versions struct {
	Edges []VersionEdges `json:"edges"`
}

type VersionEdges struct {
	Node VersionNode `json:"node"`
}

type VersionNode struct {
	Repos []Repo `json:"repos"`
}

type Repo struct {
	Name         string `json:"name"`
	LeadFilePath string `json:"leadFilePath"`
}
