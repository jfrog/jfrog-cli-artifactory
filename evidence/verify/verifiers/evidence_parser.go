package verifiers

import (
	"encoding/json"
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

	return errors.New("failed to parse evidence as either Sigstore bundle or DSSE envelope")
}

func (p *evidenceParser) tryParseSigstoreBundle(content []byte, result *model.EvidenceVerification) error {
	var sigstoreBundle bundle.Bundle
	if err := sigstoreBundle.UnmarshalJSON(content); err != nil {
		return err
	}
	result.SigstoreBundle = &sigstoreBundle
	result.MediaType = model.SigstoreBundle
	return nil
}

func (p *evidenceParser) tryParseDsseEnvelope(content []byte, result *model.EvidenceVerification) error {
	var envelope dsse.Envelope
	if err := json.Unmarshal(content, &envelope); err != nil {
		return err
	}
	result.DsseEnvelope = &envelope
	result.MediaType = model.SimpleDSSE
	return nil
}
