// Package storage provides encrypted file storage functionality.
package storage

import (
	"bytes"
	"container/list"
	"encoding/binary"
	"encoding/json"
	"errors"
	"path/filepath"
	"sync"
	"time"

	bolt "go.etcd.io/bbolt"

	"gpgfs/crypto"
)

var (
	ErrNotFound      = errors.New("file not found")
	ErrIsDirectory   = errors.New("is a directory")
	ErrNotDirectory  = errors.New("not a directory")
	ErrAlreadyExists = errors.New("file already exists")
	ErrNotEmpty      = errors.New("directory not empty")
)

// Bucket names
var (
	bucketMeta    = []byte("metadata")
	bucketData    = []byte("data")
	bucketConfig  = []byte("config")
	keyNextInode  = []byte("next_inode")
	keyPrivateKey = []byte("private_key")
)

// FileType represents the type of a file entry.
type FileType int

const (
	FileTypeRegular FileType = iota
	FileTypeDirectory
<<<<<<< Updated upstream
=======
	FileTypeSocket
>>>>>>> Stashed changes
	FileTypeSymlink
)

// FileEntry represents a file or directory in the encrypted filesystem.
type FileEntry struct {
	Name    string    `json:"name"`
	Type    FileType  `json:"type"`
	Size    int64     `json:"size"`
	Mode    uint32    `json:"mode"`
	ModTime time.Time `json:"mod_time"`
	Inode   uint64    `json:"inode"`
<<<<<<< Updated upstream
	Target  string    `json:"target,omitempty"` // Symlink target path
}

// cacheEntry holds a cached decrypted file's data
type cacheEntry struct {
	path    string
	data    []byte
	modTime time.Time
	dirty   bool // true if modified but not yet persisted
}

// fileCache provides an LRU cache for decrypted file contents
type fileCache struct {
	mu       sync.Mutex
	capacity int
	items    map[string]*list.Element
	lru      *list.List // front = most recently used
}

func newFileCache(capacity int) *fileCache {
	return &fileCache{
		capacity: capacity,
		items:    make(map[string]*list.Element),
		lru:      list.New(),
	}
}

func (c *fileCache) get(path string) (*cacheEntry, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if elem, ok := c.items[path]; ok {
		c.lru.MoveToFront(elem)
		return elem.Value.(*cacheEntry), true
	}
	return nil, false
}

func (c *fileCache) put(path string, data []byte, modTime time.Time) *cacheEntry {
	c.mu.Lock()
	defer c.mu.Unlock()

	// If already exists, update it
	if elem, ok := c.items[path]; ok {
		entry := elem.Value.(*cacheEntry)
		entry.data = data
		entry.modTime = modTime
		entry.dirty = false
		c.lru.MoveToFront(elem)
		return entry
	}

	// Evict if at capacity
	for c.lru.Len() >= c.capacity {
		c.evictOldest()
	}

	entry := &cacheEntry{
		path:    path,
		data:    data,
		modTime: modTime,
		dirty:   false,
	}
	elem := c.lru.PushFront(entry)
	c.items[path] = elem
	return entry
}

func (c *fileCache) markDirty(path string, data []byte) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if elem, ok := c.items[path]; ok {
		entry := elem.Value.(*cacheEntry)
		entry.data = data
		entry.modTime = time.Now()
		entry.dirty = true
		c.lru.MoveToFront(elem)
		return
	}

	// Not in cache, add it as dirty
	for c.lru.Len() >= c.capacity {
		c.evictOldest()
	}

	entry := &cacheEntry{
		path:    path,
		data:    data,
		modTime: time.Now(),
		dirty:   true,
	}
	elem := c.lru.PushFront(entry)
	c.items[path] = elem
}

func (c *fileCache) invalidate(path string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if elem, ok := c.items[path]; ok {
		c.lru.Remove(elem)
		delete(c.items, path)
	}
}

func (c *fileCache) rename(oldPath, newPath string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if elem, ok := c.items[oldPath]; ok {
		entry := elem.Value.(*cacheEntry)
		entry.path = newPath
		delete(c.items, oldPath)
		c.items[newPath] = elem
	}
}

