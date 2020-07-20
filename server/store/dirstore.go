/*
 * Copyright 2019 The NATS Authors
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 *
 */

package store

import (
	"container/heap"
	"container/list"
	"crypto/sha256"
	"fmt"
	"io/ioutil"
	"math"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/nats-io/jwt/v2" // only used to decode, not for storage
	"github.com/nats-io/nats-account-server/server/conf"
)

const (
	extension = "jwt"
)

// JWTChanged functions are called when the store file watcher notices a JWT changed
type JWTChanged func(publicKey string)

// JWTError functions are called when the store file watcher has an error
type JWTError func(err error)

// DirJWTStore implements the JWT Store interface, keeping JWTs in an optionally sharded
// directory structure
type DirJWTStore struct {
	sync.Mutex
	hash          [sha256.Size]byte // contains sha256 xor of stored jwt
	directory     string
	readonly      bool
	shard         bool
	expiration    *ExpirationTracker
	changed       JWTChanged
	errorOccurred JWTError
	watcher       *fsnotify.Watcher
	done          chan struct{}
}

func newDir(dirPath string, create bool) (string, error) {
	fullPath, err := conf.ValidateDirPath(dirPath)
	if err != nil {
		if !create {
			return "", err
		}
		if err = os.MkdirAll(dirPath, 0755); err != nil {
			return "", err
		}
		if fullPath, err = conf.ValidateDirPath(dirPath); err != nil {
			return "", err
		}
	}
	return fullPath, nil
}

// NewDirJWTStore returns an empty, mutable directory-based JWT store
func NewDirJWTStore(dirPath string, shard bool, create bool, changeNotification JWTChanged, errorNotification JWTError) (JWTStore, error) {
	fullPath, err := newDir(dirPath, create)
	if err != nil {
		return nil, err
	}

	theStore := &DirJWTStore{
		directory: fullPath,
		shard:     shard,
	}

	if changeNotification != nil && errorNotification != nil {
		theStore.changed = changeNotification
		theStore.errorOccurred = errorNotification
		theStore.readonly = true
		if err := theStore.startWatching(); err != nil {
			theStore.Close()
			return nil, err
		}
	} else {
		hash := [sha256.Size]byte{}
		if err := filepath.Walk(dirPath, func(path string, info os.FileInfo, err error) error {
			if strings.HasSuffix(path, extension) {
				if jwt, err := ioutil.ReadFile(path); err == nil {
					xorAssign(&hash, sha256.Sum256(jwt))
				}
			}
			return nil
		}); err != nil {
			theStore.Close()
			return nil, err
		}
		theStore.hash = hash
		theStore.readonly = false
	}
	return theStore, err
}

func NewExpiringDirJWTStore(dirPath string, shard bool, create bool, expireCheck time.Duration, limit int) (JWTStore, error) {
	fullPath, err := newDir(dirPath, create)
	if err != nil {
		return nil, err
	}

	theStore := &DirJWTStore{
		directory: fullPath,
		shard:     shard}

	if expireCheck <= time.Duration(0) {
		expireCheck = time.Minute
	}
	if limit == 0 {
		limit = math.MaxInt64
	}
	theStore.startExpiring(expireCheck, limit)
	hash := [sha256.Size]byte{}
	theStore.Lock()
	err = filepath.Walk(dirPath, func(path string, info os.FileInfo, err error) error {
		if strings.HasSuffix(path, extension) {
			if jwt, err := ioutil.ReadFile(path); err == nil {
				xorAssign(&hash, sha256.Sum256(jwt))
				_, file := filepath.Split(path)
				theStore.expiration.Track(strings.TrimSuffix(file, "."+extension), string(jwt))
			}
		}
		return nil
	})
	theStore.Unlock()
	if err != nil {
		theStore.Close()
		return nil, err
	}
	theStore.hash = hash
	theStore.readonly = false
	return theStore, err
}

// NewImmutableDirJWTStore returns an immutable store with the provided directory
func NewImmutableDirJWTStore(dirPath string, sharded bool, changeNotification JWTChanged, errorNotification JWTError) (JWTStore, error) {
	dirPath, err := conf.ValidateDirPath(dirPath)

	if err != nil {
		return nil, err
	}

	theStore := &DirJWTStore{
		directory:     dirPath,
		readonly:      true,
		shard:         sharded,
		changed:       changeNotification,
		errorOccurred: errorNotification,
	}

	if changeNotification != nil && errorNotification != nil {
		err = theStore.startWatching()

		if err != nil {
			theStore.Close()
			return nil, err
		}
	}

	return theStore, err
}

