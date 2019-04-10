package conf

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// validatePathExists checks that the provided path exists and is a dir if requested
func validatePathExists(path string, dir bool) (string, error) {
	if path == "" {
		return "", errors.New("path is not specified")
	}

	abs, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("error parsing path [%s]: %v", abs, err)
	}

	var finfo os.FileInfo
	if finfo, err = os.Stat(abs); os.IsNotExist(err) {
		return "", fmt.Errorf("the path [%s] doesn't exist", abs)
	}

	mode := finfo.Mode()
	if dir && mode.IsRegular() {
		return "", fmt.Errorf("the path [%s] is not a directory", abs)
	}

	if !dir && mode.IsDir() {
		return "", fmt.Errorf("the path [%s] is not a file", abs)
	}

	return abs, nil
}

// ValidateDirPath checks that the provided path exists and is a dir
func ValidateDirPath(path string) (string, error) {
	return validatePathExists(path, true)
}

// ValidateFilePath checks that the provided path exists and is not a dir
func ValidateFilePath(path string) (string, error) {
	return validatePathExists(path, false)
}
