package utils

import (
	"fmt"
	"os"
)

func GetEnvVariable(envVarName string) (string, error) {
	if key, exists := os.LookupEnv(envVarName); exists {
		return key, nil
	}
	return "", fmt.Errorf("'%s'  field wasn't provided.", envVarName)
}
