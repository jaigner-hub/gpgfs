// Package fs provides the FUSE filesystem implementation.
package fs

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/hanwen/go-fuse/v2/fs"
	"github.com/hanwen/go-fuse/v2/fuse"

	"gpgfs/storage"
)

// GPGFS implements the FUSE filesystem interface.
type GPGFS struct {
	fs.Inode
	storage *storage.Storage
}

// GPGFSNode represents a node in the filesystem.
type GPGFSNode struct {
	fs.Inode
	storage *storage.Storage
	path    string
	mu      sync.RWMutex
}

// NewGPGFS creates a new GPG-encrypted FUSE filesystem.
func NewGPGFS(store *storage.Storage) *GPGFS {
	return &GPGFS{
		storage: store,
	}
}

// Root returns the root node of the filesystem.
func (g *GPGFS) Root() *GPGFSNode {
	return &GPGFSNode{
		storage: g.storage,
		path:    "/",
	}
}

var _ = (fs.NodeGetattrer)((*GPGFSNode)(nil))
var _ = (fs.NodeSetattrer)((*GPGFSNode)(nil))
var _ = (fs.NodeLookuper)((*GPGFSNode)(nil))
var _ = (fs.NodeReaddirer)((*GPGFSNode)(nil))
var _ = (fs.NodeMkdirer)((*GPGFSNode)(nil))
var _ = (fs.NodeMknoder)((*GPGFSNode)(nil))
var _ = (fs.NodeCreater)((*GPGFSNode)(nil))
var _ = (fs.NodeUnlinker)((*GPGFSNode)(nil))
var _ = (fs.NodeRmdirer)((*GPGFSNode)(nil))
var _ = (fs.NodeRenamer)((*GPGFSNode)(nil))
var _ = (fs.NodeOpener)((*GPGFSNode)(nil))
var _ = (fs.NodeSymlinker)((*GPGFSNode)(nil))
var _ = (fs.NodeReadlinker)((*GPGFSNode)(nil))
<<<<<<< Updated upstream
var _ = (fs.NodeStatfser)((*GPGFSNode)(nil))
var _ = (fs.NodeAccesser)((*GPGFSNode)(nil))

// Default cache timeout for attributes and entries
const attrCacheTimeout = 1 * time.Second
const entryCacheTimeout = 1 * time.Second
=======
>>>>>>> Stashed changes

// Getattr returns file attributes.
func (n *GPGFSNode) Getattr(ctx context.Context, fh fs.FileHandle, out *fuse.AttrOut) syscall.Errno {
	n.mu.RLock()
	defer n.mu.RUnlock()

	out.SetTimeout(attrCacheTimeout)
	return n.getAttrLocked(out)
}

func (n *GPGFSNode) getAttrLocked(out *fuse.AttrOut) syscall.Errno {
	entry, err := n.storage.GetEntry(n.path)
	if err != nil {
		return syscall.ENOENT
	}

	out.Ino = entry.Inode
	out.Size = uint64(entry.Size)
	out.Mtime = uint64(entry.ModTime.Unix())
	out.Ctime = uint64(entry.ModTime.Unix())
	out.Atime = uint64(entry.ModTime.Unix())
	out.Uid = uint32(os.Getuid())
	out.Gid = uint32(os.Getgid())

	switch entry.Type {
	case storage.FileTypeDirectory:
		out.Mode = uint32(entry.Mode) | syscall.S_IFDIR
		out.Nlink = 2
<<<<<<< Updated upstream
=======
	case storage.FileTypeSocket:
		out.Mode = uint32(entry.Mode) | syscall.S_IFSOCK
		out.Nlink = 1
>>>>>>> Stashed changes
	case storage.FileTypeSymlink:
		out.Mode = uint32(entry.Mode) | syscall.S_IFLNK
		out.Nlink = 1
	default:
		out.Mode = uint32(entry.Mode) | syscall.S_IFREG
		out.Nlink = 1
	}

	return 0
}

