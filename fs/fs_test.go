package fs

import (
	"bytes"
	"crypto/rand"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/hanwen/go-fuse/v2/fs"
	"github.com/hanwen/go-fuse/v2/fuse"

	"gpgfs/storage"
)

// testMount holds references for a mounted test filesystem
type testMount struct {
	mountPoint    string
	containerPath string
	storage       *storage.Storage
	server        *fuse.Server
}

// skipIfNoFUSE skips the test if FUSE is not available
func skipIfNoFUSE(t *testing.T) {
	t.Helper()

	// Check if /dev/fuse exists
	if _, err := os.Stat("/dev/fuse"); os.IsNotExist(err) {
		t.Skip("FUSE not available: /dev/fuse does not exist")
	}

	// Check if fusermount is available
	if _, err := exec.LookPath("fusermount"); err != nil {
		if _, err := exec.LookPath("fusermount3"); err != nil {
			t.Skip("FUSE not available: fusermount not found")
		}
	}
}

// setupMount creates a new GPGFS mount for testing
func setupMount(t *testing.T, passphrase string) *testMount {
	t.Helper()
	skipIfNoFUSE(t)

	// Create temp directory for mount point
	mountPoint, err := os.MkdirTemp("", "gpgfs-mount-*")
	if err != nil {
		t.Fatalf("Failed to create mount point: %v", err)
	}

	// Create temp file for container
	containerFile, err := os.CreateTemp("", "gpgfs-container-*.db")
	if err != nil {
		os.RemoveAll(mountPoint)
		t.Fatalf("Failed to create container file: %v", err)
	}
	containerPath := containerFile.Name()
	containerFile.Close()

	// Initialize storage
	store, err := storage.NewStorage(containerPath, passphrase)
	if err != nil {
		os.RemoveAll(mountPoint)
		os.Remove(containerPath)
		t.Fatalf("Failed to create storage: %v", err)
	}

	// Create the filesystem
	root := NewGPGFS(store).Root()

	// Mount options
	opts := &fs.Options{
		MountOptions: fuse.MountOptions{
			FsName:        "gpgfs-test",
			Name:          "gpgfs",
			DisableXAttrs: true,
			Debug:         false,
		},
	}

	// Mount the filesystem
	server, err := fs.Mount(mountPoint, root, opts)
	if err != nil {
		store.Close()
		os.RemoveAll(mountPoint)
		os.Remove(containerPath)
		t.Fatalf("Failed to mount filesystem: %v", err)
	}

	// Give FUSE a moment to initialize
	time.Sleep(100 * time.Millisecond)

	tm := &testMount{
		mountPoint:    mountPoint,
		containerPath: containerPath,
		storage:       store,
		server:        server,
	}

	return tm
}

// cleanup unmounts and removes temporary files
func (tm *testMount) cleanup(t *testing.T) {
	t.Helper()

	// Unmount
	if err := tm.server.Unmount(); err != nil {
		// Try force unmount
		exec.Command("fusermount", "-uz", tm.mountPoint).Run()
	}

	// Wait for server to finish
	tm.server.Wait()

	// Close storage
	tm.storage.Close()

	// Remove temp files
	os.RemoveAll(tm.mountPoint)
	os.Remove(tm.containerPath)
}

// remount unmounts and remounts with the given passphrase, returning a new testMount
func (tm *testMount) remount(t *testing.T, passphrase string) *testMount {
	t.Helper()

	containerPath := tm.containerPath
	mountPoint := tm.mountPoint

	// Unmount
	if err := tm.server.Unmount(); err != nil {
		exec.Command("fusermount", "-uz", mountPoint).Run()
	}
	tm.server.Wait()
	tm.storage.Close()

	// Give system time to release mount
	time.Sleep(200 * time.Millisecond)

	// Reopen storage
	store, err := storage.NewStorage(containerPath, passphrase)
	if err != nil {
		t.Fatalf("Failed to reopen storage: %v", err)
	}

	// Create the filesystem
	root := NewGPGFS(store).Root()

	opts := &fs.Options{
		MountOptions: fuse.MountOptions{
			FsName:        "gpgfs-test",
			Name:          "gpgfs",
			DisableXAttrs: true,
			Debug:         false,
		},
	}

	server, err := fs.Mount(mountPoint, root, opts)
	if err != nil {
		store.Close()
		t.Fatalf("Failed to remount filesystem: %v", err)
	}

	time.Sleep(100 * time.Millisecond)

	return &testMount{
		mountPoint:    mountPoint,
		containerPath: containerPath,
		storage:       store,
		server:        server,
	}
}

// TestMountUnmount verifies basic mount and unmount functionality
func TestMountUnmount(t *testing.T) {
	tm := setupMount(t, "testpassphrase")
	defer tm.cleanup(t)

	// Verify mount point is accessible
	entries, err := os.ReadDir(tm.mountPoint)
	if err != nil {
		t.Fatalf("Failed to read mount point: %v", err)
	}

	// Should be empty initially
	if len(entries) != 0 {
		t.Errorf("Expected empty directory, got %d entries", len(entries))
	}
}

