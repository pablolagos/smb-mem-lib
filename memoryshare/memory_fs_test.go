package memoryshare

import (
	"io"
	"testing"

	"github.com/pablolagos/smb-mem-lib/vfs"
)

func TestMemoryFS_VFS_Operations(t *testing.T) {
	fs := NewMemoryFS()

	// Add test files
	err := fs.AddFile("/test.txt", []byte("test content"), 0644)
	if err != nil {
		t.Fatalf("AddFile failed: %v", err)
	}

	// Test Open
	handle, err := fs.Open("/test.txt", 0, 0)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	if handle == 0 {
		t.Error("Open returned invalid handle")
	}

	// Test GetAttr
	attrs, err := fs.GetAttr(handle)
	if err != nil {
		t.Fatalf("GetAttr failed: %v", err)
	}
	if attrs == nil {
		t.Fatal("GetAttr returned nil attributes")
	}

	size, ok := attrs.GetSizeBytes()
	if !ok || size != 12 {
		t.Errorf("File size = %d, want 12", size)
	}

	fileType := attrs.GetFileType()
	if fileType != vfs.FileTypeRegularFile {
		t.Error("File type should be regular file")
	}

	// Test Read
	buf := make([]byte, 100)
	n, err := fs.Read(handle, buf, 0, 0)
	if err != nil {
		t.Fatalf("Read failed: %v", err)
	}
	if n != 12 {
		t.Errorf("Read %d bytes, want 12", n)
	}
	if string(buf[:n]) != "test content" {
		t.Errorf("Read content = %q, want %q", string(buf[:n]), "test content")
	}

	// Test Close
	err = fs.Close(handle)
	if err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	// Test Close on invalid handle
	err = fs.Close(handle)
	if err == nil {
		t.Error("Close on invalid handle should fail")
	}
}

func TestMemoryFS_Write(t *testing.T) {
	fs := NewMemoryFS()

	// Create empty file
	err := fs.AddFile("/write_test.txt", []byte(""), 0644)
	if err != nil {
		t.Fatalf("AddFile failed: %v", err)
	}

	handle, err := fs.Open("/write_test.txt", 0, 0)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer fs.Close(handle)

	// Write data
	data := []byte("Hello, World!")
	n, err := fs.Write(handle, data, 0, 0)
	if err != nil {
		t.Fatalf("Write failed: %v", err)
	}
	if n != len(data) {
		t.Errorf("Wrote %d bytes, want %d", n, len(data))
	}

	// Read back
	buf := make([]byte, 100)
	n, err = fs.Read(handle, buf, 0, 0)
	if err != nil {
		t.Fatalf("Read failed: %v", err)
	}
	if string(buf[:n]) != string(data) {
		t.Errorf("Read %q, want %q", string(buf[:n]), string(data))
	}

	// Test append write
	appendData := []byte(" More text")
	n, err = fs.Write(handle, appendData, uint64(len(data)), 0)
	if err != nil {
		t.Fatalf("Append write failed: %v", err)
	}

	// Read all
	buf = make([]byte, 100)
	n, err = fs.Read(handle, buf, 0, 0)
	if err != nil {
		t.Fatalf("Read failed: %v", err)
	}
	expected := "Hello, World! More text"
	if string(buf[:n]) != expected {
		t.Errorf("Read %q, want %q", string(buf[:n]), expected)
	}
}

func TestMemoryFS_Directory_Operations(t *testing.T) {
	fs := NewMemoryFS()

	// Create directory
	err := fs.AddDirectory("/testdir", 0755)
	if err != nil {
		t.Fatalf("AddDirectory failed: %v", err)
	}

	// Add files to directory
	fs.AddFile("/testdir/file1.txt", []byte("content1"), 0644)
	fs.AddFile("/testdir/file2.txt", []byte("content2"), 0644)
	fs.AddFile("/testdir/file3.txt", []byte("content3"), 0644)

	// Open directory
	handle, err := fs.OpenDir("/testdir")
	if err != nil {
		t.Fatalf("OpenDir failed: %v", err)
	}
	defer fs.Close(handle)

	// Read directory entries
	entries, err := fs.ReadDir(handle, 0, 0)
	if err != nil {
		t.Fatalf("ReadDir failed: %v", err)
	}

	// Should have . and .. plus 3 files
	if len(entries) < 5 {
		t.Errorf("ReadDir returned %d entries, want at least 5", len(entries))
	}

	// Test reading directory again (should return EOF)
	entries, err = fs.ReadDir(handle, 0, 0)
	if err != io.EOF {
		t.Errorf("Second ReadDir should return EOF, got %v", err)
	}
}

func TestMemoryFS_Mkdir(t *testing.T) {
	fs := NewMemoryFS()

	// Create directory via Mkdir
	attrs, err := fs.Mkdir("/newdir", 0755)
	if err != nil {
		t.Fatalf("Mkdir failed: %v", err)
	}
	if attrs == nil {
		t.Fatal("Mkdir returned nil attributes")
	}

	fileType := attrs.GetFileType()
	if fileType != vfs.FileTypeDirectory {
		t.Error("Created node should be a directory")
	}

	// Try to create same directory again
	_, err = fs.Mkdir("/newdir", 0755)
	if err == nil {
		t.Error("Creating duplicate directory should fail")
	}
}