// Setattr sets file attributes.
func (n *GPGFSNode) Setattr(ctx context.Context, fh fs.FileHandle, in *fuse.SetAttrIn, out *fuse.AttrOut) syscall.Errno {
	n.mu.Lock()
	defer n.mu.Unlock()

	entry, err := n.storage.GetEntry(n.path)
	if err != nil {
		return syscall.ENOENT
	}

	if m, ok := in.GetMode(); ok {
		entry.Mode = m
	}

	// Handle truncation
	if sz, ok := in.GetSize(); ok {
		if entry.Type == storage.FileTypeRegular {
			// Read existing data
			existing, _ := n.storage.ReadFile(n.path)

			var newData []byte
			if sz == 0 {
				newData = []byte{}
			} else if int64(len(existing)) > int64(sz) {
				newData = existing[:sz]
			} else if int64(len(existing)) < int64(sz) {
				newData = make([]byte, sz)
				copy(newData, existing)
			} else {
				newData = existing
			}

			if err := n.storage.WriteFile(n.path, newData); err != nil {
				return syscall.EIO
			}
			entry.Size = int64(sz)
		}
	}

	// Handle mtime
	if mtime, ok := in.GetMTime(); ok {
		entry.ModTime = mtime
	}

	if err := n.storage.UpdateEntry(n.path, entry); err != nil {
		return syscall.EIO
	}

	return n.getAttrLocked(out)
}

// Lookup finds a child node.
func (n *GPGFSNode) Lookup(ctx context.Context, name string, out *fuse.EntryOut) (*fs.Inode, syscall.Errno) {
	childPath := filepath.Join(n.path, name)

	entry, err := n.storage.GetEntry(childPath)
	if err != nil {
		return nil, syscall.ENOENT
	}

	child := &GPGFSNode{
		storage: n.storage,
		path:    childPath,
	}

	var mode uint32
	switch entry.Type {
	case storage.FileTypeDirectory:
		mode = syscall.S_IFDIR | uint32(entry.Mode)
<<<<<<< Updated upstream
=======
	case storage.FileTypeSocket:
		mode = syscall.S_IFSOCK | uint32(entry.Mode)
>>>>>>> Stashed changes
	case storage.FileTypeSymlink:
		mode = syscall.S_IFLNK | uint32(entry.Mode)
	default:
		mode = syscall.S_IFREG | uint32(entry.Mode)
	}

	stable := fs.StableAttr{
		Mode: mode,
		Ino:  entry.Inode,
	}

	out.Ino = entry.Inode
	out.Size = uint64(entry.Size)
	out.Mtime = uint64(entry.ModTime.Unix())
	out.Mode = mode
	out.Uid = uint32(os.Getuid())
	out.Gid = uint32(os.Getgid())
	out.SetEntryTimeout(entryCacheTimeout)
	out.SetAttrTimeout(attrCacheTimeout)

	return n.NewInode(ctx, child, stable), 0
}

// Readdir lists directory contents.
func (n *GPGFSNode) Readdir(ctx context.Context) (fs.DirStream, syscall.Errno) {
	entries, err := n.storage.ListDirectory(n.path)
	if err != nil {
		return nil, syscall.ENOENT
	}

	var dirEntries []fuse.DirEntry
	for _, e := range entries {
		var mode uint32
		switch e.Type {
		case storage.FileTypeDirectory:
			mode = syscall.S_IFDIR
<<<<<<< Updated upstream
=======
		case storage.FileTypeSocket:
			mode = syscall.S_IFSOCK
>>>>>>> Stashed changes
		case storage.FileTypeSymlink:
			mode = syscall.S_IFLNK
		default:
			mode = syscall.S_IFREG
		}
		dirEntries = append(dirEntries, fuse.DirEntry{
			Name: e.Name,
			Ino:  e.Inode,
			Mode: mode,
		})
	}

	return fs.NewListDirStream(dirEntries), 0
}

// Mkdir creates a directory.
func (n *GPGFSNode) Mkdir(ctx context.Context, name string, mode uint32, out *fuse.EntryOut) (*fs.Inode, syscall.Errno) {
	childPath := filepath.Join(n.path, name)

	entry, err := n.storage.CreateDirectory(childPath, mode)
	if err != nil {
		if err == storage.ErrAlreadyExists {
			return nil, syscall.EEXIST
		}
		return nil, syscall.EIO
	}

	child := &GPGFSNode{
		storage: n.storage,
		path:    childPath,
	}

	stable := fs.StableAttr{
		Mode: syscall.S_IFDIR | mode,
		Ino:  entry.Inode,
	}

	out.Ino = entry.Inode
	out.Mode = syscall.S_IFDIR | mode
	out.Uid = uint32(os.Getuid())
	out.Gid = uint32(os.Getgid())
	out.SetEntryTimeout(entryCacheTimeout)
	out.SetAttrTimeout(attrCacheTimeout)

	return n.NewInode(ctx, child, stable), 0
}

