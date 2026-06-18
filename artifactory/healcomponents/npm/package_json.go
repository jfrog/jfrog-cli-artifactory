package npm

import "encoding/json"

type packageJSON struct {
	Workspaces json.RawMessage `json:"workspaces"`
}

func parsePackageJSON(data []byte) (packageJSON, error) {
	var p packageJSON
	if err := json.Unmarshal(data, &p); err != nil {
		return packageJSON{}, err
	}
	return p, nil
}

func (p packageJSON) hasWorkspaces() bool {
	if len(p.Workspaces) == 0 || string(p.Workspaces) == "null" {
		return false
	}
	if string(p.Workspaces) == "[]" {
		return false
	}
	var arr []any
	if json.Unmarshal(p.Workspaces, &arr) == nil {
		return len(arr) > 0
	}
	var obj struct {
		Packages []any `json:"packages"`
	}
	if json.Unmarshal(p.Workspaces, &obj) == nil {
		return len(obj.Packages) > 0
	}
	return true
}
