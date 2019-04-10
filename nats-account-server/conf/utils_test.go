package conf

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFilePath(t *testing.T) {
	file, err := ioutil.TempFile(os.TempDir(), "prefix")
	require.NoError(t, err)

	path, err := ValidateFilePath(file.Name())
	require.NoError(t, err)
	require.NotEqual(t, "", path)

	_, err = ValidateDirPath(file.Name())
	require.Error(t, err)
}

func TestDirPath(t *testing.T) {
	path, err := ioutil.TempDir(os.TempDir(), "prefix")
	require.NoError(t, err)

	abspath, err := ValidateDirPath(path)
	require.NoError(t, err)
	require.NotEqual(t, "", abspath)

	_, err = ValidateFilePath(path)
	require.Error(t, err)
}

func TestPathDoesntExist(t *testing.T) {
	path, err := ioutil.TempDir(os.TempDir(), "prefix")
	require.NoError(t, err)

	path = filepath.Join(path, "foo")

	_, err = ValidateDirPath(path)
	require.Error(t, err)
}

func TestEmptyPath(t *testing.T) {
	_, err := ValidateFilePath("")
	require.Error(t, err)

	_, err = ValidateDirPath("")
	require.Error(t, err)
}

func TestBadPath(t *testing.T) {
	_, err := ValidateFilePath("//foo\\br//#!90")
	require.Error(t, err)

	_, err = ValidateDirPath("//foo\\br//#!90")
	require.Error(t, err)
}
