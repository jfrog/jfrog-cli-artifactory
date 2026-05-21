package common

import "io/fs"

// PrivateFileMode is rw------- for user-owned manifests and attestation artifacts.
const PrivateFileMode fs.FileMode = 0o600

// DefaultDirMode is rwxr-xr-x for directories created during publish.
const DefaultDirMode fs.FileMode = 0o755

// DefaultFileMode is rw-r--r-- for regular file entries in publish zips on Unix.
const DefaultFileMode fs.FileMode = 0o644