func TestMemoryFS_Truncate(t *testing.T) {
	fs := NewMemoryFS()

	// Create file with content
	err := fs.AddFile("/truncate.txt", []byte("Hello, World! This is a long text."), 0644)
	if err != nil {
		t.Fatalf("AddFile failed: %v", err)
	}

	handle, err := fs.Open("/truncate.txt", 0, 0)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer fs.Close(handle)

	// Truncate to smaller size
	err = fs.Truncate(handle, 5)
	if err != nil {
		t.Fatalf("Truncate failed: %v", err)
	}

	// Verify size
	attrs, _ := fs.GetAttr(handle)
	size, _ := attrs.GetSizeBytes()
	if size != 5 {
		t.Errorf("File size after truncate = %d, want 5", size)
	}

	// Read and verify content
	buf := make([]byte, 10)
	n, _ := fs.Read(handle, buf, 0, 0)
	if string(buf[:n]) != "Hello" {
		t.Errorf("Content after truncate = %q, want %q", string(buf[:n]), "Hello")
	}
}

func TestMemoryFS_Rename(t *testing.T) {
	fs := NewMemoryFS()

	// Create file
	err := fs.AddFile("/old.txt", []byte("content"), 0644)
	if err != nil {
		t.Fatalf("AddFile failed: %v", err)
	}

	handle, err := fs.Open("/old.txt", 0, 0)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer fs.Close(handle)

	// Rename file
	err = fs.Rename(handle, "/new.txt", 0)
	if err != nil {
		t.Fatalf("Rename failed: %v", err)
	}

	// Old path should not exist
	_, err = fs.resolvePath("/old.txt")
	if err == nil {
		t.Error("Old path should not exist after rename")
	}

	// New path should exist
	node, err := fs.resolvePath("/new.txt")
	if err != nil {
		t.Fatalf("New path should exist after rename: %v", err)
	}
	if node.name != "new.txt" {
		t.Errorf("Node name = %s, want new.txt", node.name)
	}
}

func TestMemoryFS_Unlink(t *testing.T) {
	fs := NewMemoryFS()

	// Create file
	err := fs.AddFile("/delete_me.txt", []byte("temporary"), 0644)
	if err != nil {
		t.Fatalf("AddFile failed: %v", err)
	}

	handle, err := fs.Open("/delete_me.txt", 0, 0)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}

	// Delete file
	err = fs.Unlink(handle)
	if err != nil {
		t.Fatalf("Unlink failed: %v", err)
	}

	fs.Close(handle)

	// File should not exist
	_, err = fs.resolvePath("/delete_me.txt")
	if err == nil {
		t.Error("File should not exist after Unlink")
	}
}

func TestMemoryFS_Lookup(t *testing.T) {
	fs := NewMemoryFS()

	// Create directory with files
	fs.AddDirectory("/lookupdir", 0755)
	fs.AddFile("/lookupdir/file1.txt", []byte("test"), 0644)

	// Open directory
	dirHandle, err := fs.OpenDir("/lookupdir")
	if err != nil {
		t.Fatalf("OpenDir failed: %v", err)
	}
	defer fs.Close(dirHandle)

	// Lookup file in directory
	attrs, err := fs.Lookup(dirHandle, "file1.txt")
	if err != nil {
		t.Fatalf("Lookup failed: %v", err)
	}
	if attrs == nil {
		t.Fatal("Lookup returned nil attributes")
	}

	fileType := attrs.GetFileType()
	if fileType != vfs.FileTypeRegularFile {
		t.Error("Looked up node should be a regular file")
	}

	// Lookup non-existent file
	_, err = fs.Lookup(dirHandle, "nonexistent.txt")
	if err == nil {
		t.Error("Lookup of non-existent file should fail")
	}
}