// Mknod creates a special file (socket, fifo, device).
func (n *GPGFSNode) Mknod(ctx context.Context, name string, mode uint32, dev uint32, out *fuse.EntryOut) (*fs.Inode, syscall.Errno) {
	childPath := filepath.Join(n.path, name)

	// Extract the file type from mode
	fileType := mode & syscall.S_IFMT
	permissions := mode &^ syscall.S_IFMT

	var entry *storage.FileEntry
	var err error

	switch fileType {
	case syscall.S_IFSOCK:
		entry, err = n.storage.CreateSocket(childPath, permissions)
	default:
		// We only support sockets for now
		return nil, syscall.ENOTSUP
	}

	if err != nil {
		if err == storage.ErrAlreadyExists {
			return nil, syscall.EEXIST
		}
		return nil, syscall.EIO
	}

	child := &GPGFSNode{
		storage: n.storage,
		path:    childPath,
	}

	stable := fs.StableAttr{
		Mode: mode,
		Ino:  entry.Inode,
	}

	out.Ino = entry.Inode
	out.Mode = mode
	out.Uid = uint32(os.Getuid())
	out.Gid = uint32(os.Getgid())

	return n.NewInode(ctx, child, stable), 0
}

// Symlink creates a symbolic link.
func (n *GPGFSNode) Symlink(ctx context.Context, target, name string, out *fuse.EntryOut) (*fs.Inode, syscall.Errno) {
	childPath := filepath.Join(n.path, name)

	entry, err := n.storage.CreateSymlink(childPath, target)
	if err != nil {
		if err == storage.ErrAlreadyExists {
			return nil, syscall.EEXIST
		}
		return nil, syscall.EIO
	}

	child := &GPGFSNode{
		storage: n.storage,
		path:    childPath,
	}

	stable := fs.StableAttr{
		Mode: syscall.S_IFLNK | 0777,
		Ino:  entry.Inode,
	}

	out.Ino = entry.Inode
	out.Size = uint64(len(target))
	out.Mode = syscall.S_IFLNK | 0777
	out.Uid = uint32(os.Getuid())
	out.Gid = uint32(os.Getgid())

	return n.NewInode(ctx, child, stable), 0
}

// Readlink reads a symbolic link's target.
func (n *GPGFSNode) Readlink(ctx context.Context) ([]byte, syscall.Errno) {
	entry, err := n.storage.GetEntry(n.path)
	if err != nil {
		return nil, syscall.ENOENT
	}

	if entry.Type != storage.FileTypeSymlink {
		return nil, syscall.EINVAL
	}

	return []byte(entry.Target), 0
}

// Create creates a new file.
func (n *GPGFSNode) Create(ctx context.Context, name string, flags uint32, mode uint32, out *fuse.EntryOut) (inode *fs.Inode, fh fs.FileHandle, fuseFlags uint32, errno syscall.Errno) {
	childPath := filepath.Join(n.path, name)

	entry, err := n.storage.CreateFile(childPath, mode)
	if err != nil {
		if err == storage.ErrAlreadyExists {
			return nil, nil, 0, syscall.EEXIST
		}
		return nil, nil, 0, syscall.EIO
	}

	child := &GPGFSNode{
		storage: n.storage,
		path:    childPath,
	}

	stable := fs.StableAttr{
		Mode: syscall.S_IFREG | mode,
		Ino:  entry.Inode,
	}

	out.Ino = entry.Inode
	out.Mode = syscall.S_IFREG | mode
	out.Uid = uint32(os.Getuid())
	out.Gid = uint32(os.Getgid())
	out.SetEntryTimeout(entryCacheTimeout)
	out.SetAttrTimeout(attrCacheTimeout)

	handle := &GPGFileHandle{
		node: child,
	}

	return n.NewInode(ctx, child, stable), handle, 0, 0
}

// Unlink removes a file.
func (n *GPGFSNode) Unlink(ctx context.Context, name string) syscall.Errno {
	childPath := filepath.Join(n.path, name)

	if err := n.storage.DeleteFile(childPath); err != nil {
		if err == storage.ErrNotFound {
			return syscall.ENOENT
		}
		if err == storage.ErrIsDirectory {
			return syscall.EISDIR
		}
		return syscall.EIO
	}

	return 0
}