// evictOldest removes the least recently used non-dirty entry
// caller must hold the lock
func (c *fileCache) evictOldest() {
	// Try to evict oldest non-dirty entry first
	for elem := c.lru.Back(); elem != nil; elem = elem.Prev() {
		entry := elem.Value.(*cacheEntry)
		if !entry.dirty {
			c.lru.Remove(elem)
			delete(c.items, entry.path)
			return
		}
	}
	// All entries are dirty, evict oldest anyway (data loss possible but prevents memory exhaustion)
	if elem := c.lru.Back(); elem != nil {
		entry := elem.Value.(*cacheEntry)
		c.lru.Remove(elem)
		delete(c.items, entry.path)
	}
}

// fileLock provides per-file locking
type fileLock struct {
	mu       sync.Mutex
	locks    map[string]*sync.RWMutex
	refCount map[string]int
}

func newFileLock() *fileLock {
	return &fileLock{
		locks:    make(map[string]*sync.RWMutex),
		refCount: make(map[string]int),
	}
}

func (fl *fileLock) getLock(path string) *sync.RWMutex {
	fl.mu.Lock()
	defer fl.mu.Unlock()

	if lock, ok := fl.locks[path]; ok {
		fl.refCount[path]++
		return lock
	}

	lock := &sync.RWMutex{}
	fl.locks[path] = lock
	fl.refCount[path] = 1
	return lock
}

func (fl *fileLock) releaseLock(path string) {
	fl.mu.Lock()
	defer fl.mu.Unlock()

	fl.refCount[path]--
	if fl.refCount[path] <= 0 {
		delete(fl.locks, path)
		delete(fl.refCount, path)
	}
=======
	Target  string    `json:"target,omitempty"` // For symlinks
>>>>>>> Stashed changes
}

// Storage manages encrypted file storage in a single file.
type Storage struct {
	db        *bolt.DB
	gpg       *crypto.GPGHandler
	fileLocks *fileLock
	cache     *fileCache
	dbLock    sync.RWMutex // protects BoltDB transactions only, not individual file operations
}

// NewStorage opens or creates an encrypted storage container file.
func NewStorage(containerPath string, passphrase string) (*Storage, error) {
	db, err := bolt.Open(containerPath, 0600, &bolt.Options{Timeout: 1 * time.Second})
	if err != nil {
		return nil, err
	}

	s := &Storage{
		db:        db,
		fileLocks: newFileLock(),
		cache:     newFileCache(1000), // Cache up to 1000 files
	}

	// Initialize buckets and load/create GPG handler
	err = db.Update(func(tx *bolt.Tx) error {
		// Create buckets if they don't exist
		if _, err := tx.CreateBucketIfNotExists(bucketMeta); err != nil {
			return err
		}
		if _, err := tx.CreateBucketIfNotExists(bucketData); err != nil {
			return err
		}
		configBucket, err := tx.CreateBucketIfNotExists(bucketConfig)
		if err != nil {
			return err
		}

		// Load or create GPG key
		keyData := configBucket.Get(keyPrivateKey)
		if keyData != nil {
			// Decrypt and load existing key
			s.gpg, err = crypto.NewGPGHandlerFromKey(string(keyData), passphrase)
			if err != nil {
				return err
			}
		} else {
			// Create new key
			s.gpg, err = crypto.NewGPGHandler(passphrase)
			if err != nil {
				return err
			}
			// Save the key (unencrypted in the db, but db access requires the passphrase to decrypt data)
			keyArmor, err := s.gpg.ExportPrivateKey()
			if err != nil {
				return err
			}
			if err := configBucket.Put(keyPrivateKey, []byte(keyArmor)); err != nil {
				return err
			}

			// Initialize next inode counter
			if err := configBucket.Put(keyNextInode, uint64ToBytes(2)); err != nil {
				return err
			}

			// Create root directory
			metaBucket := tx.Bucket(bucketMeta)
			rootEntry := &FileEntry{
				Name:    "/",
				Type:    FileTypeDirectory,
				Mode:    0755,
				ModTime: time.Now(),
				Inode:   1,
			}
			entryData, err := json.Marshal(rootEntry)
			if err != nil {
				return err
			}
			encData, err := s.gpg.Encrypt(entryData)
			if err != nil {
				return err
			}
			if err := metaBucket.Put([]byte("/"), encData); err != nil {
				return err
			}
		}

		return nil
	})

	if err != nil {
		db.Close()
		return nil, err
	}

	return s, nil
}

