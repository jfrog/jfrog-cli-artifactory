package ca

//go:generate ${PROJECT_DIR}/scripts/mockgen.sh ${GOFILE}

import (
	"github.com/jfrog/jfrog-cli-artifactory/artifactory/docs/common"
	"github.com/pkg/errors"
	"github.com/sigstore/sigstore-go/pkg/root"
	"github.com/sigstore/sigstore-go/pkg/tuf"
	"path/filepath"
)

type TUFRootCertificateProvider interface {
	LoadTUFRootCertificate() (root.TrustedMaterial, error)
}

type tufRootCertificateProvider struct {
}

func NewTUFRootCertificateProvider() TUFRootCertificateProvider {
	return &tufRootCertificateProvider{}
}

func (t *tufRootCertificateProvider) LoadTUFRootCertificate() (root.TrustedMaterial, error) {
	opts := tuf.DefaultOptions().WithCachePath(filepath.Join(common.JfrogCliHomeDir, "evidence/security/certs"))
	trustedRoot, err := root.FetchTrustedRootWithOptions(opts)
	if err == nil {
		return trustedRoot, nil
	}

	return nil, errors.Wrapf(err, "failed to fetch trusted root")
}
