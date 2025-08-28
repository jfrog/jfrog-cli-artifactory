package verify

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/jfrog/jfrog-cli-artifactory/evidence/model"
	"github.com/jfrog/jfrog-cli-artifactory/evidence/verify/reports"
	"github.com/jfrog/jfrog-cli-artifactory/evidence/verify/verifiers"
	"github.com/jfrog/jfrog-cli-core/v2/artifactory/utils"
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/jfrog/jfrog-cli-core/v2/utils/coreutils"

	"github.com/jfrog/jfrog-client-go/artifactory"
	"github.com/jfrog/jfrog-client-go/onemodel"
	"github.com/jfrog/jfrog-client-go/utils/log"
)

const searchEvidenceQueryWithPublicKey = `{"query":"{ evidence { searchEvidence( where: { hasSubjectWith: { repositoryKey: \"%s\", path: \"%s\", name: \"%s\" }} ) { edges { cursor node { downloadPath predicateType createdAt createdBy subject { sha256 } signingKey {alias, publicKey} } } } } }"}`
const searchEvidenceQueryWithoutPublicKey = `{"query":"{ evidence { searchEvidence( where: { hasSubjectWith: { repositoryKey: \"%s\", path: \"%s\", name: \"%s\" }} ) { edges { cursor node { downloadPath predicateType createdAt createdBy subject { sha256 } } } } } }"}`

// verifyEvidenceBase provides shared logic for evidence verification commands.
type verifyEvidenceBase struct {
	serverDetails      *config.ServerDetails
	format             string
	keys               []string
	useArtifactoryKeys bool
	artifactoryClient  *artifactory.ArtifactoryServicesManager
	oneModelClient     onemodel.Manager
	verifier           verifiers.EvidenceVerifierInterface
}

// printVerifyResult prints the verification result in the requested format.
func (v *verifyEvidenceBase) printVerifyResult(result *model.VerificationResponse) error {
	switch v.format {
	case "markdown":
		return reports.MarkdownReportPrinter.Print(result)
	case "json":
		return reports.JsonReportPrinter.Print(result)
	default:
		return reports.PlaintextReportPrinter.Print(result)
	}
}

// verifyEvidence runs the verification process for the given evidence metadata and subject sha256.
func (v *verifyEvidenceBase) verifyEvidence(client *artifactory.ArtifactoryServicesManager, evidenceMetadata *[]model.SearchEvidenceEdge, sha256, subjectPath string) error {
	if v.verifier == nil {
		v.verifier = verifiers.NewEvidenceVerifier(v.keys, v.useArtifactoryKeys, client)
	}
	verify, err := v.verifier.Verify(sha256, evidenceMetadata, subjectPath)
	if err != nil {
		return err
	}

	err = v.printVerifyResult(verify)
	if verify.OverallVerificationStatus == model.Failed {
		return coreutils.CliError{ExitCode: coreutils.ExitCodeError}
	}
	return err
}

// createArtifactoryClient creates an Artifactory client for evidence operations.
func (v *verifyEvidenceBase) createArtifactoryClient() (*artifactory.ArtifactoryServicesManager, error) {
	if v.artifactoryClient != nil {
		return v.artifactoryClient, nil
	}
	artifactoryClient, err := utils.CreateUploadServiceManager(v.serverDetails, 1, 0, 0, false, nil)
	if err != nil {
		return nil, err
	}
	v.artifactoryClient = &artifactoryClient
	return v.artifactoryClient, nil
}

// queryEvidenceMetadata queries evidence metadata for a given repo, path, and name.
func (v *verifyEvidenceBase) queryEvidenceMetadata(repo string, path string, name string) (*[]model.SearchEvidenceEdge, error) {
	err := createOneModelService(v)
	if err != nil {
		return nil, err
	}
	var query string
	if v.useArtifactoryKeys {
		query = fmt.Sprintf(searchEvidenceQueryWithPublicKey, repo, path, name)
	} else {
		query = fmt.Sprintf(searchEvidenceQueryWithoutPublicKey, repo, path, name)
	}
	log.Debug("Fetch evidence metadata using query:", query)
	queryByteArray := []byte(query)
	response, err := v.oneModelClient.GraphqlQuery(queryByteArray)
	if err != nil {
		errStr := err.Error()
		if isPublicKeyFieldNotFound(errStr) {
			return nil, fmt.Errorf("the evidence service version should be at least 7.125.0 and the onemodel version should be at least 1.55.0")
		}
		return nil, fmt.Errorf("error querying evidence from One-Model service: %w", err)
	}
	evidence := model.ResponseSearchEvidence{}
	err = json.Unmarshal(response, &evidence)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal evidence metadata: %w", err)
	}
	edges := evidence.Data.Evidence.SearchEvidence.Edges
	if len(edges) == 0 {
		return nil, fmt.Errorf("no evidence found for the given subject")
	}
	return &edges, nil
}

func createOneModelService(v *verifyEvidenceBase) error {
	if v.oneModelClient != nil {
		return nil
	}
	manager, err := utils.CreateOnemodelServiceManager(v.serverDetails, false)
	if err != nil {
		return err
	}
	v.oneModelClient = manager
	return nil
}

func isPublicKeyFieldNotFound(errStr string) bool {
	return strings.Contains(errStr, "publicKey")
}