// Rmdir removes a directory.
func (n *GPGFSNode) Rmdir(ctx context.Context, name string) syscall.Errno {
	childPath := filepath.Join(n.path, name)

	if err := n.storage.DeleteDirectory(childPath); err != nil {
		if err == storage.ErrNotFound {
			return syscall.ENOENT
		}
		if err == storage.ErrNotDirectory {
			return syscall.ENOTDIR
		}
		if err == storage.ErrNotEmpty {
			return syscall.ENOTEMPTY
		}
		return syscall.EIO
	}

	return 0
}

// Symlink creates a symbolic link.
func (n *GPGFSNode) Symlink(ctx context.Context, target, name string, out *fuse.EntryOut) (*fs.Inode, syscall.Errno) {
	childPath := filepath.Join(n.path, name)

	entry, err := n.storage.CreateSymlink(childPath, target)
	if err != nil {
		if err == storage.ErrAlreadyExists {
			return nil, syscall.EEXIST
		}
		return nil, syscall.EIO
	}

	child := &GPGFSNode{
		storage: n.storage,
		path:    childPath,
	}

	stable := fs.StableAttr{
		Mode: syscall.S_IFLNK | 0777,
		Ino:  entry.Inode,
	}

	out.Ino = entry.Inode
	out.Size = uint64(entry.Size)
	out.Mode = syscall.S_IFLNK | 0777
	out.Uid = uint32(os.Getuid())
	out.Gid = uint32(os.Getgid())
	out.SetEntryTimeout(entryCacheTimeout)
	out.SetAttrTimeout(attrCacheTimeout)

	return n.NewInode(ctx, child, stable), 0
}

// Readlink reads the target of a symbolic link.
func (n *GPGFSNode) Readlink(ctx context.Context) ([]byte, syscall.Errno) {
	n.mu.RLock()
	defer n.mu.RUnlock()

	entry, err := n.storage.GetEntry(n.path)
	if err != nil {
		return nil, syscall.ENOENT
	}

	if entry.Type != storage.FileTypeSymlink {
		return nil, syscall.EINVAL
	}

	return []byte(entry.Target), 0
}

// Statfs returns filesystem statistics.
func (n *GPGFSNode) Statfs(ctx context.Context, out *fuse.StatfsOut) syscall.Errno {
	// Report generous filesystem capacity so apps don't think it's full
	// Using 1TB virtual capacity with 500GB free
	const blockSize = 4096
	const totalBlocks = (1024 * 1024 * 1024 * 1024) / blockSize // 1TB
	const freeBlocks = (500 * 1024 * 1024 * 1024) / blockSize   // 500GB free

	out.Blocks = totalBlocks
	out.Bfree = freeBlocks
	out.Bavail = freeBlocks
	out.Files = 1000000    // Max inodes
	out.Ffree = 999000     // Free inodes
	out.Bsize = blockSize  // Block size
	out.NameLen = 255      // Max filename length
	out.Frsize = blockSize // Fragment size

	return 0
}

// Access checks file permissions.
func (n *GPGFSNode) Access(ctx context.Context, mask uint32) syscall.Errno {
	// Check if the entry exists
	entry, err := n.storage.GetEntry(n.path)
	if err != nil {
		return syscall.ENOENT
	}

	// For simplicity, if the entry exists and belongs to the current user,
	// allow all requested access modes (since we own everything in our encrypted fs)
	_ = entry
	return 0
}

// Rename moves/renames a file or directory.
func (n *GPGFSNode) Rename(ctx context.Context, name string, newParent fs.InodeEmbedder, newName string, flags uint32) syscall.Errno {
	oldPath := filepath.Join(n.path, name)

	newParentNode, ok := newParent.(*GPGFSNode)
	if !ok {
		newParentNode = &GPGFSNode{
			storage: n.storage,
			path:    "/",
		}
	}
	newPath := filepath.Join(newParentNode.path, newName)

	if err := n.storage.Rename(oldPath, newPath); err != nil {
		if err == storage.ErrNotFound {
			return syscall.ENOENT
		}
		return syscall.EIO
	}

	// Update the inode tree to reflect the rename
	child := n.GetChild(name)
	if child != nil {
		// Update paths recursively for the child and all its descendants
		updatePathsRecursive(child, oldPath, newPath)
		// Move the child in the inode tree
		newParentInode := newParentNode.EmbeddedInode()
		n.MvChild(name, newParentInode, newName, true)
	}

	return 0
}