// TestCreateFile tests file creation via the FUSE mount
func TestCreateFile(t *testing.T) {
	tm := setupMount(t, "testpassphrase")
	defer tm.cleanup(t)

	filePath := filepath.Join(tm.mountPoint, "test.txt")

	// Create file
	f, err := os.Create(filePath)
	if err != nil {
		t.Fatalf("Failed to create file: %v", err)
	}
	f.Close()

	// Verify file exists
	info, err := os.Stat(filePath)
	if err != nil {
		t.Fatalf("Failed to stat file: %v", err)
	}

	if info.IsDir() {
		t.Error("Expected regular file, got directory")
	}
}

// TestWriteReadFile tests writing and reading file content
func TestWriteReadFile(t *testing.T) {
	tm := setupMount(t, "testpassphrase")
	defer tm.cleanup(t)

	filePath := filepath.Join(tm.mountPoint, "hello.txt")
	content := []byte("Hello, encrypted FUSE filesystem!")

	// Write file
	if err := os.WriteFile(filePath, content, 0644); err != nil {
		t.Fatalf("Failed to write file: %v", err)
	}

	// Read file
	readContent, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("Failed to read file: %v", err)
	}

	if !bytes.Equal(readContent, content) {
		t.Errorf("Content mismatch.\nGot: %q\nWant: %q", readContent, content)
	}
}

// TestCreateDirectory tests directory creation
func TestCreateDirectory(t *testing.T) {
	tm := setupMount(t, "testpassphrase")
	defer tm.cleanup(t)

	dirPath := filepath.Join(tm.mountPoint, "testdir")

	// Create directory
	if err := os.Mkdir(dirPath, 0755); err != nil {
		t.Fatalf("Failed to create directory: %v", err)
	}

	// Verify directory exists
	info, err := os.Stat(dirPath)
	if err != nil {
		t.Fatalf("Failed to stat directory: %v", err)
	}

	if !info.IsDir() {
		t.Error("Expected directory, got regular file")
	}
}

// TestNestedDirectories tests creating nested directory structures
func TestNestedDirectories(t *testing.T) {
	tm := setupMount(t, "testpassphrase")
	defer tm.cleanup(t)

	// Create nested structure
	nestedPath := filepath.Join(tm.mountPoint, "a", "b", "c")
	if err := os.MkdirAll(nestedPath, 0755); err != nil {
		t.Fatalf("Failed to create nested directories: %v", err)
	}

	// Create file in nested directory
	filePath := filepath.Join(nestedPath, "deep.txt")
	content := []byte("Deep in the directory tree")

	if err := os.WriteFile(filePath, content, 0644); err != nil {
		t.Fatalf("Failed to write file: %v", err)
	}

	// Read back
	readContent, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("Failed to read file: %v", err)
	}

	if !bytes.Equal(readContent, content) {
		t.Errorf("Content mismatch")
	}
}

// TestReadDir tests directory listing
func TestReadDir(t *testing.T) {
	tm := setupMount(t, "testpassphrase")
	defer tm.cleanup(t)

	// Create some files and directories
	os.WriteFile(filepath.Join(tm.mountPoint, "file1.txt"), []byte("1"), 0644)
	os.WriteFile(filepath.Join(tm.mountPoint, "file2.txt"), []byte("2"), 0644)
	os.Mkdir(filepath.Join(tm.mountPoint, "subdir"), 0755)

	// List directory
	entries, err := os.ReadDir(tm.mountPoint)
	if err != nil {
		t.Fatalf("Failed to read directory: %v", err)
	}

	if len(entries) != 3 {
		t.Errorf("Expected 3 entries, got %d", len(entries))
	}

	// Check names
	names := make([]string, len(entries))
	for i, e := range entries {
		names[i] = e.Name()
	}
	sort.Strings(names)

	expected := []string{"file1.txt", "file2.txt", "subdir"}
	for i, name := range expected {
		if names[i] != name {
			t.Errorf("Expected entry %d to be %q, got %q", i, name, names[i])
		}
	}
}

// TestDeleteFile tests file deletion
func TestDeleteFile(t *testing.T) {
	tm := setupMount(t, "testpassphrase")
	defer tm.cleanup(t)

	filePath := filepath.Join(tm.mountPoint, "todelete.txt")

	// Create file
	os.WriteFile(filePath, []byte("delete me"), 0644)

	// Delete file
	if err := os.Remove(filePath); err != nil {
		t.Fatalf("Failed to delete file: %v", err)
	}

	// Verify file is gone
	if _, err := os.Stat(filePath); !os.IsNotExist(err) {
		t.Error("File should not exist after deletion")
	}
}

// TestDeleteDirectory tests directory deletion
func TestDeleteDirectory(t *testing.T) {
	tm := setupMount(t, "testpassphrase")
	defer tm.cleanup(t)

	dirPath := filepath.Join(tm.mountPoint, "toremove")

	// Create empty directory
	os.Mkdir(dirPath, 0755)

	// Delete directory
	if err := os.Remove(dirPath); err != nil {
		t.Fatalf("Failed to delete directory: %v", err)
	}

	// Verify directory is gone
	if _, err := os.Stat(dirPath); !os.IsNotExist(err) {
		t.Error("Directory should not exist after deletion")
	}
}

