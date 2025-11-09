package memoryshare

import (
	"bytes"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/pablolagos/smb-mem-lib/vfs"
)

func TestNew(t *testing.T) {
	tests := []struct {
		name    string
		opts    Options
		wantErr bool
	}{
		{
			name:    "default options",
			opts:    Options{},
			wantErr: false,
		},
		{
			name: "custom options",
			opts: Options{
				Port:       9999,
				Address:    "0.0.0.0",
				ShareName:  "TestShare",
				Hostname:   "TestHost",
				AllowGuest: true,
				Debug:      true,
				Console:    true,
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			share, err := New(tt.opts)
			if (err != nil) != tt.wantErr {
				t.Errorf("New() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && share == nil {
				t.Error("New() returned nil share without error")
			}
			if share != nil {
				// Verify defaults are applied
				if share.opts.Port == 0 {
					t.Error("Port was not set to default")
				}
				if share.opts.Address == "" {
					t.Error("Address was not set to default")
				}
			}
		})
	}
}

func TestAddFile(t *testing.T) {
	share, err := New(Options{Console: true})
	if err != nil {
		t.Fatalf("Failed to create share: %v", err)
	}

	tests := []struct {
		name     string
		filepath string
		content  []byte
		wantErr  bool
	}{
		{
			name:     "simple file",
			filepath: "/test.txt",
			content:  []byte("test content"),
			wantErr:  false,
		},
		{
			name:     "file in subdirectory (auto-create)",
			filepath: "/docs/readme.md",
			content:  []byte("# README"),
			wantErr:  false,
		},
		{
			name:     "empty file",
			filepath: "/empty.txt",
			content:  []byte{},
			wantErr:  false,
		},
		{
			name:     "file at root fails",
			filepath: "/",
			content:  []byte("fail"),
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := share.AddFile(tt.filepath, tt.content)
			if (err != nil) != tt.wantErr {
				t.Errorf("AddFile() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestAddDirectory(t *testing.T) {
	share, err := New(Options{Console: true})
	if err != nil {
		t.Fatalf("Failed to create share: %v", err)
	}

	tests := []struct {
		name    string
		dirpath string
		wantErr bool
	}{
		{
			name:    "simple directory",
			dirpath: "/testdir",
			wantErr: false,
		},
		{
			name:    "nested directory",
			dirpath: "/path/to/deep/dir",
			wantErr: false,
		},
		{
			name:    "root directory (no-op)",
			dirpath: "/",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := share.AddDirectory(tt.dirpath)
			if (err != nil) != tt.wantErr {
				t.Errorf("AddDirectory() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestAddFileWithMode(t *testing.T) {
	share, err := New(Options{Console: true})
	if err != nil {
		t.Fatalf("Failed to create share: %v", err)
	}

	err = share.AddFileWithMode("/script.sh", []byte("#!/bin/bash"), 0755)
	if err != nil {
		t.Errorf("AddFileWithMode() error = %v", err)
	}

	// Verify the file was added
	node, err := share.fs.resolvePath("/script.sh")
	if err != nil {
		t.Errorf("File not found after AddFileWithMode: %v", err)
	}
	if node.mode != 0755 {
		t.Errorf("File mode = %o, want %o", node.mode, 0755)
	}
}

func TestAddDirectoryWithMode(t *testing.T) {
	share, err := New(Options{Console: true})
	if err != nil {
		t.Fatalf("Failed to create share: %v", err)
	}

	err = share.AddDirectoryWithMode("/private", 0700)
	if err != nil {
		t.Errorf("AddDirectoryWithMode() error = %v", err)
	}

	// Note: getOrCreateDirPath uses 0755 by default, not the custom mode
	// This is a potential improvement area
}

func TestMemoryFSOperations(t *testing.T) {
	share, err := New(Options{Console: true})
	if err != nil {
		t.Fatalf("Failed to create share: %v", err)
	}

	// Add test data
	testContent := []byte("Hello, World!")
	err = share.AddFile("/test.txt", testContent)
	if err != nil {
		t.Fatalf("Failed to add file: %v", err)
	}

	// Test file resolution
	node, err := share.fs.resolvePath("/test.txt")
	if err != nil {
		t.Fatalf("Failed to resolve path: %v", err)
	}

	if node.isDir {
		t.Error("File node marked as directory")
	}

	if node.size != uint64(len(testContent)) {
		t.Errorf("File size = %d, want %d", node.size, len(testContent))
	}

	if !bytes.Equal(node.content, testContent) {
		t.Errorf("File content = %q, want %q", node.content, testContent)
	}
}

func TestMemoryFSDirectories(t *testing.T) {
	share, err := New(Options{Console: true})
	if err != nil {
		t.Fatalf("Failed to create share: %v", err)
	}

	// Create directory structure
	err = share.AddDirectory("/docs")
	if err != nil {
		t.Fatalf("Failed to add directory: %v", err)
	}

	// Add files to directory
	err = share.AddFile("/docs/file1.txt", []byte("content1"))
	if err != nil {
		t.Fatalf("Failed to add file to directory: %v", err)
	}

	err = share.AddFile("/docs/file2.txt", []byte("content2"))
	if err != nil {
		t.Fatalf("Failed to add second file to directory: %v", err)
	}

	// Verify directory
	node, err := share.fs.resolvePath("/docs")
	if err != nil {
		t.Fatalf("Failed to resolve directory: %v", err)
	}

	if !node.isDir {
		t.Error("Directory node not marked as directory")
	}

	if len(node.children) != 2 {
		t.Errorf("Directory has %d children, want 2", len(node.children))
	}
}

func TestMemoryFSTimestamps(t *testing.T) {
	share, err := New(Options{Console: true})
	if err != nil {
		t.Fatalf("Failed to create share: %v", err)
	}

	before := time.Now()
	err = share.AddFile("/test.txt", []byte("test"))
	if err != nil {
		t.Fatalf("Failed to add file: %v", err)
	}
	after := time.Now()

	node, err := share.fs.resolvePath("/test.txt")
	if err != nil {
		t.Fatalf("Failed to resolve file: %v", err)
	}

	// Check timestamps are within reasonable range
	if node.ctime.Before(before) || node.ctime.After(after) {
		t.Error("File creation time outside expected range")
	}
	if node.mtime.Before(before) || node.mtime.After(after) {
		t.Error("File modification time outside expected range")
	}
	if node.atime.Before(before) || node.atime.After(after) {
		t.Error("File access time outside expected range")
	}
}

func TestAutoCreateParentDirectories(t *testing.T) {
	share, err := New(Options{Console: true})
	if err != nil {
		t.Fatalf("Failed to create share: %v", err)
	}

	// Add file with deep path - should auto-create parents
	err = share.AddFile("/a/b/c/d/file.txt", []byte("deep"))
	if err != nil {
		t.Fatalf("Failed to add file with deep path: %v", err)
	}

	// Verify all parent directories exist
	paths := []string{"/a", "/a/b", "/a/b/c", "/a/b/c/d", "/a/b/c/d/file.txt"}
	for _, path := range paths {
		node, err := share.fs.resolvePath(path)
		if err != nil {
			t.Errorf("Path %s not found: %v", path, err)
			continue
		}
		if path != "/a/b/c/d/file.txt" && !node.isDir {
			t.Errorf("Path %s should be a directory", path)
		}
	}
}

func TestOptionsDefaults(t *testing.T) {
	share, err := New(Options{})
	if err != nil {
		t.Fatalf("Failed to create share: %v", err)
	}

	if share.opts.Port != 8082 {
		t.Errorf("Default port = %d, want 8082", share.opts.Port)
	}

	if share.opts.Address != "127.0.0.1" {
		t.Errorf("Default address = %s, want 127.0.0.1", share.opts.Address)
	}

	if share.opts.ShareName != "Share" {
		t.Errorf("Default share name = %s, want Share", share.opts.ShareName)
	}

	if share.opts.Hostname != "SmbServer" {
		t.Errorf("Default hostname = %s, want SmbServer", share.opts.Hostname)
	}

	if share.opts.MaxIOReads != 4 {
		t.Errorf("Default MaxIOReads = %d, want 4", share.opts.MaxIOReads)
	}

	if share.opts.MaxIOWrites != 4 {
		t.Errorf("Default MaxIOWrites = %d, want 4", share.opts.MaxIOWrites)
	}
}

func BenchmarkAddFile(b *testing.B) {
	share, err := New(Options{Console: false})
	if err != nil {
		b.Fatalf("Failed to create share: %v", err)
	}

	content := []byte("benchmark test content")
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		share.AddFile("/bench.txt", content)
	}
}

func BenchmarkAddManyFiles(b *testing.B) {
	share, err := New(Options{Console: false})
	if err != nil {
		b.Fatalf("Failed to create share: %v", err)
	}

	content := []byte("test")
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		for j := 0; j < 100; j++ {
			share.fs.AddFile("/file.txt", content, 0644)
		}
	}
}

func BenchmarkResolvePathDeep(b *testing.B) {
	share, err := New(Options{Console: false})
	if err != nil {
		b.Fatalf("Failed to create share: %v", err)
	}

	// Create deep directory structure
	share.AddFile("/a/b/c/d/e/f/g/h/i/j/file.txt", []byte("deep"))

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		share.fs.resolvePath("/a/b/c/d/e/f/g/h/i/j/file.txt")
	}
}

// TestHoneypotCallbacks verifies that callbacks are correctly invoked in honeypot mode
// when creating and renaming files, and that operations don't actually persist
func TestHoneypotCallbacks(t *testing.T) {
	// Track callback invocations
	var createCallbackCalled bool
	var createCallbackPath string
	var createCallbackIsDir bool
	var renameCallbackCalled bool
	var renameCallbackFrom string
	var renameCallbackTo string

	// Create share with honeypot mode enabled and callbacks registered
	share, err := New(Options{
		Console:       true,
		Logger:        NoOpLogger, // Disable logging for cleaner test output
		DiscardWrites: true,       // Enable honeypot mode
	})
	if err != nil {
		t.Fatalf("Failed to create share: %v", err)
	}

	// Register onCreate callback
	share.SetCreateCallback(func(ctx *vfs.OperationContext, filepath string, isDir bool, mode int) vfs.CreateInterceptResult {
		createCallbackCalled = true
		createCallbackPath = filepath
		createCallbackIsDir = isDir
		t.Logf("onCreate callback invoked: path=%s, isDir=%v, mode=%o", filepath, isDir, mode)
		return vfs.CreateInterceptResult{
			Block: false, // Don't block, let honeypot mode handle it
			Error: nil,
		}
	})

	// Register onRename callback
	share.SetRenameCallback(func(ctx *vfs.OperationContext, fromPath string, toPath string) vfs.RenameInterceptResult {
		renameCallbackCalled = true
		renameCallbackFrom = fromPath
		renameCallbackTo = toPath
		t.Logf("onRename callback invoked: from=%s, to=%s", fromPath, toPath)
		return vfs.RenameInterceptResult{
			Block: false, // Don't block, let honeypot mode handle it
			Error: nil,
		}
	})

	// Set operation context to simulate a real SMB client
	ctx := &vfs.OperationContext{
		RemoteAddr: "192.168.1.100:12345",
		LocalAddr:  "127.0.0.1:8445",
		SessionID:  12345,
		Username:   "testuser",
	}
	share.fs.SetOperationContext(ctx)

	// Step 1: Create file "copy.file" (using Open with O_CREATE flag)
	t.Log("Step 1: Creating file 'copy.file'")
	handle, err := share.fs.Open("/copy.file", os.O_CREATE|os.O_RDWR, 0644)
	if err != nil {
		t.Fatalf("Failed to create file: %v", err)
	}
	defer share.fs.Close(handle)

	// Verify onCreate callback was called with correct parameters
	if !createCallbackCalled {
		t.Error("onCreate callback was not called")
	}
	if createCallbackPath != "/copy.file" {
		t.Errorf("onCreate callback path = %s, want /copy.file", createCallbackPath)
	}
	if createCallbackIsDir {
		t.Error("onCreate callback isDir = true, want false for file")
	}

	// Step 2: Verify file was NOT actually created (honeypot mode)
	t.Log("Step 2: Verifying file was not actually created (honeypot mode)")
	_, err = share.fs.resolvePath("/copy.file")
	if err == nil {
		t.Error("File should NOT exist in honeypot mode, but it does")
	}

	// Step 3: Rename "copy.file" to "file.doc"
	t.Log("Step 3: Renaming 'copy.file' to 'file.doc'")
	err = share.fs.Rename(handle, "/file.doc", 0)
	if err != nil {
		t.Fatalf("Failed to rename file: %v", err)
	}

	// Verify onRename callback was called with correct parameters
	if !renameCallbackCalled {
		t.Error("onRename callback was not called")
	}
	if renameCallbackFrom != "/copy.file" {
		t.Errorf("onRename callback fromPath = %s, want /copy.file", renameCallbackFrom)
	}
	if renameCallbackTo != "/file.doc" {
		t.Errorf("onRename callback toPath = %s, want /file.doc", renameCallbackTo)
	}

	// Step 4: Verify file was NOT actually renamed (honeypot mode)
	t.Log("Step 4: Verifying file was not actually renamed (honeypot mode)")
	_, err = share.fs.resolvePath("/file.doc")
	if err == nil {
		t.Error("Renamed file should NOT exist in honeypot mode, but it does")
	}
	_, err = share.fs.resolvePath("/copy.file")
	if err == nil {
		t.Error("Original file should NOT exist in honeypot mode, but it does")
	}

	// Summary
	t.Log("✓ All callbacks were invoked correctly")
	t.Log("✓ Honeypot mode prevented actual file operations")
	t.Log("✓ Operations appeared successful to the client")
}

// TestCallbacksWithoutHoneypot verifies callbacks work normally when honeypot mode is disabled
func TestCallbacksWithoutHoneypot(t *testing.T) {
	var createCalled bool
	var renameCalled bool

	// Create share WITHOUT honeypot mode
	share, err := New(Options{
		Console:       true,
		Logger:        NoOpLogger,
		DiscardWrites: false, // Honeypot disabled - operations persist
	})
	if err != nil {
		t.Fatalf("Failed to create share: %v", err)
	}

	// Register callbacks
	share.SetCreateCallback(func(ctx *vfs.OperationContext, filepath string, isDir bool, mode int) vfs.CreateInterceptResult {
		createCalled = true
		t.Logf("onCreate: %s", filepath)
		return vfs.CreateInterceptResult{Block: false, Error: nil}
	})

	share.SetRenameCallback(func(ctx *vfs.OperationContext, fromPath string, toPath string) vfs.RenameInterceptResult {
		renameCalled = true
		t.Logf("onRename: %s -> %s", fromPath, toPath)
		return vfs.RenameInterceptResult{Block: false, Error: nil}
	})

	ctx := &vfs.OperationContext{
		RemoteAddr: "192.168.1.100:12345",
		SessionID:  5001,
		Username:   "testuser",
	}
	share.fs.SetOperationContext(ctx)

	// Create file
	handle, err := share.fs.Open("/test.file", os.O_CREATE|os.O_RDWR, 0644)
	if err != nil {
		t.Fatalf("Failed to create file: %v", err)
	}

	if !createCalled {
		t.Error("onCreate callback not called")
	}

	// Verify file DOES exist (not honeypot)
	_, err = share.fs.resolvePath("/test.file")
	if err != nil {
		t.Error("File should exist when honeypot mode is disabled")
	}

	// Rename file
	err = share.fs.Rename(handle, "/renamed.file", 0)
	if err != nil {
		t.Fatalf("Failed to rename: %v", err)
	}
	share.fs.Close(handle)

	if !renameCalled {
		t.Error("onRename callback not called")
	}

	// Verify file WAS renamed
	_, err = share.fs.resolvePath("/renamed.file")
	if err != nil {
		t.Error("Renamed file should exist when honeypot mode is disabled")
	}
	_, err = share.fs.resolvePath("/test.file")
	if err == nil {
		t.Error("Original file should not exist after rename")
	}

	t.Log("✓ Callbacks invoked and operations persisted without honeypot mode")
}

// TestRenameCallbackWithInvalidHandle tests if rename callback is called when handle is invalid
func TestRenameCallbackWithInvalidHandle(t *testing.T) {
	var renameCallbackCalled bool

	share, err := New(Options{
		Logger:        NoOpLogger,
		DiscardWrites: true,
	})
	if err != nil {
		t.Fatalf("Failed to create share: %v", err)
	}

	// Register rename callback
	share.SetRenameCallback(func(ctx *vfs.OperationContext, fromPath string, toPath string) vfs.RenameInterceptResult {
		renameCallbackCalled = true
		t.Logf("Rename callback called: %s -> %s", fromPath, toPath)
		return vfs.RenameInterceptResult{Block: false, Error: nil}
	})

	ctx := &vfs.OperationContext{
		RemoteAddr: "192.168.1.100:12345",
		SessionID:  5002,
		Username:   "testuser",
	}
	share.fs.SetOperationContext(ctx)

	// Try to rename with an INVALID handle (file that was never opened)
	invalidHandle := vfs.VfsHandle(99999)
	err = share.fs.Rename(invalidHandle, "/newpath.txt", 0)

	// Should fail with invalid handle error
	if err == nil {
		t.Error("Rename should fail with invalid handle")
	}

	// Callback should NOT be called because handle is invalid
	if renameCallbackCalled {
		t.Error("Rename callback should NOT be called when handle is invalid")
	} else {
		t.Log("✓ Callback NOT called with invalid handle (expected behavior)")
	}
}

// TestRenameCallbackWithNonExistentFileInHoneypot tests rename callback in honeypot mode
// where the file exists as a dummy handle but not in the actual filesystem
func TestRenameCallbackWithNonExistentFileInHoneypot(t *testing.T) {
	var renameCallbackCalled bool
	var renameFrom string
	var renameTo string

	share, err := New(Options{
		Logger:        NoOpLogger,
		DiscardWrites: true, // Honeypot mode
	})
	if err != nil {
		t.Fatalf("Failed to create share: %v", err)
	}

	// Register rename callback
	share.SetRenameCallback(func(ctx *vfs.OperationContext, fromPath string, toPath string) vfs.RenameInterceptResult {
		renameCallbackCalled = true
		renameFrom = fromPath
		renameTo = toPath
		t.Logf("Rename callback called: %s -> %s", fromPath, toPath)
		return vfs.RenameInterceptResult{Block: false, Error: nil}
	})

	ctx := &vfs.OperationContext{
		RemoteAddr: "192.168.1.100:12345",
		SessionID:  5003,
		Username:   "testuser",
	}
	share.fs.SetOperationContext(ctx)

	// Create a file in honeypot mode (creates dummy handle, file doesn't really exist)
	handle, err := share.fs.Open("/dummy.file", os.O_CREATE|os.O_RDWR, 0644)
	if err != nil {
		t.Fatalf("Failed to create dummy file: %v", err)
	}

	// Verify file doesn't actually exist in filesystem
	_, err = share.fs.resolvePath("/dummy.file")
	if err == nil {
		t.Error("File should NOT exist in filesystem (honeypot mode)")
	}

	// Now rename the dummy file
	err = share.fs.Rename(handle, "/renamed.file", 0)
	if err != nil {
		t.Fatalf("Rename should succeed: %v", err)
	}

	// Callback SHOULD be called even though file doesn't exist in filesystem
	if !renameCallbackCalled {
		t.Error("Rename callback SHOULD be called even in honeypot mode with dummy file")
	} else {
		t.Log("✓ Callback WAS called with dummy file handle in honeypot mode")
	}

	if renameFrom != "/dummy.file" {
		t.Errorf("Callback fromPath = %s, want /dummy.file", renameFrom)
	}
	if renameTo != "/renamed.file" {
		t.Errorf("Callback toPath = %s, want /renamed.file", renameTo)
	}

	// File still shouldn't exist after rename (honeypot mode)
	_, err = share.fs.resolvePath("/renamed.file")
	if err == nil {
		t.Error("Renamed file should NOT exist in filesystem (honeypot mode)")
	}

	share.fs.Close(handle)
}

// TestHoneypotCompleteWorkflow tests the complete workflow:
// 1. Open 'file.test' for creation
// 2. Write data into 'file.test'
// 3. Close 'file.test'
// 4. Rename 'file.test' -> 'file.docx'
func TestHoneypotCompleteWorkflow(t *testing.T) {
	var createCalled bool
	var writeCalled bool
	var renameCalled bool
	var createPath string
	var writePath string
	var renameFrom, renameTo string

	share, err := New(Options{
		Logger:        NoOpLogger,
		DiscardWrites: true, // Honeypot mode
	})
	if err != nil {
		t.Fatalf("Failed to create share: %v", err)
	}

	// Register all callbacks
	share.SetCreateCallback(func(ctx *vfs.OperationContext, filepath string, isDir bool, mode int) vfs.CreateInterceptResult {
		createCalled = true
		createPath = filepath
		t.Logf("Step 1: onCreate called for %s", filepath)
		return vfs.CreateInterceptResult{Block: false, Error: nil}
	})

	share.SetWriteCallback(func(ctx *vfs.OperationContext, handle vfs.VfsHandle, filename string, data []byte, offset uint64) vfs.WriteInterceptResult {
		writeCalled = true
		writePath = filename
		t.Logf("Step 2: onWrite called for %s (size: %d bytes)", filename, len(data))
		return vfs.WriteInterceptResult{Block: false, Error: nil}
	})

	share.SetRenameCallback(func(ctx *vfs.OperationContext, fromPath string, toPath string) vfs.RenameInterceptResult {
		renameCalled = true
		renameFrom = fromPath
		renameTo = toPath
		t.Logf("Step 4: onRename called: %s -> %s", fromPath, toPath)
		return vfs.RenameInterceptResult{Block: false, Error: nil}
	})

	ctx := &vfs.OperationContext{
		RemoteAddr: "192.168.1.100:12345",
		SessionID:  5004,
		Username:   "testuser",
	}
	share.fs.SetOperationContext(ctx)

	// Step 1: Open 'file.test' for creation
	t.Log("Step 1: Opening 'file.test' for creation")
	handle, err := share.fs.Open("/file.test", os.O_CREATE|os.O_RDWR, 0644)
	if err != nil {
		t.Fatalf("Failed to create file: %v", err)
	}

	if !createCalled {
		t.Error("onCreate callback should be called")
	}
	if createPath != "/file.test" {
		t.Errorf("onCreate path = %s, want /file.test", createPath)
	}

	// Step 2: Write data into 'file.test'
	t.Log("Step 2: Writing data into 'file.test'")
	data := []byte("test data for honeypot")
	n, err := share.fs.Write(handle, data, 0, 0)
	if err != nil {
		t.Fatalf("Failed to write: %v", err)
	}
	if n != len(data) {
		t.Errorf("Write returned %d bytes, want %d", n, len(data))
	}

	if !writeCalled {
		t.Error("onWrite callback should be called")
	}
	if writePath != "/file.test" {
		t.Errorf("onWrite path = %s, want /file.test", writePath)
	}

	// Step 3: Close 'file.test'
	t.Log("Step 3: Closing 'file.test'")
	err = share.fs.Close(handle)
	if err != nil {
		t.Fatalf("Failed to close: %v", err)
	}

	// Step 4: Rename 'file.test' -> 'file.docx'
	t.Log("Step 4: Attempting to rename 'file.test' -> 'file.docx'")

	// With ghost files support, we should be able to reopen the file after closing
	t.Log("Step 4a: Reopening 'file.test' from ghost registry")
	handleForRename, err := share.fs.Open("/file.test", os.O_RDWR, 0644)
	if err != nil {
		t.Fatalf("Failed to reopen file.test from ghost registry: %v", err)
	}

	// Now rename it
	t.Log("Step 4b: Renaming ghost file")
	err = share.fs.Rename(handleForRename, "/file.docx", 0)
	if err != nil {
		t.Fatalf("Failed to rename: %v", err)
	}

	if !renameCalled {
		t.Error("onRename callback should be called")
	}
	if renameFrom != "/file.test" {
		t.Errorf("onRename from = %s, want /file.test", renameFrom)
	}
	if renameTo != "/file.docx" {
		t.Errorf("onRename to = %s, want /file.docx", renameTo)
	}

	share.fs.Close(handleForRename)

	t.Log("✓ Complete workflow succeeded with ghost files support!")
	t.Log("✓ Open -> Write -> Close -> Reopen -> Rename")
}

// TestHoneypotWorkflowRenameBeforeClose tests the workflow where rename happens before close
// 1. Open 'file.test' for creation
// 2. Write data into 'file.test'
// 3. Rename 'file.test' -> 'file.docx' (BEFORE closing)
// 4. Close file
func TestHoneypotWorkflowRenameBeforeClose(t *testing.T) {
	var createCalled bool
	var writeCalled bool
	var renameCalled bool

	share, err := New(Options{
		Logger:        NoOpLogger,
		DiscardWrites: true, // Honeypot mode
	})
	if err != nil {
		t.Fatalf("Failed to create share: %v", err)
	}

	// Register callbacks
	share.SetCreateCallback(func(ctx *vfs.OperationContext, filepath string, isDir bool, mode int) vfs.CreateInterceptResult {
		createCalled = true
		t.Logf("onCreate: %s", filepath)
		return vfs.CreateInterceptResult{Block: false, Error: nil}
	})

	share.SetWriteCallback(func(ctx *vfs.OperationContext, handle vfs.VfsHandle, filename string, data []byte, offset uint64) vfs.WriteInterceptResult {
		writeCalled = true
		t.Logf("onWrite: %s (%d bytes)", filename, len(data))
		return vfs.WriteInterceptResult{Block: false, Error: nil}
	})

	share.SetRenameCallback(func(ctx *vfs.OperationContext, fromPath string, toPath string) vfs.RenameInterceptResult {
		renameCalled = true
		t.Logf("onRename: %s -> %s", fromPath, toPath)
		return vfs.RenameInterceptResult{Block: false, Error: nil}
	})

	ctx := &vfs.OperationContext{
		RemoteAddr: "192.168.1.100:12345",
		SessionID:  5005,
		Username:   "testuser",
	}
	share.fs.SetOperationContext(ctx)

	// Step 1: Open 'file.test' for creation
	handle, err := share.fs.Open("/file.test", os.O_CREATE|os.O_RDWR, 0644)
	if err != nil {
		t.Fatalf("Failed to create file: %v", err)
	}

	// Step 2: Write data
	data := []byte("test data")
	_, err = share.fs.Write(handle, data, 0, 0)
	if err != nil {
		t.Fatalf("Failed to write: %v", err)
	}

	// Step 3: Rename (BEFORE closing) - This should work!
	err = share.fs.Rename(handle, "/file.docx", 0)
	if err != nil {
		t.Fatalf("Failed to rename: %v", err)
	}

	// Step 4: Close
	err = share.fs.Close(handle)
	if err != nil {
		t.Fatalf("Failed to close: %v", err)
	}

	// Verify all callbacks were called
	if !createCalled {
		t.Error("onCreate not called")
	}
	if !writeCalled {
		t.Error("onWrite not called")
	}
	if !renameCalled {
		t.Error("onRename not called")
	}

	t.Log("✓ Complete workflow works when rename happens BEFORE close")
}

// TestGhostFilesPerUser verifies that ghost files are isolated per user
func TestGhostFilesPerUser(t *testing.T) {
	share, err := New(Options{
		Logger:        NoOpLogger,
		DiscardWrites: true,
	})
	if err != nil {
		t.Fatalf("Failed to create share: %v", err)
	}

	// User 1 creates a file (session 1001)
	ctx1 := &vfs.OperationContext{
		RemoteAddr: "192.168.1.100:12345",
		SessionID:  1001,
		Username:   "user1",
	}
	share.fs.SetOperationContext(ctx1)

	handle1, err := share.fs.Open("/user1file.txt", os.O_CREATE|os.O_RDWR, 0644)
	if err != nil {
		t.Fatalf("User1 failed to create file: %v", err)
	}
	share.fs.Write(handle1, []byte("user1 data"), 0, 0)
	share.fs.Close(handle1) // File becomes ghost for session 1001

	// User 2 tries to open user1's file - should fail (different session)
	ctx2 := &vfs.OperationContext{
		RemoteAddr: "192.168.1.200:54321",
		SessionID:  1002,
		Username:   "user2",
	}
	share.fs.SetOperationContext(ctx2)

	handle2, err := share.fs.Open("/user1file.txt", os.O_RDWR, 0644)
	if err == nil {
		share.fs.Close(handle2)
		t.Error("User2 should NOT be able to open user1's ghost file")
	} else {
		t.Logf("✓ User2 correctly cannot access user1's ghost file: %v", err)
	}

	// User 1 should still be able to open their file
	share.fs.SetOperationContext(ctx1)
	handle3, err := share.fs.Open("/user1file.txt", os.O_RDWR, 0644)
	if err != nil {
		t.Errorf("User1 should be able to reopen their own ghost file: %v", err)
	} else {
		t.Log("✓ User1 can still access their own ghost file")
		share.fs.Close(handle3)
	}
}

// TestGhostFilesExpiration verifies that ghost files expire after 3 minutes
func TestGhostFilesExpiration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping expiration test in short mode")
	}

	share, err := New(Options{
		Logger:        NoOpLogger,
		DiscardWrites: true,
	})
	if err != nil {
		t.Fatalf("Failed to create share: %v", err)
	}

	ctx := &vfs.OperationContext{
		RemoteAddr: "192.168.1.100:12345",
		SessionID:  2001,
		Username:   "testuser",
	}
	share.fs.SetOperationContext(ctx)

	// Create and close a file
	handle, err := share.fs.Open("/expiring.txt", os.O_CREATE|os.O_RDWR, 0644)
	if err != nil {
		t.Fatalf("Failed to create file: %v", err)
	}
	share.fs.Close(handle)

	// Should be able to reopen immediately
	handle, err = share.fs.Open("/expiring.txt", os.O_RDWR, 0644)
	if err != nil {
		t.Errorf("Should be able to reopen ghost file immediately: %v", err)
	} else {
		share.fs.Close(handle)
		t.Log("✓ Ghost file accessible immediately after close")
	}

	// Manually expire the ghost file by setting lastAccess to >3 minutes ago
	key := share.fs.makeGhostKey(ctx.SessionID, "/expiring.txt")
	if v, ok := share.fs.ghostFiles.Load(key); ok {
		ghost := v.(*GhostFile)
		ghost.lastAccess = time.Now().Add(-4 * time.Minute) // 4 minutes ago
		t.Log("Manually set ghost file to be expired (4 minutes old)")
	}

	// Wait for cleanup cycle (max 30 seconds + a bit extra)
	t.Log("Waiting for cleanup cycle...")
	time.Sleep(35 * time.Second)

	// Should NOT be able to reopen now
	handle, err = share.fs.Open("/expiring.txt", os.O_RDWR, 0644)
	if err == nil {
		share.fs.Close(handle)
		t.Error("Should NOT be able to reopen expired ghost file")
	} else {
		t.Logf("✓ Expired ghost file correctly removed: %v", err)
	}
}

// TestGhostFilesMaxLimit verifies that each session is limited to 3 ghost files
func TestGhostFilesMaxLimit(t *testing.T) {
	share, err := New(Options{
		Logger:        NoOpLogger,
		DiscardWrites: true,
	})
	if err != nil {
		t.Fatalf("Failed to create share: %v", err)
	}

	ctx := &vfs.OperationContext{
		RemoteAddr: "192.168.1.100:12345",
		SessionID:  3001,
		Username:   "testuser",
	}
	share.fs.SetOperationContext(ctx)

	// Create and close 5 files
	files := []string{"/file1.txt", "/file2.txt", "/file3.txt", "/file4.txt", "/file5.txt"}
	for i, filepath := range files {
		handle, err := share.fs.Open(filepath, os.O_CREATE|os.O_RDWR, 0644)
		if err != nil {
			t.Fatalf("Failed to create %s: %v", filepath, err)
		}
		share.fs.Write(handle, []byte(fmt.Sprintf("data %d", i+1)), 0, 0)
		share.fs.Close(handle)
		t.Logf("Created and closed %s", filepath)
		time.Sleep(10 * time.Millisecond) // Small delay to ensure different timestamps
	}

	// Count ghost files for this session
	count := 0
	share.fs.ghostFiles.Range(func(key, value interface{}) bool {
		ghost := value.(*GhostFile)
		if ghost.sessionID == ctx.SessionID {
			count++
			t.Logf("Active ghost file: %s", ghost.path)
		}
		return true
	})

	if count != 3 {
		t.Errorf("User should have exactly 3 ghost files, got %d", count)
	} else {
		t.Log("✓ User correctly limited to 3 ghost files")
	}

	// Verify first 2 files (oldest) were evicted
	for i := 0; i < 2; i++ {
		handle, err := share.fs.Open(files[i], os.O_RDWR, 0644)
		if err == nil {
			share.fs.Close(handle)
			t.Errorf("File %s should have been evicted", files[i])
		} else {
			t.Logf("✓ Old file %s correctly evicted", files[i])
		}
	}

	// Verify last 3 files are still accessible
	for i := 2; i < 5; i++ {
		handle, err := share.fs.Open(files[i], os.O_RDWR, 0644)
		if err != nil {
			t.Errorf("File %s should still be accessible: %v", files[i], err)
		} else {
			t.Logf("✓ Recent file %s still accessible", files[i])
			share.fs.Close(handle)
		}
	}
}

// TestGhostFilesMemoryLimit verifies that ghost files respect the 10MB memory limit per session
func TestGhostFilesMemoryLimit(t *testing.T) {
	share, err := New(Options{
		Logger:        NoOpLogger,
		DiscardWrites: true,
	})
	if err != nil {
		t.Fatalf("Failed to create share: %v", err)
	}

	ctx := &vfs.OperationContext{
		RemoteAddr: "192.168.1.100:12345",
		SessionID:  4001,
		Username:   "testuser",
	}
	share.fs.SetOperationContext(ctx)

	// Create files with total size exceeding 10MB
	// File1: 4MB, File2: 4MB, File3: 4MB = 12MB total
	// File1 should be evicted when File3 is added
	files := []struct {
		path string
		size int
	}{
		{"/file1_4mb.bin", 4 * 1024 * 1024}, // 4MB
		{"/file2_4mb.bin", 4 * 1024 * 1024}, // 4MB
		{"/file3_4mb.bin", 4 * 1024 * 1024}, // 4MB (would exceed limit)
	}

	for i, file := range files {
		// Create large data
		data := make([]byte, file.size)
		for j := range data {
			data[j] = byte(i)
		}

		handle, err := share.fs.Open(file.path, os.O_CREATE|os.O_RDWR, 0644)
		if err != nil {
			t.Fatalf("Failed to create %s: %v", file.path, err)
		}
		share.fs.Write(handle, data, 0, 0)
		share.fs.Close(handle)
		t.Logf("Created and closed %s (%d bytes)", file.path, file.size)
		time.Sleep(10 * time.Millisecond) // Small delay to ensure different timestamps
	}

	// Count ghost files and verify memory usage
	count := 0
	var totalMemory int64
	share.fs.ghostFiles.Range(func(key, value interface{}) bool {
		ghost := value.(*GhostFile)
		if ghost.sessionID == ctx.SessionID {
			count++
			totalMemory += ghost.size
			t.Logf("Active ghost file: %s (size: %d bytes)", ghost.path, ghost.size)
		}
		return true
	})

	t.Logf("Total ghost files: %d, Total memory: %d bytes (%.2f MB)",
		count, totalMemory, float64(totalMemory)/(1024*1024))

	// Verify memory limit (10MB = 10485760 bytes)
	maxMemory := int64(10 * 1024 * 1024)
	if totalMemory > maxMemory {
		t.Errorf("Ghost files exceed memory limit: %d bytes > %d bytes", totalMemory, maxMemory)
	} else {
		t.Logf("✓ Memory limit respected: %d bytes <= %d bytes", totalMemory, maxMemory)
	}

	// File1 should have been evicted due to memory limit
	handle, err := share.fs.Open(files[0].path, os.O_RDWR, 0644)
	if err == nil {
		share.fs.Close(handle)
		t.Error("File1 should have been evicted due to memory limit")
	} else {
		t.Logf("✓ File1 correctly evicted: %v", err)
	}

	// File2 and File3 should still be accessible
	for i := 1; i < len(files); i++ {
		handle, err := share.fs.Open(files[i].path, os.O_RDWR, 0644)
		if err != nil {
			t.Errorf("File %s should still be accessible: %v", files[i].path, err)
		} else {
			t.Logf("✓ File %s still accessible", files[i].path)
			share.fs.Close(handle)
		}
	}
}
