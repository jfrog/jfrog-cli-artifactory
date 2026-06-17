package healcomponents

import (
	"os"
	"path/filepath"

	"github.com/jfrog/jfrog-client-go/utils/errorutils"
)

type lockfileBackup struct {
	path    string
	content []byte // nil means the file did not exist before apply
}

// ApplyLockfiles backs up existing lockfiles under projectRoot, writes updates, returns restore func.
// Paths listed in treatAsAbsent are restored by deletion even if they exist on disk (bootstrapped locks).
func ApplyLockfiles(projectRoot string, lockfiles []Lockfile, treatAsAbsent []string) (restore func() error, err error) {
	if len(lockfiles) == 0 {
		return func() error { return nil }, nil
	}

	absent := make(map[string]bool, len(treatAsAbsent))
	for _, path := range treatAsAbsent {
		absent[path] = true
	}

	var backups []lockfileBackup
	for _, lf := range lockfiles {
		fullPath := filepath.Join(projectRoot, lf.Path)
		backup := lockfileBackup{path: fullPath}
		if !absent[lf.Path] {
			data, readErr := os.ReadFile(fullPath)
			if readErr == nil {
				backup.content = data
			} else if !os.IsNotExist(readErr) {
				return nil, errorutils.CheckError(readErr)
			}
		}
		backups = append(backups, backup)

		if err = os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
			return nil, errorutils.CheckError(err)
		}
		if err = os.WriteFile(fullPath, lf.Content, 0644); err != nil {
			return nil, errorutils.CheckError(err)
		}
	}

	return func() error {
		for _, backup := range backups {
			if backup.content == nil {
				if err = os.Remove(backup.path); err != nil && !os.IsNotExist(err) {
					return errorutils.CheckError(err)
				}
				continue
			}
			if err = os.WriteFile(backup.path, backup.content, 0644); err != nil {
				return errorutils.CheckError(err)
			}
		}
		return nil
	}, nil
}
