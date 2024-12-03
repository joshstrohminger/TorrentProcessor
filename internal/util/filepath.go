package util

import (
	"path/filepath"
	"strings"
)

// IsFileBelowDir checks if the file is contained withing dir or one of its child directories.
func IsFileBelowDir(file string, dir string) (bool, error) {
	relPath, err := filepath.Rel(dir, file)
	if err != nil {
		return false, err
	}

	// Check if the relative path starts with "..", meaning it's not within the parent directory
	return !strings.HasPrefix(relPath, ".."), nil
}