func TestMemoryFS_ExtendedAttributes(t *testing.T) {
	fs := NewMemoryFS()

	// Create file
	err := fs.AddFile("/xattr_test.txt", []byte("test"), 0644)
	if err != nil {
		t.Fatalf("AddFile failed: %v", err)
	}

	handle, err := fs.Open("/xattr_test.txt", 0, 0)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer fs.Close(handle)

	// Set extended attribute
	err = fs.Setxattr(handle, "user.comment", []byte("This is a comment"))
	if err != nil {
		t.Fatalf("Setxattr failed: %v", err)
	}

	// List extended attributes
	keys, err := fs.Listxattr(handle)
	if err != nil {
		t.Fatalf("Listxattr failed: %v", err)
	}
	if len(keys) != 1 {
		t.Errorf("Listxattr returned %d keys, want 1", len(keys))
	}
	if keys[0] != "user.comment" {
		t.Errorf("Listxattr returned key %q, want %q", keys[0], "user.comment")
	}

	// Get extended attribute
	buf := make([]byte, 100)
	n, err := fs.Getxattr(handle, "user.comment", buf)
	if err != nil {
		t.Fatalf("Getxattr failed: %v", err)
	}
	if string(buf[:n]) != "This is a comment" {
		t.Errorf("Getxattr returned %q, want %q", string(buf[:n]), "This is a comment")
	}

	// Get size of extended attribute
	size, err := fs.Getxattr(handle, "user.comment", nil)
	if err != nil {
		t.Fatalf("Getxattr (size) failed: %v", err)
	}
	if size != 17 {
		t.Errorf("Xattr size = %d, want 17", size)
	}

	// Remove extended attribute
	err = fs.Removexattr(handle, "user.comment")
	if err != nil {
		t.Fatalf("Removexattr failed: %v", err)
	}

	// List should be empty now
	keys, err = fs.Listxattr(handle)
	if err != nil {
		t.Fatalf("Listxattr failed: %v", err)
	}
	if len(keys) != 0 {
		t.Errorf("Listxattr returned %d keys after removal, want 0", len(keys))
	}
}

func TestMemoryFS_SetAttr(t *testing.T) {
	fs := NewMemoryFS()

	// Create file
	err := fs.AddFile("/setattr_test.txt", []byte("test"), 0644)
	if err != nil {
		t.Fatalf("AddFile failed: %v", err)
	}

	handle, err := fs.Open("/setattr_test.txt", 0, 0)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer fs.Close(handle)

	// Get original attributes
	origAttrs, err := fs.GetAttr(handle)
	if err != nil {
		t.Fatalf("GetAttr failed: %v", err)
	}

	// Create new attributes with modified mode
	newAttrs := vfs.Attributes{}
	newAttrs.SetUnixMode(0755)

	// Set attributes
	updatedAttrs, err := fs.SetAttr(handle, &newAttrs)
	if err != nil {
		t.Fatalf("SetAttr failed: %v", err)
	}

	mode, ok := updatedAttrs.GetUnixMode()
	if !ok || mode != 0755 {
		t.Errorf("Mode after SetAttr = %o, want 0755", mode)
	}

	// Verify original timestamps are preserved
	origTime, _ := origAttrs.GetAccessTime()
	newTime, _ := updatedAttrs.GetAccessTime()
	if !origTime.Equal(newTime) {
		t.Error("Access time should be preserved when not explicitly set")
	}
}

func TestMemoryFS_StatFS(t *testing.T) {
	fs := NewMemoryFS()

	attrs, err := fs.StatFS(0)
	if err != nil {
		t.Fatalf("StatFS failed: %v", err)
	}
	if attrs == nil {
		t.Fatal("StatFS returned nil attributes")
	}

	blockSize, ok := attrs.GetBlockSize()
	if !ok || blockSize != 4096 {
		t.Errorf("Block size = %d, want 4096", blockSize)
	}

	blocks, ok := attrs.GetBlocks()
	if !ok || blocks == 0 {
		t.Error("Total blocks should be non-zero")
	}

	freeBlocks, ok := attrs.GetFreeBlocks()
	if !ok || freeBlocks == 0 {
		t.Error("Free blocks should be non-zero")
	}
}

func TestMemoryFS_ConcurrentAccess(t *testing.T) {
	fs := NewMemoryFS()

	// Create test file
	fs.AddFile("/concurrent.txt", []byte("initial"), 0644)

	// Open multiple handles concurrently
	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go func(id int) {
			handle, err := fs.Open("/concurrent.txt", 0, 0)
			if err != nil {
				t.Errorf("Goroutine %d: Open failed: %v", id, err)
				done <- false
				return
			}

			buf := make([]byte, 100)
			_, err = fs.Read(handle, buf, 0, 0)
			if err != nil {
				t.Errorf("Goroutine %d: Read failed: %v", id, err)
			}

			fs.Close(handle)
			done <- true
		}(i)
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}
}

func BenchmarkMemoryFS_Open(b *testing.B) {
	fs := NewMemoryFS()
	fs.AddFile("/bench.txt", []byte("benchmark"), 0644)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		handle, _ := fs.Open("/bench.txt", 0, 0)
		fs.Close(handle)
	}
}

func BenchmarkMemoryFS_Read(b *testing.B) {
	fs := NewMemoryFS()
	fs.AddFile("/bench.txt", []byte("benchmark test content for reading"), 0644)
	handle, _ := fs.Open("/bench.txt", 0, 0)
	defer fs.Close(handle)

	buf := make([]byte, 100)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		fs.Read(handle, buf, 0, 0)
	}
}

func BenchmarkMemoryFS_Write(b *testing.B) {
	fs := NewMemoryFS()
	fs.AddFile("/bench.txt", []byte(""), 0644)
	handle, _ := fs.Open("/bench.txt", 0, 0)
	defer fs.Close(handle)

	data := []byte("benchmark write data")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		fs.Write(handle, data, 0, 0)
	}
}