func (store *DirJWTStore) startWatching() error {
	store.Lock()
	defer store.Unlock()

	watcher, err := fsnotify.NewWatcher()

	if err != nil {
		return err
	}

	store.watcher = watcher

	// Watch the top level dir (could be sharded)
	dirPath := store.directory
	watcher.Add(dirPath)

	var files []string
	err = filepath.Walk(dirPath, func(path string, info os.FileInfo, err error) error {
		if info.IsDir() && store.shard && filepath.Dir(path) == store.directory {
			files = append(files, path)
		}

		if !info.IsDir() && strings.HasSuffix(path, extension) {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		return err
	}

	for _, file := range files {
		watcher.Add(file)
	}

	done := make(chan struct{})
	store.done = done

	go func() {
	WATCHLOOP:
		for {
			select {
			case event, ok := <-watcher.Events:
				if !ok {
					break WATCHLOOP
				}

				if event.Op&fsnotify.Write == fsnotify.Write {
					// Check for jwt change, ignore others
					if strings.HasSuffix(event.Name, extension) {
						fileName := filepath.Base(event.Name)
						pubKey := strings.Replace(fileName, ".jwt", "", -1)
						store.changed(pubKey)
					}
				} else if event.Op&fsnotify.Create == fsnotify.Create {
					if strings.HasSuffix(event.Name, extension) {
						err := watcher.Add(event.Name)
						if err != nil {
							store.errorOccurred(err)
						}
					} else if filepath.Dir(event.Name) == store.directory && store.shard { // Only go 1 level down
						err := watcher.Add(event.Name)
						if err != nil {
							store.errorOccurred(err)
						}
						var files []string
						err = filepath.Walk(event.Name, func(path string, info os.FileInfo, err error) error {
							if !info.IsDir() && strings.HasSuffix(path, extension) {
								files = append(files, path)
							}
							return nil
						})

						if err != nil {
							store.errorOccurred(err)
							break
						}

						for _, file := range files {
							watcher.Add(file)
						}
					}
				}
			case err, ok := <-watcher.Errors:
				if !ok {
					break WATCHLOOP
				}
				store.errorOccurred(err)
			}
		}
		close(done)
	}()

	return nil
}

// Load checks the memory store and returns the matching JWT or an error
func (store *DirJWTStore) load(publicKey string) (string, error) {
	store.Lock()
	defer store.Unlock()

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
func (store *DirJWTStore) save(publicKey string, theJWT string) error {
	store.Lock()
	defer store.Unlock()

	if store.readonly {
		return fmt.Errorf("store is read-only")
	}

	path := store.pathForKey(publicKey)
	if path == "" {
		return fmt.Errorf("invalid public key")
	}

	dirPath := filepath.Dir(path)
	if _, err := conf.ValidateDirPath(dirPath); err != nil {
		if err := os.MkdirAll(dirPath, 0755); err != nil {
			return err
		}
	}

	return store.write(path, publicKey, theJWT)
}

func (store *DirJWTStore) LoadAcc(publicKey string) (string, error) {
	return store.load(publicKey)
}

func (store *DirJWTStore) SaveAcc(publicKey string, theJWT string) error {
	return store.save(publicKey, theJWT)
}

func (store *DirJWTStore) LoadAct(hash string) (string, error) {
	return store.load(hash)
}

func (store *DirJWTStore) SaveAct(hash string, theJWT string) error {
	return store.save(hash, theJWT)
}

// IsReadOnly returns a flag determined at creation time
func (store *DirJWTStore) IsReadOnly() bool {
	return store.readonly
}

func (store *DirJWTStore) pathForKey(publicKey string) string {
	if len(publicKey) < 2 {
		return ""
	}

	var dirPath string

	if store.shard {
		last := publicKey[len(publicKey)-2:]
		fileName := fmt.Sprintf("%s.%s", publicKey, extension)
		dirPath = filepath.Join(store.directory, last, fileName)
	} else {
		fileName := fmt.Sprintf("%s.%s", publicKey, extension)
		dirPath = filepath.Join(store.directory, fileName)
	}

	return dirPath
}

// Close is a no-op for a directory store
func (store *DirJWTStore) Close() {
	store.Lock()
	defer store.Unlock()
	if store.watcher != nil {
		store.watcher.Close()
	} else {
		store.expiration.Close()
	}
	if store.done != nil {
		<-store.done
	}
	store.expiration = nil
	store.watcher = nil
	store.done = nil
}

// Pack up to maxJWTs into a package
func (store *DirJWTStore) Pack(maxJWTs int) (string, error) {
	count := 0
	var pack []string

	if maxJWTs > 0 {
		pack = make([]string, 0, maxJWTs)
	} else {
		pack = []string{}
	}

	store.Lock()

	dirPath := store.directory

	err := filepath.Walk(dirPath, func(path string, info os.FileInfo, err error) error {
		if !info.IsDir() && strings.HasSuffix(path, extension) { // this is a JWT
			if count == maxJWTs { // won't match negative
				return nil
			}

			pubKey := filepath.Base(path)
			pubKey = pubKey[0:strings.Index(pubKey, ".")]

			jwtBytes, err := ioutil.ReadFile(path)
			if err != nil {
				return err
			}

			if store.expiration != nil {
				if claim, err := jwt.DecodeGeneric(string(jwtBytes)); err == nil && claim.Expires > 0 && claim.Expires < time.Now().Unix() {
					return nil
				}
			}

			pack = append(pack, fmt.Sprintf("%s|%s", pubKey, string(jwtBytes)))
			count++
		}
		return nil
	})

	store.Unlock()

	if err != nil {
		return "", err
	}

	return strings.Join(pack, "\n"), nil
}

// Merge takes the JWTs from package and adds them to the store
// Merge is destructive in the sense that it doesn't check if the JWT
// is newer or anything like that.
func (store *DirJWTStore) Merge(pack string) error {
	newJWTs := strings.Split(pack, "\n")

	store.Lock()
	defer store.Unlock()

	for _, line := range newJWTs {
		if line == "" { // ignore blank lines
			continue
		}

		split := strings.Split(line, "|")
		if len(split) != 2 {
			return fmt.Errorf("line in package didn't contain 2 entries: %q", line)
		}

		if err := store.saveIfNewer(split[0], split[1]); err != nil {
			return err
		}
	}

	return nil
}

// Assumes the lock is held, and only updates if the jwt is new, or the one on disk is older
func (store *DirJWTStore) saveIfNewer(publicKey string, theJWT string) error {
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

	if _, err := os.Stat(path); err == nil {
		newJWT, err := jwt.DecodeGeneric(theJWT)
		if err != nil {
			return err
		}

		existing, err := ioutil.ReadFile(path)
		if err != nil {
			return err
		}

		existingJWT, err := jwt.DecodeGeneric(string(existing))
		if err != nil {
			return err
		}

		if existingJWT.IssuedAt > newJWT.IssuedAt {
			return nil
		}
	}

	return store.write(path, publicKey, theJWT)
}

func xorAssign(lVal *[sha256.Size]byte, rVal [sha256.Size]byte) {
	for i := range rVal {
		(*lVal)[i] ^= rVal[i]
	}
}

// write that keeps hash of all jwt in sync
// Assumes the lock is held
func (store *DirJWTStore) write(path string, publicKey string, theJWT string) error {
	oldHash := [sha256.Size]byte{}
	if _, err := os.Stat(path); err == nil {
		if existing, err := ioutil.ReadFile(path); err != nil {
			return err
		} else if store.expiration != nil {
			oldHash = sha256.Sum256(existing)
			// this write is an update, move to back
			if e, ok := store.expiration.idx[publicKey]; ok {
				store.expiration.lru.MoveToBack(e)
			}
		}
	} else if store.expiration != nil && store.expiration.lru.Len() >= store.expiration.limit {
		// this write is an add, pick the least recently used value for removal
		i := store.expiration.lru.Front().Value.(*JWTItem)
		delPath := store.pathForKey(i.publicKey)
		if existing, err := ioutil.ReadFile(delPath); err != nil {
			return err
		} else if err := os.Remove(delPath); err != nil {
			return err
		} else {
			delHash := sha256.Sum256(existing)
			xorAssign(&store.hash, delHash)
			heap.Remove(store.expiration, i.index)
		}
	}

	if err := ioutil.WriteFile(path, []byte(theJWT), 0644); err != nil {
		return err
	} else {
		xorAssign(&store.hash, oldHash)                       // remove old hash or no op
		xorAssign(&store.hash, sha256.Sum256([]byte(theJWT))) // add new one
		if store.expiration != nil {
			store.expiration.Track(publicKey, theJWT)
		}
		return nil
	}
}

func (store *DirJWTStore) Hash() [sha256.Size]byte {
	store.Lock()
	defer store.Unlock()
	return store.hash
}

// An JWTItem is something managed by the priority queue
type JWTItem struct {
	index      int
	publicKey  string
	expiration time.Time
}

// A ExpirationTracker implements heap.Interface and holds Items.
type ExpirationTracker struct {
	heap  []*JWTItem
	idx   map[string]*list.Element
	lru   *list.List
	limit int
	quit  chan<- struct{}
}

func (pq *ExpirationTracker) Len() int { return len(pq.heap) }

func (q *ExpirationTracker) Less(i, j int) bool {
	pq := q.heap
	return pq[i].expiration.Before(pq[j].expiration)
}

func (q *ExpirationTracker) Swap(i, j int) {
	pq := q.heap
	pq[i], pq[j] = pq[j], pq[i]
	pq[i].index = i
	pq[j].index = j
}

func (q *ExpirationTracker) Push(x interface{}) {
	n := len(q.heap)
	item := x.(*JWTItem)
	item.index = n
	q.heap = append(q.heap, item)
	q.idx[item.publicKey] = q.lru.PushBack(item)
}

func (q *ExpirationTracker) Pop() interface{} {
	old := q.heap
	n := len(old)
	item := old[n-1]
	old[n-1] = nil // avoid memory leak
	item.index = -1
	q.heap = old[0 : n-1]
	q.lru.Remove(q.idx[item.publicKey])
	delete(q.idx, item.publicKey)
	return item
}

func (pq *ExpirationTracker) UnTrack(publicKey string) {
	if it, ok := pq.idx[publicKey]; ok {
		delete(pq.idx, publicKey)
		heap.Remove(pq, it.Value.(*JWTItem).index)
	}
}

func (pq *ExpirationTracker) Track(publicKey string, theJWT string) {
	if g, err := jwt.DecodeGeneric(theJWT); err == nil && g != nil {
		var exp time.Time
		if g.Expires == 0 {
			// in order to never expire, assume maximum time possible.
			exp = time.Unix(math.MaxInt64-62135596801, 999999999)
		} else {
			exp = time.Unix(g.Expires, 0)
		}
		if e, ok := pq.idx[publicKey]; ok {
			i := e.Value.(*JWTItem)
			i.expiration = exp
			heap.Fix(pq, i.index)
		} else {
			heap.Push(pq, &JWTItem{-1, publicKey, exp})
		}
	}
}

func (pq *ExpirationTracker) PopItem() *JWTItem {
	return heap.Pop(pq).(*JWTItem)
}

func (pq *ExpirationTracker) Close() {
	if pq == nil || pq.quit == nil {
		return
	}
	close(pq.quit)
	pq.quit = nil
}

func (store *DirJWTStore) startExpiring(reCheck time.Duration, limit int) {
	store.Lock()
	defer store.Unlock()
	done := make(chan struct{})
	store.done = done

	quit := make(chan struct{})
	pq := &ExpirationTracker{
		make([]*JWTItem, 0, 10),
		make(map[string]*list.Element),
		list.New(),
		limit,
		quit,
	}
	store.expiration = pq

	go func() {
		t := time.NewTicker(reCheck)
		defer t.Stop()
		defer close(done)
		for {
			now := time.Now()
			store.Lock()
			if pq.Len() > 0 {
				if it := pq.PopItem(); it.expiration.Before(now) {
					path := store.pathForKey(it.publicKey)
					if existing, err := ioutil.ReadFile(path); err != nil {
						heap.Push(pq, it) // retry later
					} else if err := os.Remove(path); err != nil {
						heap.Push(pq, it) // retry later
					} else {
						oldHash := sha256.Sum256(existing)
						xorAssign(&store.hash, oldHash)
						pq.UnTrack(it.publicKey)
						store.Unlock()
						continue // we removed an entry, check next one
					}
				} else {
					heap.Push(pq, it)
				}
			}
			store.Unlock()
			select {
			case <-t.C:
			case <-quit:
				return
			}
		}
	}()
}
