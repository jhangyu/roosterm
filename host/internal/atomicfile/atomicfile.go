// Package atomicfile replaces a file in one rename so readers never see a partial file.
package atomicfile

import (
	"fmt"
	"io/fs"
	"os"
)

// Write writes data to path+".tmp" with perm, chmods it to perm (umask-proof), and renames it over path.
// ponytail: no fsync (same as the code it replaced); add file+dir fsync if crash-durability of the password hash matters.
func Write(path string, data []byte, perm fs.FileMode) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, perm); err != nil {
		return fmt.Errorf("write %s: %w", tmp, err)
	}
	if err := os.Chmod(tmp, perm); err != nil {
		return fmt.Errorf("chmod %s: %w", tmp, err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("rename %s to %s: %w", tmp, path, err)
	}
	return nil
}
