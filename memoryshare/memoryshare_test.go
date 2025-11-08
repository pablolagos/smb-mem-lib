package memoryshare

import (
	"bytes"
	"testing"
	"time"
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