// TestDeleteNonEmptyDirectory tests that non-empty directory deletion fails
func TestDeleteNonEmptyDirectory(t *testing.T) {
	tm := setupMount(t, "testpassphrase")
	defer tm.cleanup(t)

	dirPath := filepath.Join(tm.mountPoint, "nonempty")
	filePath := filepath.Join(dirPath, "file.txt")

	// Create directory with file
	os.Mkdir(dirPath, 0755)
	os.WriteFile(filePath, []byte("content"), 0644)

	// Try to delete non-empty directory (should fail)
	err := os.Remove(dirPath)
	if err == nil {
		t.Error("Deleting non-empty directory should fail")
	}
}

// TestRenameFile tests file renaming
func TestRenameFile(t *testing.T) {
	tm := setupMount(t, "testpassphrase")
	defer tm.cleanup(t)

	oldPath := filepath.Join(tm.mountPoint, "original.txt")
	newPath := filepath.Join(tm.mountPoint, "renamed.txt")
	content := []byte("test content for rename")

	// Create file
	os.WriteFile(oldPath, content, 0644)

	// Rename file
	if err := os.Rename(oldPath, newPath); err != nil {
		t.Fatalf("Failed to rename file: %v", err)
	}

	// Verify old path doesn't exist
	if _, err := os.Stat(oldPath); !os.IsNotExist(err) {
		t.Error("Old file should not exist")
	}

	// Verify new path exists with correct content
	readContent, err := os.ReadFile(newPath)
	if err != nil {
		t.Fatalf("Failed to read renamed file: %v", err)
	}

	if !bytes.Equal(readContent, content) {
		t.Error("Content mismatch after rename")
	}
}

// TestRenameDirectory tests directory renaming
func TestRenameDirectory(t *testing.T) {
	tm := setupMount(t, "testpassphrase")
	defer tm.cleanup(t)

	oldDir := filepath.Join(tm.mountPoint, "olddir")
	newDir := filepath.Join(tm.mountPoint, "newdir")
	filePath := "file.txt"
	content := []byte("content in renamed dir")

	// Create directory with file
	os.Mkdir(oldDir, 0755)
	os.WriteFile(filepath.Join(oldDir, filePath), content, 0644)

	// Rename directory
	if err := os.Rename(oldDir, newDir); err != nil {
		t.Fatalf("Failed to rename directory: %v", err)
	}

	// Verify old path doesn't exist
	if _, err := os.Stat(oldDir); !os.IsNotExist(err) {
		t.Error("Old directory should not exist")
	}

	// Verify file in new directory
	readContent, err := os.ReadFile(filepath.Join(newDir, filePath))
	if err != nil {
		t.Fatalf("Failed to read file in renamed directory: %v", err)
	}

	if !bytes.Equal(readContent, content) {
		t.Error("Content mismatch in renamed directory")
	}
}

// TestMoveFileBetweenDirectories tests moving a file to a different directory
func TestMoveFileBetweenDirectories(t *testing.T) {
	tm := setupMount(t, "testpassphrase")
	defer tm.cleanup(t)

	dir1 := filepath.Join(tm.mountPoint, "dir1")
	dir2 := filepath.Join(tm.mountPoint, "dir2")
	content := []byte("moving file content")

	os.Mkdir(dir1, 0755)
	os.Mkdir(dir2, 0755)

	oldPath := filepath.Join(dir1, "moveme.txt")
	newPath := filepath.Join(dir2, "moved.txt")

	os.WriteFile(oldPath, content, 0644)

	// Move file
	if err := os.Rename(oldPath, newPath); err != nil {
		t.Fatalf("Failed to move file: %v", err)
	}

	// Verify old path doesn't exist
	if _, err := os.Stat(oldPath); !os.IsNotExist(err) {
		t.Error("Old file should not exist")
	}

	// Verify new path
	readContent, err := os.ReadFile(newPath)
	if err != nil {
		t.Fatalf("Failed to read moved file: %v", err)
	}

	if !bytes.Equal(readContent, content) {
		t.Error("Content mismatch after move")
	}
}

