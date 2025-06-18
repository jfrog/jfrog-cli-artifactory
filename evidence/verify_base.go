package evidence

import (
	"encoding/json"
	"fmt"
	"io"

	gofrogio "github.com/jfrog/gofrog/io"
	"github.com/jfrog/jfrog-client-go/onemodel"

	"github.com/gookit/color"
	"github.com/jfrog/jfrog-cli-artifactory/evidence/cryptox"
	"github.com/jfrog/jfrog-cli-artifactory/evidence/model"
	"github.com/jfrog/jfrog-cli-core/v2/artifactory/utils"
	coreConfig "github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/jfrog/jfrog-cli-core/v2/utils/coreutils"
	"github.com/jfrog/jfrog-client-go/artifactory"
	servicesUtils "github.com/jfrog/jfrog-client-go/artifactory/services/utils"
	clientlog "github.com/jfrog/jfrog-client-go/utils/log"
)

var success = color.Green.Render("success")
var failed = color.Red.Render("failed")

const searchEvidenceQuery = `{"query":"{ evidence { searchEvidence( where: { hasSubjectWith: { repositoryKey: \"%s\", path: \"%s\", name: \"%s\" }} ) { edges { cursor node { downloadPath predicateType predicateCategory createdAt createdBy subject { sha256 } } } } } }"}`

// verifyEvidenceBase provides shared logic for evidence verification commands.
type verifyEvidenceBase struct {
	serverDetails     *coreConfig.ServerDetails
	format            string
	keys              []string
	artifactoryClient *artifactory.ArtifactoryServicesManager
	oneModelClient    onemodel.Manager
}

// printVerifyResult prints the verification result in the requested format.
func (v *verifyEvidenceBase) printVerifyResult(result *model.VerificationResponse) error {
	switch v.format {
	case "json":
		return printJson(result)
	case "full":
		return printFullText(result)
	default:
		return printText(result)
	}
}

// verifyEvidences runs the verification process for the given evidence metadata and subject sha256.
func (v *verifyEvidenceBase) verifyEvidences(client *artifactory.ArtifactoryServicesManager, evidenceMetadata *[]model.SearchEvidenceEdge, sha256 string) error {
	verifier := cryptox.NewVerifier(v.keys, client)
	verify, err := verifier.Verify(sha256, evidenceMetadata)
	if err != nil {
		return err
	}
	return v.printVerifyResult(verify)
}

// createArtifactoryClient creates an Artifactory client for evidence operations.
func (v *verifyEvidenceBase) createArtifactoryClient() (*artifactory.ArtifactoryServicesManager, error) {
	if v.artifactoryClient != nil {
		return v.artifactoryClient, nil
	}
	artifactoryClient, err := utils.CreateUploadServiceManager(v.serverDetails, 1, 0, 0, false, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create Artifactory client: %w", err)
	}
	v.artifactoryClient = &artifactoryClient
	return v.artifactoryClient, nil
}

// queryEvidenceMetadata queries evidence metadata for a given repo, path, and name.
func (v *verifyEvidenceBase) queryEvidenceMetadata(repo string, path string, name string) (*[]model.SearchEvidenceEdge, error) {
	if v.oneModelClient == nil {
		manager, err := utils.CreateOnemodelServiceManager(v.serverDetails, false)
		if err != nil {
			return nil, fmt.Errorf("failed to create evidence service manager: %w", err)
		}
		v.oneModelClient = manager
	}
	query := fmt.Sprintf(searchEvidenceQuery, repo, path, name)
	queryByteArray := []byte(query)
	response, err := v.oneModelClient.GraphqlQuery(queryByteArray)
	if err != nil {
		return nil, fmt.Errorf("failed to query evidence metadata: %w", err)
	}
	evidences := model.ResponseSearchEvidence{}
	err = json.Unmarshal(response, &evidences)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal evidence metadata: %w", err)
	}
	edges := evidences.Data.Evidence.SearchEvidence.Edges
	if len(edges) == 0 {
		return nil, fmt.Errorf("no evidence found for the given subject")
	}
	return &edges, nil
}

func printText(result *model.VerificationResponse) error {
	err := validateResponse(result)
	if err != nil {
		return err
	}
	clientlog.Info(fmt.Sprintf("Verification result: %s", getColoredStatus(result.OverallVerificationStatus)))
	if result.OverallVerificationStatus != model.Success {
		err = printTable(result)
	}
	if err != nil {
		return err
	}
	return nil
}

func printFullText(result *model.VerificationResponse) error {
	err := validateResponse(result)
	if err != nil {
		return err
	}
	printChecksum(result)
	err = printTable(result)
	if err != nil {
		return err
	}
	printOverallResult(result)
	return nil
}

func printChecksum(result *model.VerificationResponse) {
	clientlog.Info(fmt.Sprintf("Subject checksum: %s", result.Checksum))
}

func printTable(result *model.VerificationResponse) error {
	evidencesVerificationResults := result.EvidencesVerificationResults
	err := coreutils.PrintTable(*evidencesVerificationResults, "", "", result.OverallVerificationStatus != model.Success)
	if err != nil {
		return err
	}
	return nil
}

func printOverallResult(result *model.VerificationResponse) {
	clientlog.Info(fmt.Sprintf("Verification result: %s", getColoredStatus(result.OverallVerificationStatus)))
}

func validateResponse(result *model.VerificationResponse) error {
	if result == nil {
		return fmt.Errorf("verification response is empty")
	}
	return nil
}

func printJson(result *model.VerificationResponse) error {
	err := validateResponse(result)
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

func getColoredStatus(status model.VerificationStatus) string {
	switch status {
	case model.Success:
		return success
	default:
		return failed
	}
}

// ExecuteAqlQuery executes an AQL query and returns the result.
func ExecuteAqlQuery(query string, client *artifactory.ArtifactoryServicesManager) (*AqlResult, error) {
	stream, err := (*client).Aql(query)
	if err != nil {
		return nil, err
	}
	defer gofrogio.Close(stream, &err)
	result, err := io.ReadAll(stream)
	if err != nil {
		return nil, err
	}
	parsedResult := new(AqlResult)
	if err = json.Unmarshal(result, parsedResult); err != nil {
		return nil, err
	}
	return parsedResult, nil
}

type AqlResult struct {
	Results []*servicesUtils.ResultItem `json:"results,omitempty"`
}
