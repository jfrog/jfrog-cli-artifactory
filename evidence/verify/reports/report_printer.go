package reports

import "github.com/jfrog/jfrog-cli-artifactory/evidence/model"

type ReportPrinter interface {
	Print(result *model.VerificationResponse) error
}