// TestPersistenceAcrossRemount tests that data persists across unmount/remount
func TestPersistenceAcrossRemount(t *testing.T) {
	passphrase := "persistencetest"
	tm := setupMount(t, passphrase)

	// Create some files and directories
	dir1 := filepath.Join(tm.mountPoint, "persist_dir")
	file1 := filepath.Join(tm.mountPoint, "persist_file.txt")
	nestedFile := filepath.Join(dir1, "nested.txt")

	content1 := []byte("Top level file content")
	content2 := []byte("Nested file content")

	os.Mkdir(dir1, 0755)
	os.WriteFile(file1, content1, 0644)
	os.WriteFile(nestedFile, content2, 0644)

	// Remount
	tm = tm.remount(t, passphrase)
	defer tm.cleanup(t)

	// Verify files exist
	readContent1, err := os.ReadFile(filepath.Join(tm.mountPoint, "persist_file.txt"))
	if err != nil {
		t.Fatalf("Failed to read file after remount: %v", err)
	}
	if !bytes.Equal(readContent1, content1) {
		t.Error("Content mismatch in top level file after remount")
	}

	readContent2, err := os.ReadFile(filepath.Join(tm.mountPoint, "persist_dir", "nested.txt"))
	if err != nil {
		t.Fatalf("Failed to read nested file after remount: %v", err)
	}
	if !bytes.Equal(readContent2, content2) {
		t.Error("Content mismatch in nested file after remount")
	}

	// Verify directory listing
	entries, err := os.ReadDir(tm.mountPoint)
	if err != nil {
		t.Fatalf("Failed to read directory after remount: %v", err)
	}
	if len(entries) != 2 {
		t.Errorf("Expected 2 entries after remount, got %d", len(entries))
	}
}

// TestLargeFile tests handling of larger files
func TestLargeFile(t *testing.T) {
	tm := setupMount(t, "testpassphrase")
	defer tm.cleanup(t)

	filePath := filepath.Join(tm.mountPoint, "large.bin")

	// Create 1MB of random data
	data := make([]byte, 1024*1024)
	if _, err := rand.Read(data); err != nil {
		t.Fatalf("Failed to generate random data: %v", err)
	}

	// Write large file
	if err := os.WriteFile(filePath, data, 0644); err != nil {
		t.Fatalf("Failed to write large file: %v", err)
	}

	// Read back
	readData, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("Failed to read large file: %v", err)
	}

	if !bytes.Equal(readData, data) {
		t.Error("Large file content mismatch")
	}

	// Verify size
	info, err := os.Stat(filePath)
	if err != nil {
		t.Fatalf("Failed to stat large file: %v", err)
	}
	if info.Size() != int64(len(data)) {
		t.Errorf("Size mismatch: got %d, want %d", info.Size(), len(data))
	}
}

// TestFileAppend tests appending to existing files
func TestFileAppend(t *testing.T) {
	tm := setupMount(t, "testpassphrase")
	defer tm.cleanup(t)

	filePath := filepath.Join(tm.mountPoint, "append.txt")

	// Write initial content
	initial := []byte("Initial content\n")
	if err := os.WriteFile(filePath, initial, 0644); err != nil {
		t.Fatalf("Failed to write initial content: %v", err)
	}

	// Append content
	f, err := os.OpenFile(filePath, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		t.Fatalf("Failed to open file for append: %v", err)
	}
	appended := []byte("Appended content\n")
	f.Write(appended)
	f.Close()

	// Read back
	readContent, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("Failed to read file: %v", err)
	}

	expected := append(initial, appended...)
	if !bytes.Equal(readContent, expected) {
		t.Errorf("Content mismatch.\nGot: %q\nWant: %q", readContent, expected)
	}
}

// TestFileTruncate tests file truncation
func TestFileTruncate(t *testing.T) {
	tm := setupMount(t, "testpassphrase")
	defer tm.cleanup(t)

	filePath := filepath.Join(tm.mountPoint, "truncate.txt")
	original := []byte("This is some content that will be truncated")

	// Write file
	if err := os.WriteFile(filePath, original, 0644); err != nil {
		t.Fatalf("Failed to write file: %v", err)
	}

	// Truncate to 10 bytes
	if err := os.Truncate(filePath, 10); err != nil {
		t.Fatalf("Failed to truncate file: %v", err)
	}

	// Read back
	readContent, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("Failed to read file: %v", err)
	}

	expected := original[:10]
	if !bytes.Equal(readContent, expected) {
		t.Errorf("Content mismatch after truncate.\nGot: %q\nWant: %q", readContent, expected)
	}
}

// TestFileOverwrite tests overwriting file contents
func TestFileOverwrite(t *testing.T) {
	tm := setupMount(t, "testpassphrase")
	defer tm.cleanup(t)

	filePath := filepath.Join(tm.mountPoint, "overwrite.txt")

	// Write initial content
	if err := os.WriteFile(filePath, []byte("Original content"), 0644); err != nil {
		t.Fatalf("Failed to write initial content: %v", err)
	}

	// Overwrite with new content
	newContent := []byte("New content")
	if err := os.WriteFile(filePath, newContent, 0644); err != nil {
		t.Fatalf("Failed to overwrite file: %v", err)
	}

	// Read back
	readContent, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("Failed to read file: %v", err)
	}

	if !bytes.Equal(readContent, newContent) {
		t.Errorf("Content mismatch.\nGot: %q\nWant: %q", readContent, newContent)
	}
}

