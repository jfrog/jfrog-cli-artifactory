package common

import "io/fs"

const (
	// PrivateFileMode is rw------- for user-owned manifests and attestation artifacts.
	PrivateFileMode fs.FileMode = 0o600
	// DefaultDirMode is rwxr-xr-x for directories created during publish.
	DefaultDirMode fs.FileMode = 0o755
	// InstallDirMode is rwxr-x--- for directories created during agent install, unzip, and copy.
	InstallDirMode fs.FileMode = 0o750
	// InstallManifestFileMode is rw-r----- for .jfrog/*-info.json install manifests.
	InstallManifestFileMode fs.FileMode = 0o640
	// DefaultFileMode is rw-r--r-- for regular file entries in publish zips on Unix.
	DefaultFileMode fs.FileMode = 0o644
)
