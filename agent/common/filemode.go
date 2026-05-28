package common

import "io/fs"

// PrivateFileMode is rw------- for user-owned manifests and attestation artifacts.
const PrivateFileMode fs.FileMode = 0o600

// DefaultDirMode is rwxr-xr-x for directories created during publish.
const DefaultDirMode fs.FileMode = 0o755

// InstallDirMode is rwxr-x--- for directories created during agent install, unzip, and copy.
const InstallDirMode fs.FileMode = 0o750

// InstallManifestFileMode is rw-r----- for .jfrog/*-info.json install manifests.
const InstallManifestFileMode fs.FileMode = 0o640

// DefaultFileMode is rw-r--r-- for regular file entries in publish zips on Unix.
const DefaultFileMode fs.FileMode = 0o644