// TestMultipleFilesInDirectory tests creating multiple files in a directory
func TestMultipleFilesInDirectory(t *testing.T) {
	tm := setupMount(t, "testpassphrase")
	defer tm.cleanup(t)

	dirPath := filepath.Join(tm.mountPoint, "multidir")
	os.Mkdir(dirPath, 0755)

	// Create multiple files
	files := map[string][]byte{
		"file1.txt": []byte("Content 1"),
		"file2.txt": []byte("Content 2"),
		"file3.txt": []byte("Content 3"),
		"file4.txt": []byte("Content 4"),
		"file5.txt": []byte("Content 5"),
	}

	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dirPath, name), content, 0644); err != nil {
			t.Fatalf("Failed to write %s: %v", name, err)
		}
	}

	// Verify all files
	for name, expectedContent := range files {
		readContent, err := os.ReadFile(filepath.Join(dirPath, name))
		if err != nil {
			t.Errorf("Failed to read %s: %v", name, err)
			continue
		}
		if !bytes.Equal(readContent, expectedContent) {
			t.Errorf("Content mismatch for %s", name)
		}
	}

	// Verify directory listing
	entries, err := os.ReadDir(dirPath)
	if err != nil {
		t.Fatalf("Failed to read directory: %v", err)
	}
	if len(entries) != len(files) {
		t.Errorf("Expected %d entries, got %d", len(files), len(entries))
	}
}

// TestFilePermissions tests file permission handling
func TestFilePermissions(t *testing.T) {
	tm := setupMount(t, "testpassphrase")
	defer tm.cleanup(t)

	filePath := filepath.Join(tm.mountPoint, "perms.txt")

	// Create file with specific permissions
	f, err := os.OpenFile(filePath, os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatalf("Failed to create file: %v", err)
	}
	f.Close()

	// Check permissions
	info, err := os.Stat(filePath)
	if err != nil {
		t.Fatalf("Failed to stat file: %v", err)
	}

	// The mode should include 0600 (but may also include file type bits)
	if info.Mode().Perm() != 0600 {
		t.Errorf("Expected permissions 0600, got %o", info.Mode().Perm())
	}
}

// TestDirectoryPermissions tests directory permission handling
func TestDirectoryPermissions(t *testing.T) {
	tm := setupMount(t, "testpassphrase")
	defer tm.cleanup(t)

	dirPath := filepath.Join(tm.mountPoint, "permdir")

	// Create directory with specific permissions
	if err := os.Mkdir(dirPath, 0700); err != nil {
		t.Fatalf("Failed to create directory: %v", err)
	}

	// Check permissions
	info, err := os.Stat(dirPath)
	if err != nil {
		t.Fatalf("Failed to stat directory: %v", err)
	}

	if info.Mode().Perm() != 0700 {
		t.Errorf("Expected permissions 0700, got %o", info.Mode().Perm())
	}
}

// TestSeekAndRead tests seeking within a file
func TestSeekAndRead(t *testing.T) {
	tm := setupMount(t, "testpassphrase")
	defer tm.cleanup(t)

	filePath := filepath.Join(tm.mountPoint, "seektest.txt")
	content := []byte("0123456789ABCDEF")

	if err := os.WriteFile(filePath, content, 0644); err != nil {
		t.Fatalf("Failed to write file: %v", err)
	}

	f, err := os.Open(filePath)
	if err != nil {
		t.Fatalf("Failed to open file: %v", err)
	}
	defer f.Close()

	// Seek to offset 5
	if _, err := f.Seek(5, io.SeekStart); err != nil {
		t.Fatalf("Failed to seek: %v", err)
	}

	// Read 4 bytes
	buf := make([]byte, 4)
	n, err := f.Read(buf)
	if err != nil {
		t.Fatalf("Failed to read: %v", err)
	}
	if n != 4 {
		t.Errorf("Expected to read 4 bytes, got %d", n)
	}
	if string(buf) != "5678" {
		t.Errorf("Expected '5678', got %q", string(buf))
	}
}

// TestPartialWrite tests writing at a specific offset
func TestPartialWrite(t *testing.T) {
	tm := setupMount(t, "testpassphrase")
	defer tm.cleanup(t)

	filePath := filepath.Join(tm.mountPoint, "partialwrite.txt")
	initial := []byte("AAAAAAAAAA") // 10 A's

	if err := os.WriteFile(filePath, initial, 0644); err != nil {
		t.Fatalf("Failed to write initial content: %v", err)
	}

	// Open for read/write, seek to middle, and write
	f, err := os.OpenFile(filePath, os.O_RDWR, 0644)
	if err != nil {
		t.Fatalf("Failed to open file: %v", err)
	}

	if _, err := f.Seek(3, io.SeekStart); err != nil {
		t.Fatalf("Failed to seek: %v", err)
	}

	if _, err := f.Write([]byte("BBB")); err != nil {
		t.Fatalf("Failed to write: %v", err)
	}
	f.Close()

	// Read back
	readContent, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("Failed to read file: %v", err)
	}

	expected := []byte("AAABBBAAAA")
	if !bytes.Equal(readContent, expected) {
		t.Errorf("Content mismatch.\nGot: %q\nWant: %q", readContent, expected)
	}
}