// Close closes the storage container.
func (s *Storage) Close() error {
	return s.db.Close()
}

func (s *Storage) nextInode() (uint64, error) {
	var inode uint64
	err := s.db.Update(func(tx *bolt.Tx) error {
		configBucket := tx.Bucket(bucketConfig)
		inodeBytes := configBucket.Get(keyNextInode)
		inode = bytesToUint64(inodeBytes)
		return configBucket.Put(keyNextInode, uint64ToBytes(inode+1))
	})
	return inode, err
}

// GetEntry retrieves a file entry by path.
func (s *Storage) GetEntry(path string) (*FileEntry, error) {
	lock := s.fileLocks.getLock(path)
	lock.RLock()
	defer func() {
		lock.RUnlock()
		s.fileLocks.releaseLock(path)
	}()

	var entry *FileEntry
	err := s.db.View(func(tx *bolt.Tx) error {
		metaBucket := tx.Bucket(bucketMeta)
		encData := metaBucket.Get([]byte(path))
		if encData == nil {
			return ErrNotFound
		}

		data, err := s.gpg.Decrypt(encData)
		if err != nil {
			return err
		}

		entry = &FileEntry{}
		return json.Unmarshal(data, entry)
	})

	if err != nil {
		return nil, err
	}
	return entry, nil
}

// ListDirectory returns all entries in a directory.
func (s *Storage) ListDirectory(path string) ([]*FileEntry, error) {
	lock := s.fileLocks.getLock(path)
	lock.RLock()
	defer func() {
		lock.RUnlock()
		s.fileLocks.releaseLock(path)
	}()

	// First verify it's a directory
	var entries []*FileEntry
	err := s.db.View(func(tx *bolt.Tx) error {
		metaBucket := tx.Bucket(bucketMeta)

		// Check parent is a directory
		encData := metaBucket.Get([]byte(path))
		if encData == nil {
			return ErrNotFound
		}
		data, err := s.gpg.Decrypt(encData)
		if err != nil {
			return err
		}
		var parentEntry FileEntry
		if err := json.Unmarshal(data, &parentEntry); err != nil {
			return err
		}
		if parentEntry.Type != FileTypeDirectory {
			return ErrNotDirectory
		}

		// Find all direct children
		c := metaBucket.Cursor()
		for k, v := c.First(); k != nil; k, v = c.Next() {
			keyPath := string(k)
			if keyPath == path {
				continue
			}
			// Check if this is a direct child
			if filepath.Dir(keyPath) == path || (path == "/" && filepath.Dir(keyPath) == "/") {
				decData, err := s.gpg.Decrypt(v)
				if err != nil {
					return err
				}
				var entry FileEntry
				if err := json.Unmarshal(decData, &entry); err != nil {
					return err
				}
				entries = append(entries, &entry)
			}
		}
		return nil
	})

	if err != nil {
		return nil, err
	}
	return entries, nil
}

// CreateFile creates a new file entry.
func (s *Storage) CreateFile(path string, mode uint32) (*FileEntry, error) {
	lock := s.fileLocks.getLock(path)
	lock.Lock()
	defer func() {
		lock.Unlock()
		s.fileLocks.releaseLock(path)
	}()

	inode, err := s.nextInode()
	if err != nil {
		return nil, err
	}

	entry := &FileEntry{
		Name:    filepath.Base(path),
		Type:    FileTypeRegular,
		Size:    0,
		Mode:    mode,
		ModTime: time.Now(),
		Inode:   inode,
	}

	err = s.db.Update(func(tx *bolt.Tx) error {
		metaBucket := tx.Bucket(bucketMeta)

		// Check if already exists
		if metaBucket.Get([]byte(path)) != nil {
			return ErrAlreadyExists
		}

		// Check parent exists and is a directory
		parent := filepath.Dir(path)
		parentData := metaBucket.Get([]byte(parent))
		if parentData == nil {
			return ErrNotFound
		}
		decParent, err := s.gpg.Decrypt(parentData)
		if err != nil {
			return err
		}
		var parentEntry FileEntry
		if err := json.Unmarshal(decParent, &parentEntry); err != nil {
			return err
		}
		if parentEntry.Type != FileTypeDirectory {
			return ErrNotDirectory
		}

		// Save the new entry
		entryData, err := json.Marshal(entry)
		if err != nil {
			return err
		}
		encData, err := s.gpg.Encrypt(entryData)
		if err != nil {
			return err
		}
		return metaBucket.Put([]byte(path), encData)
	})

	if err != nil {
		return nil, err
	}
	return entry, nil
}

