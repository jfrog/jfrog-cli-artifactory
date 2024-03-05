package cryptolib

import (
	"io/fs"
	"os"
	"strings"
)

func ReadKey(keyFiles []fs.DirEntry, path string) (*[]*SSLibKey, error) {
	var privateKeys []*SSLibKey
	for _, file := range keyFiles {
		if strings.HasPrefix(file.Name(), ".") {
			continue
		}
		content, err := os.ReadFile(path + "/" + file.Name())
		if err != nil {
			return nil, err
		}
		slibKey, err := LoadKey(content)
		if err != nil {
			return nil, err
		}
		if slibKey.KeyVal.Private != "" {
			privateKeys = append(privateKeys, slibKey)
			break // For now, we only support one private key
		}
	}
	return &privateKeys, nil
}
