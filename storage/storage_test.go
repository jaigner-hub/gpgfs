package storage

import (
	"os"
	"path/filepath"
	"testing"
)

func setupTestStorage(t *testing.T) (*Storage, func()) {
	tmpFile, err := os.CreateTemp("", "gpgfs-test-*.db")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	tmpFile.Close()

	store, err := NewStorage(tmpFile.Name(), "testpassphrase")
	if err != nil {
		os.Remove(tmpFile.Name())
		t.Fatalf("Failed to create storage: %v", err)
	}

	cleanup := func() {
		store.Close()
		os.Remove(tmpFile.Name())
	}

	return store, cleanup
}

func TestCreateFile(t *testing.T) {
	store, cleanup := setupTestStorage(t)
	defer cleanup()

	entry, err := store.CreateFile("/test.txt", 0644)
	if err != nil {
		t.Fatalf("Failed to create file: %v", err)
	}

	if entry.Name != "test.txt" {
		t.Errorf("Expected name 'test.txt', got '%s'", entry.Name)
	}

	if entry.Type != FileTypeRegular {
		t.Error("Expected regular file type")
	}
}

func TestWriteReadFile(t *testing.T) {
	store, cleanup := setupTestStorage(t)
	defer cleanup()

	_, err := store.CreateFile("/test.txt", 0644)
	if err != nil {
		t.Fatalf("Failed to create file: %v", err)
	}

	content := []byte("Hello, encrypted filesystem!")
	if err := store.WriteFile("/test.txt", content); err != nil {
		t.Fatalf("Failed to write file: %v", err)
	}

	readContent, err := store.ReadFile("/test.txt")
	if err != nil {
		t.Fatalf("Failed to read file: %v", err)
	}

	if string(readContent) != string(content) {
		t.Errorf("Content mismatch. Got '%s', want '%s'", readContent, content)
	}
}

func TestCreateDirectory(t *testing.T) {
	store, cleanup := setupTestStorage(t)
	defer cleanup()

	entry, err := store.CreateDirectory("/testdir", 0755)
	if err != nil {
		t.Fatalf("Failed to create directory: %v", err)
	}

	if entry.Name != "testdir" {
		t.Errorf("Expected name 'testdir', got '%s'", entry.Name)
	}

	if entry.Type != FileTypeDirectory {
		t.Error("Expected directory type")
	}
}

func TestListDirectory(t *testing.T) {
	store, cleanup := setupTestStorage(t)
	defer cleanup()

	store.CreateFile("/file1.txt", 0644)
	store.CreateFile("/file2.txt", 0644)
	store.CreateDirectory("/subdir", 0755)

	entries, err := store.ListDirectory("/")
	if err != nil {
		t.Fatalf("Failed to list directory: %v", err)
	}

	if len(entries) != 3 {
		t.Errorf("Expected 3 entries, got %d", len(entries))
	}
}

func TestNestedFiles(t *testing.T) {
	store, cleanup := setupTestStorage(t)
	defer cleanup()

	_, err := store.CreateDirectory("/subdir", 0755)
	if err != nil {
		t.Fatalf("Failed to create subdirectory: %v", err)
	}

	_, err = store.CreateFile("/subdir/nested.txt", 0644)
	if err != nil {
		t.Fatalf("Failed to create nested file: %v", err)
	}

	content := []byte("Nested content")
	if err := store.WriteFile("/subdir/nested.txt", content); err != nil {
		t.Fatalf("Failed to write nested file: %v", err)
	}

	readContent, err := store.ReadFile("/subdir/nested.txt")
	if err != nil {
		t.Fatalf("Failed to read nested file: %v", err)
	}

	if string(readContent) != string(content) {
		t.Errorf("Content mismatch. Got '%s', want '%s'", readContent, content)
	}
}

func TestDeleteFile(t *testing.T) {
	store, cleanup := setupTestStorage(t)
	defer cleanup()

	store.CreateFile("/todelete.txt", 0644)
	store.WriteFile("/todelete.txt", []byte("delete me"))

	if err := store.DeleteFile("/todelete.txt"); err != nil {
		t.Fatalf("Failed to delete file: %v", err)
	}

	_, err := store.GetEntry("/todelete.txt")
	if err != ErrNotFound {
		t.Error("Expected ErrNotFound after deletion")
	}
}

func TestRename(t *testing.T) {
	store, cleanup := setupTestStorage(t)
	defer cleanup()

	store.CreateFile("/original.txt", 0644)
	store.WriteFile("/original.txt", []byte("test content"))

	if err := store.Rename("/original.txt", "/renamed.txt"); err != nil {
		t.Fatalf("Failed to rename: %v", err)
	}

	_, err := store.GetEntry("/original.txt")
	if err != ErrNotFound {
		t.Error("Original file should not exist")
	}

	entry, err := store.GetEntry("/renamed.txt")
	if err != nil {
		t.Fatalf("Renamed file should exist: %v", err)
	}

	if entry.Name != "renamed.txt" {
		t.Errorf("Expected name 'renamed.txt', got '%s'", entry.Name)
	}
}

