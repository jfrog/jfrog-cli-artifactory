package verifiers

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/jfrog/jfrog-cli-artifactory/evidence/dsse"
	"github.com/jfrog/jfrog-cli-artifactory/evidence/model"
	"github.com/jfrog/jfrog-client-go/artifactory"
	"github.com/pkg/errors"
	"github.com/sigstore/sigstore-go/pkg/bundle"
)

type evidenceParserInterface interface {
	parseEvidence(evidence *model.SearchEvidenceEdge, evidenceResult *model.EvidenceVerification) error
}

type evidenceParser struct {
	artifactoryClient artifactory.ArtifactoryServicesManager
}

func newEvidenceParser(client *artifactory.ArtifactoryServicesManager) evidenceParserInterface {
	return &evidenceParser{
		artifactoryClient: *client,
	}
}

func (p *evidenceParser) parseEvidence(evidence *model.SearchEvidenceEdge, evidenceResult *model.EvidenceVerification) error {
	if evidence == nil || evidenceResult == nil {
		return fmt.Errorf("empty evidence or result provided for parsing")
	}
	file, err := p.artifactoryClient.ReadRemoteFile(evidence.Node.DownloadPath)
	if err != nil {
		return errors.Wrap(err, "failed to read remote file")
	}
	defer func(file io.ReadCloser) {
		_ = file.Close()
	}(file)

	fileContent, err := io.ReadAll(file)
	if err != nil {
		return errors.Wrap(err, "failed to read file content: "+evidence.Node.DownloadPath)
	}
	// Try Sigstore bundle first
	if err := p.tryParseSigstoreBundle(fileContent, evidenceResult); err == nil {
		return nil
	}

	// Fall back to DSSE envelope
	if err := p.tryParseDsseEnvelope(fileContent, evidenceResult); err == nil {
		return nil
	}

	return fmt.Errorf("unsupported evidence file for client-side verification: " + evidence.Node.DownloadPath)
}

func (p *evidenceParser) tryParseSigstoreBundle(content []byte, result *model.EvidenceVerification) error {
	if result == nil {
		return fmt.Errorf("empty result provided for Sigstore bundle parsing")
	}
	var sigstoreBundle bundle.Bundle
	if err := sigstoreBundle.UnmarshalJSON(content); err != nil {
		return err
	}
	result.SigstoreBundle = &sigstoreBundle
	result.MediaType = model.SigstoreBundle
	return nil
}

func (p *evidenceParser) tryParseDsseEnvelope(content []byte, result *model.EvidenceVerification) error {
	if result == nil {
		return fmt.Errorf("empty result provided for DSSE envelope parsing")
	}
	var envelope dsse.Envelope
	if err := json.Unmarshal(content, &envelope); err != nil {
		return err
	}
	result.DsseEnvelope = &envelope
	result.MediaType = model.SimpleDSSE
	return nil
}
