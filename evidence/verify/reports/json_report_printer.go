package reports

import (
	"encoding/json"
	"fmt"

	"github.com/jfrog/jfrog-cli-artifactory/evidence/model"
)

var JsonReportPrinter = &jsonReportPrinter{}

type jsonReportPrinter struct {
}

func (p *jsonReportPrinter) Print(result *model.VerificationResponse) error {
	err := verifyNotEmptyResponse(result)
	if err != nil {
		return err
	}
	resultJson, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(resultJson))

	return nil
}