// TestEmptyFile tests creating and reading an empty file
func TestEmptyFile(t *testing.T) {
	tm := setupMount(t, "testpassphrase")
	defer tm.cleanup(t)

	filePath := filepath.Join(tm.mountPoint, "empty.txt")

	// Create empty file
	f, err := os.Create(filePath)
	if err != nil {
		t.Fatalf("Failed to create file: %v", err)
	}
	f.Close()

	// Read empty file
	content, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("Failed to read empty file: %v", err)
	}

	if len(content) != 0 {
		t.Errorf("Expected empty content, got %d bytes", len(content))
	}

	// Verify size
	info, err := os.Stat(filePath)
	if err != nil {
		t.Fatalf("Failed to stat file: %v", err)
	}
	if info.Size() != 0 {
		t.Errorf("Expected size 0, got %d", info.Size())
	}
}

// TestSpecialCharactersInFilename tests filenames with special characters
func TestSpecialCharactersInFilename(t *testing.T) {
	tm := setupMount(t, "testpassphrase")
	defer tm.cleanup(t)

	// Test various special characters (but not / which is not allowed)
	testNames := []string{
		"file with spaces.txt",
		"file-with-dashes.txt",
		"file_with_underscores.txt",
		"file.multiple.dots.txt",
		"UPPERCASE.TXT",
		"MixedCase.Txt",
		"123numeric.txt",
	}

	for _, name := range testNames {
		filePath := filepath.Join(tm.mountPoint, name)
		content := []byte("Content of " + name)

		if err := os.WriteFile(filePath, content, 0644); err != nil {
			t.Errorf("Failed to write %q: %v", name, err)
			continue
		}

		readContent, err := os.ReadFile(filePath)
		if err != nil {
			t.Errorf("Failed to read %q: %v", name, err)
			continue
		}

		if !bytes.Equal(readContent, content) {
			t.Errorf("Content mismatch for %q", name)
		}
	}
}

// TestConcurrentReads tests concurrent read operations
func TestConcurrentReads(t *testing.T) {
	tm := setupMount(t, "testpassphrase")
	defer tm.cleanup(t)

	filePath := filepath.Join(tm.mountPoint, "concurrent.txt")
	content := []byte("Content for concurrent read test")

	if err := os.WriteFile(filePath, content, 0644); err != nil {
		t.Fatalf("Failed to write file: %v", err)
	}

	// Spawn multiple goroutines to read concurrently
	done := make(chan error, 10)
	for i := 0; i < 10; i++ {
		go func() {
			readContent, err := os.ReadFile(filePath)
			if err != nil {
				done <- err
				return
			}
			if !bytes.Equal(readContent, content) {
				done <- fmt.Errorf("content mismatch")
				return
			}
			done <- nil
		}()
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		if err := <-done; err != nil {
			t.Errorf("Concurrent read error: %v", err)
		}
	}
}

// TestFileNotFound tests accessing non-existent files
func TestFileNotFound(t *testing.T) {
	tm := setupMount(t, "testpassphrase")
	defer tm.cleanup(t)

	filePath := filepath.Join(tm.mountPoint, "nonexistent.txt")

	_, err := os.ReadFile(filePath)
	if !os.IsNotExist(err) {
		t.Errorf("Expected not exist error, got: %v", err)
	}
}

// TestDeleteNonExistentFile tests deleting a non-existent file
func TestDeleteNonExistentFile(t *testing.T) {
	tm := setupMount(t, "testpassphrase")
	defer tm.cleanup(t)

	filePath := filepath.Join(tm.mountPoint, "nonexistent.txt")

	err := os.Remove(filePath)
	if !os.IsNotExist(err) {
		t.Errorf("Expected not exist error, got: %v", err)
	}
}

// TestCreateExistingFile tests creating a file that already exists
func TestCreateExistingFile(t *testing.T) {
	tm := setupMount(t, "testpassphrase")
	defer tm.cleanup(t)

	filePath := filepath.Join(tm.mountPoint, "existing.txt")

	// Create file
	os.WriteFile(filePath, []byte("original"), 0644)

	// Creating again with O_EXCL should fail
	_, err := os.OpenFile(filePath, os.O_CREATE|os.O_EXCL, 0644)
	if err == nil {
		t.Error("Expected error when creating existing file with O_EXCL")
	}
}

// TestCreateExistingDirectory tests creating a directory that already exists
func TestCreateExistingDirectory(t *testing.T) {
	tm := setupMount(t, "testpassphrase")
	defer tm.cleanup(t)

	dirPath := filepath.Join(tm.mountPoint, "existingdir")

	// Create directory
	os.Mkdir(dirPath, 0755)

	// Try to create again
	err := os.Mkdir(dirPath, 0755)
	if err == nil {
		t.Error("Expected error when creating existing directory")
	}
}

// TestDeepNesting tests deeply nested directory structures
func TestDeepNesting(t *testing.T) {
	tm := setupMount(t, "testpassphrase")
	defer tm.cleanup(t)

	// Create a deep path
	parts := make([]string, 20)
	for i := range parts {
		parts[i] = fmt.Sprintf("level%d", i)
	}
	deepPath := filepath.Join(tm.mountPoint, filepath.Join(parts...))

	if err := os.MkdirAll(deepPath, 0755); err != nil {
		t.Fatalf("Failed to create deep path: %v", err)
	}

	// Create file at the deepest level
	filePath := filepath.Join(deepPath, "deep.txt")
	content := []byte("Deep content")

	if err := os.WriteFile(filePath, content, 0644); err != nil {
		t.Fatalf("Failed to write deep file: %v", err)
	}

	// Read back
	readContent, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("Failed to read deep file: %v", err)
	}

	if !bytes.Equal(readContent, content) {
		t.Error("Deep file content mismatch")
	}
}

