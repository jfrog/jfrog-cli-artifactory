// Package permissions holds helpers for restricting the permissions of files
// that `jf setup` writes with embedded credentials (a token in a URL, an
// _authToken line, a <password> element, ...).
package permissions

import (
	"os"

	"github.com/jfrog/jfrog-cli-core/v2/utils/coreutils"
	"github.com/jfrog/jfrog-client-go/utils/errorutils"
	"github.com/jfrog/jfrog-client-go/utils/log"
)

// chmodOwnerOnly restricts path to owner-only access (0600).
//
// It is a no-op on Windows: os.Chmod there only toggles FILE_ATTRIBUTE_READONLY
// and cannot express an owner-only DACL, so the file keeps the ACLs it inherited
// from its directory. Those are per-user on a default profile but not on a
// redirected or shared one, so a nil return on Windows means "not tightened",
// not "protected".
func chmodOwnerOnly(path string) error {
	if coreutils.IsWindows() {
		log.Debug("Not restricting permissions of", path,
			"- chmod cannot set Windows ACLs, so the file keeps the ones inherited from its directory")
		return nil
	}
	return errorutils.CheckError(os.Chmod(path, 0600))
}

// WriteFileOwnerOnly writes data to path with owner-only permissions (0600). Use
// it for files this process creates itself with embedded credentials (pip.conf,
// uv.toml). It also re-applies the mode to a pre-existing file, because
// os.WriteFile only sets the mode when it creates the file - a config left at
// 0644 by an earlier run would otherwise stay world-readable. The error is
// returned to the caller: this is our own write, so a failure is not best-effort.
func WriteFileOwnerOnly(path string, data []byte) error {
	if err := os.WriteFile(path, data, 0600); err != nil {
		return errorutils.CheckError(err)
	}
	return chmodOwnerOnly(path)
}

// RestrictExisting best-effort restricts an existing credential file that another
// module wrote (go env -w, the Maven/Gradle/pnpm/Yarn tooling, pip config set) to
// owner-only. It never fails the caller: the package manager is already
// configured, so a permission we could not tighten - or a path that another tool
// resolved differently than we predicted - is warned about, not fatal. Tightening
// stops here; the setup command's real work is done.
func RestrictExisting(path string) {
	if _, err := os.Stat(path); err != nil {
		log.Warn("Could not locate " + path + " to restrict its permissions. " +
			"If it holds credentials, restrict it to owner-only access manually.")
		return
	}
	if err := chmodOwnerOnly(path); err != nil {
		log.Warn("Could not restrict permissions of " + path + ". " +
			"If it holds credentials, restrict it to owner-only access manually.")
	}
}
