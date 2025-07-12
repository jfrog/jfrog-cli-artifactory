package get

import (
	"fmt"
	"os"

	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/jfrog/jfrog-client-go/utils/log"
)

type getEvidenceBase struct {
	serverDetails    *config.ServerDetails
	outputFileName   string
	format           string
	includePredicate bool
}

func (g *getEvidenceBase) exportEvidenceToFile(evidence []byte, outputFileName, format string) error {
	if outputFileName == "" {
		outputFileName = "evidences"
	}

	if format == "" {
		format = "json"
	}

	switch format {
	case "json":
		return exportEvidenceToJsonFile(evidence, outputFileName)
	default:
		log.Error("Unsupported format. Supported formats are: json")
		return fmt.Errorf("unsupported format: %s", format)
	}
}

func exportEvidenceToJsonFile(evidence []byte, outputFileName string) error {
	file, err := os.Create(outputFileName)
	if err != nil {
		return err
	}

	defer file.Close()

	_, err = file.Write(evidence)
	if err != nil {
		return err
	}

	log.Info("Evidence successfully exported to", outputFileName)
	return nil
}