// TestRemoveAllDirectory tests recursive directory removal
func TestRemoveAllDirectory(t *testing.T) {
	tm := setupMount(t, "testpassphrase")
	defer tm.cleanup(t)

	dirPath := filepath.Join(tm.mountPoint, "removeall")
	os.MkdirAll(filepath.Join(dirPath, "sub1", "sub2"), 0755)
	os.WriteFile(filepath.Join(dirPath, "file1.txt"), []byte("1"), 0644)
	os.WriteFile(filepath.Join(dirPath, "sub1", "file2.txt"), []byte("2"), 0644)
	os.WriteFile(filepath.Join(dirPath, "sub1", "sub2", "file3.txt"), []byte("3"), 0644)

	// Remove all
	if err := os.RemoveAll(dirPath); err != nil {
		t.Fatalf("Failed to remove all: %v", err)
	}

	// Verify directory is gone
	if _, err := os.Stat(dirPath); !os.IsNotExist(err) {
		t.Error("Directory should not exist after RemoveAll")
	}
}

// TestLargeFileRemount tests that large files persist across remount
func TestLargeFileRemount(t *testing.T) {
	passphrase := "largefiletest"
	tm := setupMount(t, passphrase)

	filePath := "largepersist.bin"
	fullPath := filepath.Join(tm.mountPoint, filePath)

	// Create 512KB of random data
	data := make([]byte, 512*1024)
	if _, err := rand.Read(data); err != nil {
		tm.cleanup(t)
		t.Fatalf("Failed to generate random data: %v", err)
	}

	if err := os.WriteFile(fullPath, data, 0644); err != nil {
		tm.cleanup(t)
		t.Fatalf("Failed to write large file: %v", err)
	}

	// Remount
	tm = tm.remount(t, passphrase)
	defer tm.cleanup(t)

	// Read and verify
	readData, err := os.ReadFile(filepath.Join(tm.mountPoint, filePath))
	if err != nil {
		t.Fatalf("Failed to read large file after remount: %v", err)
	}

	if !bytes.Equal(readData, data) {
		t.Error("Large file content mismatch after remount")
	}
}

// TestBinaryFileContent tests handling of binary file content
func TestBinaryFileContent(t *testing.T) {
	tm := setupMount(t, "testpassphrase")
	defer tm.cleanup(t)

	filePath := filepath.Join(tm.mountPoint, "binary.bin")

	// Create binary data with all byte values
	data := make([]byte, 256)
	for i := range data {
		data[i] = byte(i)
	}

	if err := os.WriteFile(filePath, data, 0644); err != nil {
		t.Fatalf("Failed to write binary file: %v", err)
	}

	readData, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("Failed to read binary file: %v", err)
	}

	if !bytes.Equal(readData, data) {
		t.Error("Binary file content mismatch")
	}
}

// TestFileStatAfterWrite tests that file stat reflects correct size after write
func TestFileStatAfterWrite(t *testing.T) {
	tm := setupMount(t, "testpassphrase")
	defer tm.cleanup(t)

	filePath := filepath.Join(tm.mountPoint, "stattest.txt")
	content := []byte("Hello, World!")

	if err := os.WriteFile(filePath, content, 0644); err != nil {
		t.Fatalf("Failed to write file: %v", err)
	}

	info, err := os.Stat(filePath)
	if err != nil {
		t.Fatalf("Failed to stat file: %v", err)
	}

	if info.Size() != int64(len(content)) {
		t.Errorf("Size mismatch: got %d, want %d", info.Size(), len(content))
	}
}

// TestUnlinkOpenFile tests behavior when deleting an open file
func TestUnlinkOpenFile(t *testing.T) {
	tm := setupMount(t, "testpassphrase")
	defer tm.cleanup(t)

	filePath := filepath.Join(tm.mountPoint, "openunlink.txt")
	content := []byte("Content of file to unlink while open")

	if err := os.WriteFile(filePath, content, 0644); err != nil {
		t.Fatalf("Failed to write file: %v", err)
	}

	// Open the file
	f, err := os.Open(filePath)
	if err != nil {
		t.Fatalf("Failed to open file: %v", err)
	}
	defer f.Close()

	// Delete the file while it's open
	if err := os.Remove(filePath); err != nil {
		t.Fatalf("Failed to delete open file: %v", err)
	}

	// File should no longer be accessible by name
	if _, err := os.Stat(filePath); !os.IsNotExist(err) {
		t.Error("File should not exist after deletion")
	}
}