func TestPersistence(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "gpgfs-persist-*.db")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	tmpPath := tmpFile.Name()
	tmpFile.Close()
	defer os.Remove(tmpPath)

	// Create storage and add files
	store1, err := NewStorage(tmpPath, "testpassphrase")
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}

	store1.CreateFile("/persist.txt", 0644)
	store1.WriteFile("/persist.txt", []byte("persistent data"))
	store1.Close()

	// Reopen storage
	store2, err := NewStorage(tmpPath, "testpassphrase")
	if err != nil {
		t.Fatalf("Failed to reopen storage: %v", err)
	}
	defer store2.Close()

	content, err := store2.ReadFile("/persist.txt")
	if err != nil {
		t.Fatalf("Failed to read persistent file: %v", err)
	}

	if string(content) != "persistent data" {
		t.Errorf("Persistence failed. Got '%s', want 'persistent data'", content)
	}
}

func TestContainerFile(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "gpgfs-container-*.db")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	tmpPath := tmpFile.Name()
	tmpFile.Close()
	defer os.Remove(tmpPath)

	store, err := NewStorage(tmpPath, "testpassphrase")
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}
	store.Close()

	// Check that container file exists
	if _, err := os.Stat(tmpPath); os.IsNotExist(err) {
		t.Error("Container file should exist")
	}
}

func TestLargeFile(t *testing.T) {
	store, cleanup := setupTestStorage(t)
	defer cleanup()

	_, err := store.CreateFile("/large.bin", 0644)
	if err != nil {
		t.Fatalf("Failed to create file: %v", err)
	}

	// 1MB of data
	data := make([]byte, 1024*1024)
	for i := range data {
		data[i] = byte(i % 256)
	}

	if err := store.WriteFile("/large.bin", data); err != nil {
		t.Fatalf("Failed to write large file: %v", err)
	}

	readData, err := store.ReadFile("/large.bin")
	if err != nil {
		t.Fatalf("Failed to read large file: %v", err)
	}

	if len(readData) != len(data) {
		t.Errorf("Size mismatch. Got %d, want %d", len(readData), len(data))
	}

	for i := range data {
		if readData[i] != data[i] {
			t.Errorf("Data mismatch at byte %d", i)
			break
		}
	}
}

func TestRenameDirectory(t *testing.T) {
	store, cleanup := setupTestStorage(t)
	defer cleanup()

	store.CreateDirectory("/olddir", 0755)
	store.CreateFile("/olddir/file.txt", 0644)
	store.WriteFile("/olddir/file.txt", []byte("content"))

	if err := store.Rename("/olddir", "/newdir"); err != nil {
		t.Fatalf("Failed to rename directory: %v", err)
	}

	// Check old path doesn't exist
	_, err := store.GetEntry("/olddir")
	if err != ErrNotFound {
		t.Error("Old directory should not exist")
	}

	// Check new path exists
	_, err = store.GetEntry("/newdir")
	if err != nil {
		t.Fatalf("New directory should exist: %v", err)
	}

	// Check child was moved
	content, err := store.ReadFile("/newdir/file.txt")
	if err != nil {
		t.Fatalf("Child file should exist: %v", err)
	}
	if string(content) != "content" {
		t.Errorf("Content mismatch: got '%s'", content)
	}
}

func TestGetAbsPath(t *testing.T) {
	store, cleanup := setupTestStorage(t)
	defer cleanup()

	// Root should exist
	entry, err := store.GetEntry("/")
	if err != nil {
		t.Fatalf("Root should exist: %v", err)
	}
	if entry.Type != FileTypeDirectory {
		t.Error("Root should be a directory")
	}
}

func TestDeleteNonEmptyDirectory(t *testing.T) {
	store, cleanup := setupTestStorage(t)
	defer cleanup()

	store.CreateDirectory("/notempty", 0755)
	store.CreateFile("/notempty/file.txt", 0644)

	err := store.DeleteDirectory("/notempty")
	if err != ErrNotEmpty {
		t.Errorf("Expected ErrNotEmpty, got: %v", err)
	}
}

func TestReopenWithSamePassphrase(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "gpgfs-reopen-*.db")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	tmpPath := tmpFile.Name()
	tmpFile.Close()
	defer os.Remove(tmpPath)

	// Create with passphrase
	store1, err := NewStorage(tmpPath, "mypassphrase")
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}
	store1.CreateFile("/secret.txt", 0644)
	store1.WriteFile("/secret.txt", []byte("secret data"))
	store1.Close()

	// Reopen with same passphrase
	store2, err := NewStorage(tmpPath, "mypassphrase")
	if err != nil {
		t.Fatalf("Failed to reopen storage: %v", err)
	}
	defer store2.Close()

	// Reading should succeed
	content, err := store2.ReadFile("/secret.txt")
	if err != nil {
		t.Fatalf("Failed to read file: %v", err)
	}
	if string(content) != "secret data" {
		t.Errorf("Content mismatch: got '%s'", content)
	}
}

func BenchmarkWriteFile(b *testing.B) {
	tmpFile, _ := os.CreateTemp("", "gpgfs-bench-*.db")
	tmpPath := tmpFile.Name()
	tmpFile.Close()
	defer os.Remove(tmpPath)

	store, _ := NewStorage(tmpPath, "testpassphrase")
	defer store.Close()

	data := make([]byte, 4096)
	for i := range data {
		data[i] = byte(i % 256)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		path := filepath.Join("/", "bench"+string(rune('0'+i%10))+".txt")
		store.CreateFile(path, 0644)
		store.WriteFile(path, data)
	}
}
