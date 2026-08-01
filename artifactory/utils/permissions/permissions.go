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

// ChmodOwnerOnly restricts path to owner-only access (0600), for configuration
// files that hold credentials.
//
// It is a no-op on Windows: os.Chmod there only toggles FILE_ATTRIBUTE_READONLY
// and cannot express an owner-only DACL, so the file keeps the ACLs it inherited
// from its directory. Those are per-user on a default profile but not on a
// redirected or shared one, so a nil return on Windows means "not tightened",
// not "protected".
func ChmodOwnerOnly(path string) error {
	if coreutils.IsWindows() {
		log.Debug("Not restricting permissions of", path,
			"- chmod cannot set Windows ACLs, so the file keeps the ones inherited from its directory")
		return nil
	}
	return errorutils.CheckError(os.Chmod(path, 0600))
}