// CreateDirectory creates a new directory entry.
func (s *Storage) CreateDirectory(path string, mode uint32) (*FileEntry, error) {
	lock := s.fileLocks.getLock(path)
	lock.Lock()
	defer func() {
		lock.Unlock()
		s.fileLocks.releaseLock(path)
	}()

	inode, err := s.nextInode()
	if err != nil {
		return nil, err
	}

	entry := &FileEntry{
		Name:    filepath.Base(path),
		Type:    FileTypeDirectory,
		Mode:    mode,
		ModTime: time.Now(),
		Inode:   inode,
	}

	err = s.db.Update(func(tx *bolt.Tx) error {
		metaBucket := tx.Bucket(bucketMeta)

		// Check if already exists
		if metaBucket.Get([]byte(path)) != nil {
			return ErrAlreadyExists
		}

		// Check parent exists and is a directory
		parent := filepath.Dir(path)
		parentData := metaBucket.Get([]byte(parent))
		if parentData == nil {
			return ErrNotFound
		}
		decParent, err := s.gpg.Decrypt(parentData)
		if err != nil {
			return err
		}
		var parentEntry FileEntry
		if err := json.Unmarshal(decParent, &parentEntry); err != nil {
			return err
		}
		if parentEntry.Type != FileTypeDirectory {
			return ErrNotDirectory
		}

		// Save the new entry
		entryData, err := json.Marshal(entry)
		if err != nil {
			return err
		}
		encData, err := s.gpg.Encrypt(entryData)
		if err != nil {
			return err
		}
		return metaBucket.Put([]byte(path), encData)
	})

	if err != nil {
		return nil, err
	}
	return entry, nil
}

