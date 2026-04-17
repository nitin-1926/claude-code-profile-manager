package filetree

import "os"

// SymlinkToDirectory reports whether path is a symbolic link whose target is a directory.
func SymlinkToDirectory(path string) (bool, error) {
	fi, err := os.Lstat(path)
	if err != nil {
		return false, err
	}
	if fi.Mode()&os.ModeSymlink == 0 {
		return false, nil
	}
	dest, err := os.Stat(path)
	if err != nil {
		return false, err
	}
	return dest.IsDir(), nil
}