// TestSymlinksNotSupported verifies symlinks aren't supported (or handles gracefully)
func TestSymlinksNotSupported(t *testing.T) {
	tm := setupMount(t, "testpassphrase")
	defer tm.cleanup(t)

	filePath := filepath.Join(tm.mountPoint, "target.txt")
	linkPath := filepath.Join(tm.mountPoint, "link.txt")

	os.WriteFile(filePath, []byte("target content"), 0644)

	// Try to create symlink - should fail
	err := os.Symlink(filePath, linkPath)
	if err == nil {
		t.Log("Symlinks are supported (unexpected but not an error)")
		return
	}

	// Verify the error is appropriate (ENOSYS or similar)
	if !strings.Contains(err.Error(), "not supported") &&
		!strings.Contains(err.Error(), "operation not permitted") &&
		!os.IsPermission(err) {
		// Some form of error is expected
		t.Logf("Symlink creation failed as expected: %v", err)
	}
}

// TestTruncateToZero tests truncating a file to zero length
func TestTruncateToZero(t *testing.T) {
	tm := setupMount(t, "testpassphrase")
	defer tm.cleanup(t)

	filePath := filepath.Join(tm.mountPoint, "trunczero.txt")

	// Write initial content
	if err := os.WriteFile(filePath, []byte("Some content here"), 0644); err != nil {
		t.Fatalf("Failed to write file: %v", err)
	}

	// Truncate to zero
	if err := os.Truncate(filePath, 0); err != nil {
		t.Fatalf("Failed to truncate: %v", err)
	}

	// Read back
	content, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("Failed to read file: %v", err)
	}

	if len(content) != 0 {
		t.Errorf("Expected empty file, got %d bytes", len(content))
	}
}

// TestExtendWithTruncate tests extending a file using truncate
func TestExtendWithTruncate(t *testing.T) {
	tm := setupMount(t, "testpassphrase")
	defer tm.cleanup(t)

	filePath := filepath.Join(tm.mountPoint, "extend.txt")
	initial := []byte("ABC")

	if err := os.WriteFile(filePath, initial, 0644); err != nil {
		t.Fatalf("Failed to write file: %v", err)
	}

	// Extend to 10 bytes
	if err := os.Truncate(filePath, 10); err != nil {
		t.Fatalf("Failed to extend with truncate: %v", err)
	}

	content, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("Failed to read file: %v", err)
	}

	if len(content) != 10 {
		t.Errorf("Expected 10 bytes, got %d", len(content))
	}

	// First 3 bytes should be "ABC"
	if string(content[:3]) != "ABC" {
		t.Errorf("Expected 'ABC' at start, got %q", string(content[:3]))
	}

	// Remaining bytes should be zero
	for i := 3; i < 10; i++ {
		if content[i] != 0 {
			t.Errorf("Expected zero byte at position %d, got %d", i, content[i])
		}
	}
}

// TestDirectoryEntryTypes tests that directory entries have correct types
func TestDirectoryEntryTypes(t *testing.T) {
	tm := setupMount(t, "testpassphrase")
	defer tm.cleanup(t)

	// Create file and directory
	os.WriteFile(filepath.Join(tm.mountPoint, "afile.txt"), []byte("content"), 0644)
	os.Mkdir(filepath.Join(tm.mountPoint, "adir"), 0755)

	entries, err := os.ReadDir(tm.mountPoint)
	if err != nil {
		t.Fatalf("Failed to read directory: %v", err)
	}

	for _, entry := range entries {
		if entry.Name() == "afile.txt" {
			if entry.IsDir() {
				t.Error("afile.txt should not be a directory")
			}
		} else if entry.Name() == "adir" {
			if !entry.IsDir() {
				t.Error("adir should be a directory")
			}
		}
	}
}

// TestRenameOverwrite tests renaming a file over an existing file
func TestRenameOverwrite(t *testing.T) {
	tm := setupMount(t, "testpassphrase")
	defer tm.cleanup(t)

	srcPath := filepath.Join(tm.mountPoint, "source.txt")
	dstPath := filepath.Join(tm.mountPoint, "destination.txt")

	srcContent := []byte("source content")
	dstContent := []byte("destination content")

	os.WriteFile(srcPath, srcContent, 0644)
	os.WriteFile(dstPath, dstContent, 0644)

	// Rename source over destination
	if err := os.Rename(srcPath, dstPath); err != nil {
		t.Fatalf("Failed to rename: %v", err)
	}

	// Source should not exist
	if _, err := os.Stat(srcPath); !os.IsNotExist(err) {
		t.Error("Source file should not exist")
	}

	// Destination should have source content
	content, err := os.ReadFile(dstPath)
	if err != nil {
		t.Fatalf("Failed to read destination: %v", err)
	}

	if !bytes.Equal(content, srcContent) {
		t.Errorf("Expected source content in destination")
	}
}

// TestDeleteDirectoryAsFile tests that deleting a directory as a file fails
func TestDeleteDirectoryAsFile(t *testing.T) {
	tm := setupMount(t, "testpassphrase")
	defer tm.cleanup(t)

	dirPath := filepath.Join(tm.mountPoint, "adir")
	os.Mkdir(dirPath, 0755)

	// unlink should fail on a directory
	err := syscall.Unlink(dirPath)
	if err == nil {
		t.Error("Unlink on directory should fail")
	}
}