<<<<<<< Updated upstream
// CreateSymlink creates a new symlink entry.
func (s *Storage) CreateSymlink(path string, target string) (*FileEntry, error) {
	lock := s.fileLocks.getLock(path)
	lock.Lock()
	defer func() {
		lock.Unlock()
		s.fileLocks.releaseLock(path)
	}()
=======
// CreateSocket creates a new socket entry.
func (s *Storage) CreateSocket(path string, mode uint32) (*FileEntry, error) {
	s.metaLock.Lock()
	defer s.metaLock.Unlock()

	inode, err := s.nextInode()
	if err != nil {
		return nil, err
	}

	entry := &FileEntry{
		Name:    filepath.Base(path),
		Type:    FileTypeSocket,
		Size:    0,
		Mode:    mode,
		ModTime: time.Now(),
		Inode:   inode,
	}

	err = s.db.Update(func(tx *bolt.Tx) error {
		metaBucket := tx.Bucket(bucketMeta)

		// Check if already exists
		if metaBucket.Get([]byte(path)) != nil {
			return ErrAlreadyExists
		}

		// Check parent exists and is a directory
		parent := filepath.Dir(path)
		parentData := metaBucket.Get([]byte(parent))
		if parentData == nil {
			return ErrNotFound
		}
		decParent, err := s.gpg.Decrypt(parentData)
		if err != nil {
			return err
		}
		var parentEntry FileEntry
		if err := json.Unmarshal(decParent, &parentEntry); err != nil {
			return err
		}
		if parentEntry.Type != FileTypeDirectory {
			return ErrNotDirectory
		}

		// Save the new entry
		entryData, err := json.Marshal(entry)
		if err != nil {
			return err
		}
		encData, err := s.gpg.Encrypt(entryData)
		if err != nil {
			return err
		}
		return metaBucket.Put([]byte(path), encData)
	})

	if err != nil {
		return nil, err
	}
	return entry, nil
}

// CreateSymlink creates a new symlink entry.
func (s *Storage) CreateSymlink(path string, target string) (*FileEntry, error) {
	s.metaLock.Lock()
	defer s.metaLock.Unlock()
>>>>>>> Stashed changes

	inode, err := s.nextInode()
	if err != nil {
		return nil, err
	}

	entry := &FileEntry{
		Name:    filepath.Base(path),
		Type:    FileTypeSymlink,
		Size:    int64(len(target)),
<<<<<<< Updated upstream
		Mode:    0777, // Symlinks typically have 0777 mode
=======
		Mode:    0777,
>>>>>>> Stashed changes
		ModTime: time.Now(),
		Inode:   inode,
		Target:  target,
	}

	err = s.db.Update(func(tx *bolt.Tx) error {
		metaBucket := tx.Bucket(bucketMeta)

		// Check if already exists
		if metaBucket.Get([]byte(path)) != nil {
			return ErrAlreadyExists
		}

		// Check parent exists and is a directory
		parent := filepath.Dir(path)
		parentData := metaBucket.Get([]byte(parent))
		if parentData == nil {
			return ErrNotFound
		}
		decParent, err := s.gpg.Decrypt(parentData)
		if err != nil {
			return err
		}
		var parentEntry FileEntry
		if err := json.Unmarshal(decParent, &parentEntry); err != nil {
			return err
		}
		if parentEntry.Type != FileTypeDirectory {
			return ErrNotDirectory
		}

		// Save the new entry
		entryData, err := json.Marshal(entry)
		if err != nil {
			return err
		}
		encData, err := s.gpg.Encrypt(entryData)
		if err != nil {
			return err
		}
		return metaBucket.Put([]byte(path), encData)
	})

	if err != nil {
		return nil, err
	}
	return entry, nil
}

// WriteFile writes encrypted data to a file.
func (s *Storage) WriteFile(path string, data []byte) error {
	lock := s.fileLocks.getLock(path)
	lock.Lock()
	defer func() {
		lock.Unlock()
		s.fileLocks.releaseLock(path)
	}()

	err := s.db.Update(func(tx *bolt.Tx) error {
		metaBucket := tx.Bucket(bucketMeta)
		dataBucket := tx.Bucket(bucketData)

		// Get and verify entry
		encMeta := metaBucket.Get([]byte(path))
		if encMeta == nil {
			return ErrNotFound
		}
		decMeta, err := s.gpg.Decrypt(encMeta)
		if err != nil {
			return err
		}
		var entry FileEntry
		if err := json.Unmarshal(decMeta, &entry); err != nil {
			return err
		}
		if entry.Type != FileTypeRegular {
			return ErrIsDirectory
		}

		// Encrypt and store data
		encData, err := s.gpg.Encrypt(data)
		if err != nil {
			return err
		}
		if err := dataBucket.Put(uint64ToBytes(entry.Inode), encData); err != nil {
			return err
		}

		// Update metadata
		entry.Size = int64(len(data))
		entry.ModTime = time.Now()
		entryData, err := json.Marshal(&entry)
		if err != nil {
			return err
		}
		encMeta, err = s.gpg.Encrypt(entryData)
		if err != nil {
			return err
		}
		return metaBucket.Put([]byte(path), encMeta)
	})

	if err == nil {
		// Update cache with the written data
		s.cache.put(path, data, time.Now())
	}

	return err
}

// ReadFile reads and decrypts a file.
func (s *Storage) ReadFile(path string) ([]byte, error) {
	// Check cache first
	if cached, ok := s.cache.get(path); ok {
		// Return a copy to prevent modification of cached data
		result := make([]byte, len(cached.data))
		copy(result, cached.data)
		return result, nil
	}

	lock := s.fileLocks.getLock(path)
	lock.RLock()
	defer func() {
		lock.RUnlock()
		s.fileLocks.releaseLock(path)
	}()

	// Double-check cache after acquiring lock
	if cached, ok := s.cache.get(path); ok {
		result := make([]byte, len(cached.data))
		copy(result, cached.data)
		return result, nil
	}

	var data []byte
	var modTime time.Time
	err := s.db.View(func(tx *bolt.Tx) error {
		metaBucket := tx.Bucket(bucketMeta)
		dataBucket := tx.Bucket(bucketData)

		// Get entry
		encMeta := metaBucket.Get([]byte(path))
		if encMeta == nil {
			return ErrNotFound
		}
		decMeta, err := s.gpg.Decrypt(encMeta)
		if err != nil {
			return err
		}
		var entry FileEntry
		if err := json.Unmarshal(decMeta, &entry); err != nil {
			return err
		}
		if entry.Type != FileTypeRegular {
			return ErrIsDirectory
		}
		modTime = entry.ModTime

		// Get data
		encData := dataBucket.Get(uint64ToBytes(entry.Inode))
		if encData == nil {
			data = []byte{} // Empty file
			return nil
		}

		data, err = s.gpg.Decrypt(encData)
		return err
	})

	if err != nil {
		return nil, err
	}

	// Cache the result
	s.cache.put(path, data, modTime)

	// Return a copy
	result := make([]byte, len(data))
	copy(result, data)
	return result, nil
}

<<<<<<< Updated upstream
// DeleteFile removes a file or symlink.
=======
// DeleteFile removes a file, socket, or symlink.
>>>>>>> Stashed changes
func (s *Storage) DeleteFile(path string) error {
	lock := s.fileLocks.getLock(path)
	lock.Lock()
	defer func() {
		lock.Unlock()
		s.fileLocks.releaseLock(path)
	}()

	err := s.db.Update(func(tx *bolt.Tx) error {
		metaBucket := tx.Bucket(bucketMeta)
		dataBucket := tx.Bucket(bucketData)

		// Get entry
		encMeta := metaBucket.Get([]byte(path))
		if encMeta == nil {
			return ErrNotFound
		}
		decMeta, err := s.gpg.Decrypt(encMeta)
		if err != nil {
			return err
		}
		var entry FileEntry
		if err := json.Unmarshal(decMeta, &entry); err != nil {
			return err
		}
		if entry.Type == FileTypeDirectory {
			return ErrIsDirectory
		}

<<<<<<< Updated upstream
		// Delete data (only for regular files, symlinks have no data)
=======
		// Delete data (only for regular files)
>>>>>>> Stashed changes
		if entry.Type == FileTypeRegular {
			dataBucket.Delete(uint64ToBytes(entry.Inode))
		}

		// Delete metadata
		return metaBucket.Delete([]byte(path))
	})

	if err == nil {
		// Invalidate cache
		s.cache.invalidate(path)
	}

	return err
}

// DeleteDirectory removes an empty directory.
func (s *Storage) DeleteDirectory(path string) error {
	if path == "/" {
		return errors.New("cannot delete root directory")
	}

	lock := s.fileLocks.getLock(path)
	lock.Lock()
	defer func() {
		lock.Unlock()
		s.fileLocks.releaseLock(path)
	}()

	return s.db.Update(func(tx *bolt.Tx) error {
		metaBucket := tx.Bucket(bucketMeta)

		// Get entry
		encMeta := metaBucket.Get([]byte(path))
		if encMeta == nil {
			return ErrNotFound
		}
		decMeta, err := s.gpg.Decrypt(encMeta)
		if err != nil {
			return err
		}
		var entry FileEntry
		if err := json.Unmarshal(decMeta, &entry); err != nil {
			return err
		}
		if entry.Type != FileTypeDirectory {
			return ErrNotDirectory
		}

		// Check if empty
		c := metaBucket.Cursor()
		for k, _ := c.First(); k != nil; k, _ = c.Next() {
			keyPath := string(k)
			if keyPath != path && filepath.Dir(keyPath) == path {
				return ErrNotEmpty
			}
		}

		// Delete metadata
		return metaBucket.Delete([]byte(path))
	})
}

// Rename moves/renames a file or directory.
func (s *Storage) Rename(oldPath, newPath string) error {
	// For rename, we need to lock both paths
	oldLock := s.fileLocks.getLock(oldPath)
	newLock := s.fileLocks.getLock(newPath)

	// Lock in consistent order to avoid deadlock
	if oldPath < newPath {
		oldLock.Lock()
		newLock.Lock()
	} else {
		newLock.Lock()
		oldLock.Lock()
	}
	defer func() {
		oldLock.Unlock()
		newLock.Unlock()
		s.fileLocks.releaseLock(oldPath)
		s.fileLocks.releaseLock(newPath)
	}()

	var isDir bool
	var childrenToRename []struct{ old, new string }

	err := s.db.Update(func(tx *bolt.Tx) error {
		metaBucket := tx.Bucket(bucketMeta)

		// Get old entry
		encMeta := metaBucket.Get([]byte(oldPath))
		if encMeta == nil {
			return ErrNotFound
		}
		decMeta, err := s.gpg.Decrypt(encMeta)
		if err != nil {
			return err
		}
		var entry FileEntry
		if err := json.Unmarshal(decMeta, &entry); err != nil {
			return err
		}
		isDir = entry.Type == FileTypeDirectory

		// Check new parent exists
		newParent := filepath.Dir(newPath)
		parentData := metaBucket.Get([]byte(newParent))
		if parentData == nil {
			return ErrNotFound
		}
		decParent, err := s.gpg.Decrypt(parentData)
		if err != nil {
			return err
		}
		var parentEntry FileEntry
		if err := json.Unmarshal(decParent, &parentEntry); err != nil {
			return err
		}
		if parentEntry.Type != FileTypeDirectory {
			return ErrNotDirectory
		}

		// Update entry name
		entry.Name = filepath.Base(newPath)
		entry.ModTime = time.Now()

		// Save to new path
		entryData, err := json.Marshal(&entry)
		if err != nil {
			return err
		}
		newEncMeta, err := s.gpg.Encrypt(entryData)
		if err != nil {
			return err
		}
		if err := metaBucket.Put([]byte(newPath), newEncMeta); err != nil {
			return err
		}

		// Delete old path
		if err := metaBucket.Delete([]byte(oldPath)); err != nil {
			return err
		}

		// If directory, move all children
		if entry.Type == FileTypeDirectory {
			var toMove []struct{ old, new string }
			c := metaBucket.Cursor()
			for k, _ := c.First(); k != nil; k, _ = c.Next() {
				keyPath := string(k)
				if len(keyPath) > len(oldPath) && bytes.HasPrefix(k, []byte(oldPath+"/")) {
					newChildPath := newPath + keyPath[len(oldPath):]
					toMove = append(toMove, struct{ old, new string }{keyPath, newChildPath})
				}
			}

			for _, m := range toMove {
				childEnc := metaBucket.Get([]byte(m.old))
				if childEnc != nil {
					if err := metaBucket.Put([]byte(m.new), childEnc); err != nil {
						return err
					}
					if err := metaBucket.Delete([]byte(m.old)); err != nil {
						return err
					}
				}
			}
			childrenToRename = toMove
		}

		return nil
	})

	if err == nil {
		// Update cache for renamed file
		s.cache.rename(oldPath, newPath)
		// Update cache for children if directory
		if isDir {
			for _, m := range childrenToRename {
				s.cache.rename(m.old, m.new)
			}
		}
	}

	return err
}

// UpdateEntry updates a file entry's metadata.
func (s *Storage) UpdateEntry(path string, entry *FileEntry) error {
	lock := s.fileLocks.getLock(path)
	lock.Lock()
	defer func() {
		lock.Unlock()
		s.fileLocks.releaseLock(path)
	}()

	return s.db.Update(func(tx *bolt.Tx) error {
		metaBucket := tx.Bucket(bucketMeta)

		if metaBucket.Get([]byte(path)) == nil {
			return ErrNotFound
		}

		entryData, err := json.Marshal(entry)
		if err != nil {
			return err
		}
		encData, err := s.gpg.Encrypt(entryData)
		if err != nil {
			return err
		}
		return metaBucket.Put([]byte(path), encData)
	})
}

func uint64ToBytes(v uint64) []byte {
	b := make([]byte, 8)
	binary.BigEndian.PutUint64(b, v)
	return b
}

func bytesToUint64(b []byte) uint64 {
	if len(b) < 8 {
		return 0
	}
	return binary.BigEndian.Uint64(b)
}