// updatePathsRecursive updates the path field for a node and all its children
func updatePathsRecursive(inode *fs.Inode, oldPrefix, newPrefix string) {
	if node, ok := inode.Operations().(*GPGFSNode); ok {
		node.mu.Lock()
		// Replace the old prefix with the new prefix in this node's path
		if node.path == oldPrefix {
			node.path = newPrefix
		} else if len(node.path) > len(oldPrefix) && node.path[:len(oldPrefix)+1] == oldPrefix+"/" {
			node.path = newPrefix + node.path[len(oldPrefix):]
		}
		node.mu.Unlock()
	}

	// Recursively update all children
	for _, childInode := range inode.Children() {
		updatePathsRecursive(childInode, oldPrefix, newPrefix)
	}
}

// Open opens a file.
func (n *GPGFSNode) Open(ctx context.Context, flags uint32) (fs.FileHandle, uint32, syscall.Errno) {
	return &GPGFileHandle{node: n}, 0, 0
}

// GPGFileHandle implements file operations with write buffering.
type GPGFileHandle struct {
	node   *GPGFSNode
	mu     sync.Mutex
	buffer []byte // in-memory write buffer
	dirty  bool   // true if buffer has unpersisted changes
}

var _ = (fs.FileReader)((*GPGFileHandle)(nil))
var _ = (fs.FileWriter)((*GPGFileHandle)(nil))
var _ = (fs.FileFlusher)((*GPGFileHandle)(nil))
var _ = (fs.FileSetattrer)((*GPGFileHandle)(nil))
var _ = (fs.FileGetattrer)((*GPGFileHandle)(nil))
var _ = (fs.FileFsyncer)((*GPGFileHandle)(nil))
var _ = (fs.FileReleaser)((*GPGFileHandle)(nil))

// Getattr returns file attributes via the file handle.
func (fh *GPGFileHandle) Getattr(ctx context.Context, out *fuse.AttrOut) syscall.Errno {
	fh.mu.Lock()
	defer fh.mu.Unlock()

	entry, err := fh.node.storage.GetEntry(fh.node.path)
	if err != nil {
		return syscall.ENOENT
	}

	out.Ino = entry.Inode
	// Return buffer size if we have pending writes
	if fh.buffer != nil {
		out.Size = uint64(len(fh.buffer))
	} else {
		out.Size = uint64(entry.Size)
	}
	out.Mtime = uint64(entry.ModTime.Unix())
	out.Ctime = uint64(entry.ModTime.Unix())
	out.Atime = uint64(entry.ModTime.Unix())
	out.Uid = uint32(os.Getuid())
	out.Gid = uint32(os.Getgid())

	switch entry.Type {
	case storage.FileTypeDirectory:
		out.Mode = uint32(entry.Mode) | syscall.S_IFDIR
		out.Nlink = 2
	case storage.FileTypeSymlink:
		out.Mode = uint32(entry.Mode) | syscall.S_IFLNK
		out.Nlink = 1
	default:
		out.Mode = uint32(entry.Mode) | syscall.S_IFREG
		out.Nlink = 1
	}

	return 0
}

