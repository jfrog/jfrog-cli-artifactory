package externalproviders

import (
	"github.com/jfrog/jfrog-client-go/utils/errorutils"
	"github.com/jfrog/jfrog-client-go/utils/io/fileutils"
	"github.com/jfrog/jfrog-client-go/utils/log"
	"gopkg.in/yaml.v3"
	"os"
)

func LoadConfig(path string) (map[string]*yaml.Node, error) {
	log.Debug("Loading external provider config", path)
	_, err := fileutils.IsFileExists(path, false)
	if err != nil {
		return nil, errorutils.CheckError(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var root yaml.Node
	if err := yaml.Unmarshal(data, &root); err != nil {
		return nil, err
	}
	evidenceConfig := make(map[string]*yaml.Node)
	if root.Kind == yaml.DocumentNode && len(root.Content) > 0 {
		root = *root.Content[0]
	}
	for i := 0; i < len(root.Content); i += 2 {
		key := root.Content[i].Value
		val := root.Content[i+1]
		evidenceConfig[key] = val
	}
	return evidenceConfig, nil
}
