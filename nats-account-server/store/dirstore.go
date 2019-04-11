package store

import (
	"fmt"
	"github.com/nats-io/account-server/nats-account-server/conf"
	"io/ioutil"
	"os"
	"path/filepath"
)

const (
	extension = "jwt"
)

// DirJWTStore implements the JWT Store interface, keeping JWTs in a sharded
// directory structure
type DirJWTStore struct {
	directory string
	readonly  bool
}

// NewDirJWTStore returns an empty, mutable directory-based JWT store
func NewDirJWTStore(dirPath string, create bool) (JWTStore, error) {
	dirPath, err := conf.ValidateDirPath(dirPath)

	if err != nil {
		if !create {
			return nil, err
		}

		err := os.MkdirAll(dirPath, 0755)

		if err != nil {
			return nil, err
		}
	}

	return &DirJWTStore{
		directory: dirPath,
		readonly:  false,
	}, nil
}

// NewImmutableDirJWTStore returns an immutable store with the provided directory
func NewImmutableDirJWTStore(dirPath string) (JWTStore, error) {
	dirPath, err := conf.ValidateDirPath(dirPath)

	if err != nil {
		return nil, err
	}

	return &DirJWTStore{
		directory: dirPath,
		readonly:  true,
	}, nil
}

// Load checks the memory store and returns the matching JWT or an error
func (store *DirJWTStore) Load(publicKey string) (string, error) {
	path := store.pathForKey(publicKey)

	if path == "" {
		return "", fmt.Errorf("invalid public key")
	}

	data, err := ioutil.ReadFile(path)

	if err != nil {
		return "", err
	}

	return string(data), nil
}

// Save puts the JWT in a map by public key, no checks are performed
func (store *DirJWTStore) Save(publicKey string, theJWT string) error {
	if store.readonly {
		return fmt.Errorf("store is read-only")
	}

	path := store.pathForKey(publicKey)

	if path == "" {
		return fmt.Errorf("invalid public key")
	}

	dirPath := filepath.Dir(path)
	_, err := conf.ValidateDirPath(dirPath)
	if err != nil {
		err := os.MkdirAll(dirPath, 0755)
		if err != nil {
			return err
		}
	}

	return ioutil.WriteFile(path, []byte(theJWT), 0644)
}

// IsReadOnly returns a flag determined at creation time
func (store *DirJWTStore) IsReadOnly() bool {
	return store.readonly
}

func (store *DirJWTStore) pathForKey(publicKey string) string {
	if len(publicKey) < 2 {
		return ""
	}

	last := publicKey[len(publicKey)-2:]
	fileName := fmt.Sprintf("%s.%s", publicKey, extension)

	dirPath := filepath.Join(store.directory, last, fileName)

	return dirPath
}