// Setattr sets file attributes via the file handle.
func (fh *GPGFileHandle) Setattr(ctx context.Context, in *fuse.SetAttrIn, out *fuse.AttrOut) syscall.Errno {
	fh.mu.Lock()
	defer fh.mu.Unlock()

	entry, err := fh.node.storage.GetEntry(fh.node.path)
	if err != nil {
		return syscall.ENOENT
	}

	if m, ok := in.GetMode(); ok {
		entry.Mode = m
	}

	// Handle truncation
	if sz, ok := in.GetSize(); ok {
		if entry.Type == storage.FileTypeRegular {
			// Use buffer if we have one, otherwise read from storage
			var existing []byte
			if fh.buffer != nil {
				existing = fh.buffer
			} else {
				existing, _ = fh.node.storage.ReadFile(fh.node.path)
			}

			var newData []byte
			if sz == 0 {
				newData = []byte{}
			} else if int64(len(existing)) > int64(sz) {
				newData = existing[:sz]
			} else if int64(len(existing)) < int64(sz) {
				newData = make([]byte, sz)
				copy(newData, existing)
			} else {
				newData = existing
			}

			// Update buffer instead of writing directly to storage
			fh.buffer = newData
			fh.dirty = true
			entry.Size = int64(sz)
		}
	}

	// Handle mtime
	if mtime, ok := in.GetMTime(); ok {
		entry.ModTime = mtime
	} else {
		entry.ModTime = time.Now()
	}

	if err := fh.node.storage.UpdateEntry(fh.node.path, entry); err != nil {
		return syscall.EIO
	}

	// Fill in output attributes
	out.Ino = entry.Inode
	out.Size = uint64(entry.Size)
	out.Mtime = uint64(entry.ModTime.Unix())
	out.Ctime = uint64(entry.ModTime.Unix())
	out.Atime = uint64(entry.ModTime.Unix())
	out.Uid = uint32(os.Getuid())
	out.Gid = uint32(os.Getgid())

	switch entry.Type {
	case storage.FileTypeDirectory:
		out.Mode = uint32(entry.Mode) | syscall.S_IFDIR
		out.Nlink = 2
	case storage.FileTypeSymlink:
		out.Mode = uint32(entry.Mode) | syscall.S_IFLNK
		out.Nlink = 1
	default:
		out.Mode = uint32(entry.Mode) | syscall.S_IFREG
		out.Nlink = 1
	}

	return 0
}

// Read reads data from the file (uses buffer if dirty, otherwise storage).
func (fh *GPGFileHandle) Read(ctx context.Context, dest []byte, off int64) (fuse.ReadResult, syscall.Errno) {
	fh.mu.Lock()
	defer fh.mu.Unlock()

	// Use buffer if we have pending writes, otherwise read from storage
	var data []byte
	if fh.buffer != nil {
		data = fh.buffer
	} else {
		var err error
		data, err = fh.node.storage.ReadFile(fh.node.path)
		if err != nil {
			return nil, syscall.EIO
		}
	}

	if off >= int64(len(data)) {
		return fuse.ReadResultData(nil), 0
	}

	end := off + int64(len(dest))
	if end > int64(len(data)) {
		end = int64(len(data))
	}

	return fuse.ReadResultData(data[off:end]), 0
}

// Write writes data to the in-memory buffer (flushed on Flush/Release).
func (fh *GPGFileHandle) Write(ctx context.Context, data []byte, off int64) (uint32, syscall.Errno) {
	fh.mu.Lock()
	defer fh.mu.Unlock()

	// Initialize buffer from storage if this is the first write
	if fh.buffer == nil {
		existing, _ := fh.node.storage.ReadFile(fh.node.path)
		if existing == nil {
			existing = []byte{}
		}
		fh.buffer = existing
	}

	// Extend buffer if necessary
	requiredLen := off + int64(len(data))
	if int64(len(fh.buffer)) < requiredLen {
		newData := make([]byte, requiredLen)
		copy(newData, fh.buffer)
		fh.buffer = newData
	}

	// Write the new data to buffer
	copy(fh.buffer[off:], data)
	fh.dirty = true

	return uint32(len(data)), 0
}

// flushBuffer persists the in-memory buffer to storage if dirty.
func (fh *GPGFileHandle) flushBuffer() syscall.Errno {
	if !fh.dirty || fh.buffer == nil {
		return 0
	}

	if err := fh.node.storage.WriteFile(fh.node.path, fh.buffer); err != nil {
		return syscall.EIO
	}
	fh.dirty = false
	return 0
}

// Flush flushes any buffered data to storage.
func (fh *GPGFileHandle) Flush(ctx context.Context) syscall.Errno {
	fh.mu.Lock()
	defer fh.mu.Unlock()
	return fh.flushBuffer()
}

// Fsync synchronizes file contents to storage.
func (fh *GPGFileHandle) Fsync(ctx context.Context, flags uint32) syscall.Errno {
	fh.mu.Lock()
	defer fh.mu.Unlock()
	return fh.flushBuffer()
}

// Release is called when the file handle is closed. Ensures data is flushed.
func (fh *GPGFileHandle) Release(ctx context.Context) syscall.Errno {
	fh.mu.Lock()
	defer fh.mu.Unlock()
	errno := fh.flushBuffer()
	fh.buffer = nil // free memory
	return errno
}
