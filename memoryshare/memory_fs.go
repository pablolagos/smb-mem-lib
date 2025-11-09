package memoryshare

import (
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"io"
	"path"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/pablolagos/smb-mem-lib/vfs"
)

// MemoryNode represents a file or directory in memory
type MemoryNode struct {
	name      string
	isDir     bool
	content   []byte
	size      uint64
	children  map[string]*MemoryNode
	parent    *MemoryNode
	inode     uint64
	mode      uint32
	uid       uint32
	gid       uint32
	atime     time.Time
	mtime     time.Time
	ctime     time.Time
	btime     time.Time
	xattrs    map[string][]byte
	symlinkTo string
	isSymlink bool
	mu        sync.RWMutex
}

// OpenMemoryFile represents an open file handle
type OpenMemoryFile struct {
	node   *MemoryNode
	handle vfs.VfsHandle
	path   string
	isDir  bool
	dirPos int
}

// GhostFile represents a file that was closed but is kept in memory temporarily
// for honeypot mode to allow reopening after close
// Ghost files are per-session to prevent cross-session access
type GhostFile struct {
	node       *MemoryNode
	path       string
	sessionID  uint64 // SMB session ID of the session who created this
	size       int64  // Size of the file in bytes
	lastAccess time.Time
}

// GhostFileStats provides statistics about ghost file usage in honeypot mode
type GhostFileStats struct {
	// Total number of ghost files across all sessions
	TotalFiles int

	// Total number of active sessions with ghost files
	ActiveSessions int

	// Virtual memory usage (tracked file sizes, not real RAM)
	VirtualMemoryTotal int64
	VirtualMemoryLimit int64

	// Estimated real memory usage (actual RAM consumed)
	// Approximately 330 bytes per ghost file
	RealMemoryEstimate int64

	// Per-session statistics (map of sessionID to file count)
	SessionFileCounts map[uint64]int

	// Per-session virtual memory usage
	SessionVirtualMemory map[uint64]int64
}

// MemoryFS implements VFSFileSystem interface with in-memory storage
type MemoryFS struct {
	root      *MemoryNode
	openFiles sync.Map
	nextInode uint64
	mu        sync.RWMutex

	// Write callback support
	writeCallback  vfs.WriteCallback
	createCallback vfs.CreateCallback
	renameCallback vfs.RenameCallback
	callbackMu     sync.RWMutex

	// Operation context per goroutine
	// Key: goroutine ID, Value: *vfs.OperationContext
	contextMap sync.Map

	// Honeypot mode: discard all writes/modifications
	discardWrites bool

	// Ghost files registry for honeypot mode
	// Key: "sessionID:filepath" (e.g., "12345:/file.txt"), Value: *GhostFile
	// This ensures ghost files are isolated per session
	ghostFiles     sync.Map
	ghostCleanupCh chan struct{}

	// Memory usage tracking for ghost files per session
	// Key: sessionID (uint64), Value: total bytes (int64)
	ghostMemoryUsage sync.Map

	// Global memory tracking for all ghost files (across all sessions)
	ghostMemoryGlobal int64 // Use atomic operations for this

	// Configuration for ghost files
	maxGhostFilesPerSession  int
	maxGhostMemoryPerSession int64
	maxGhostMemoryGlobal     int64

	// Logger for filesystem operations
	logger SimpleLogger
	logMu  sync.RWMutex
}

// NewMemoryFS creates a new in-memory file system
func NewMemoryFS() *MemoryFS {
	now := time.Now()
	root := &MemoryNode{
		name:     "/",
		isDir:    true,
		children: make(map[string]*MemoryNode),
		inode:    1,
		mode:     0755,
		atime:    now,
		mtime:    now,
		ctime:    now,
		btime:    now,
		xattrs:   make(map[string][]byte),
	}

	fs := &MemoryFS{
		root:                     root,
		nextInode:                2,
		ghostCleanupCh:           make(chan struct{}),
		maxGhostFilesPerSession:  3,
		maxGhostMemoryPerSession: 10 * 1024 * 1024,   // 10MB default
		maxGhostMemoryGlobal:     1024 * 1024 * 1024, // 1GB default
		ghostMemoryGlobal:        0,
	}

	// Start ghost files cleanup goroutine
	go fs.cleanupGhostFiles()

	return fs
}

// AddFile adds a file with content to the memory filesystem
func (fs *MemoryFS) AddFile(filepath string, content []byte, mode uint32) error {
	fs.mu.Lock()
	defer fs.mu.Unlock()

	filepath = path.Clean(filepath)
	if filepath == "/" {
		return fmt.Errorf("cannot create file at root")
	}

	dir := path.Dir(filepath)
	filename := path.Base(filepath)

	// Create parent directories if needed
	parentNode, err := fs.getOrCreateDirPath(dir)
	if err != nil {
		return err
	}

	now := time.Now()
	node := &MemoryNode{
		name:    filename,
		isDir:   false,
		content: make([]byte, len(content)),
		size:    uint64(len(content)),
		parent:  parentNode,
		inode:   fs.nextInode,
		mode:    mode,
		atime:   now,
		mtime:   now,
		ctime:   now,
		btime:   now,
		xattrs:  make(map[string][]byte),
	}
	fs.nextInode++
	copy(node.content, content)

	parentNode.mu.Lock()
	parentNode.children[filename] = node
	parentNode.mu.Unlock()

	fs.logf("Added file: %s (size: %d bytes)", filepath, len(content))
	return nil
}

// AddDirectory adds a directory to the memory filesystem
func (fs *MemoryFS) AddDirectory(dirpath string, mode uint32) error {
	fs.mu.Lock()
	defer fs.mu.Unlock()

	dirpath = path.Clean(dirpath)
	if dirpath == "/" {
		return nil // root already exists
	}

	_, err := fs.getOrCreateDirPath(dirpath)
	return err
}

// getOrCreateDirPath creates all directories in the path if they don't exist
func (fs *MemoryFS) getOrCreateDirPath(dirpath string) (*MemoryNode, error) {
	dirpath = path.Clean(dirpath)
	if dirpath == "/" || dirpath == "." {
		return fs.root, nil
	}

	parts := strings.Split(strings.Trim(dirpath, "/"), "/")
	current := fs.root

	for _, part := range parts {
		if part == "" {
			continue
		}

		current.mu.Lock()
		child, exists := current.children[part]
		if !exists {
			now := time.Now()
			child = &MemoryNode{
				name:     part,
				isDir:    true,
				children: make(map[string]*MemoryNode),
				parent:   current,
				inode:    fs.nextInode,
				mode:     0755,
				atime:    now,
				mtime:    now,
				ctime:    now,
				btime:    now,
				xattrs:   make(map[string][]byte),
			}
			fs.nextInode++
			current.children[part] = child
		}
		current.mu.Unlock()

		if !child.isDir {
			return nil, fmt.Errorf("path component is not a directory: %s", part)
		}
		current = child
	}

	return current, nil
}

// resolvePath resolves a path to a node
func (fs *MemoryFS) resolvePath(filepath string) (*MemoryNode, error) {
	filepath = path.Clean(filepath)
	if filepath == "/" || filepath == "." {
		return fs.root, nil
	}

	parts := strings.Split(strings.Trim(filepath, "/"), "/")
	current := fs.root

	for _, part := range parts {
		if part == "" {
			continue
		}

		current.mu.RLock()
		child, exists := current.children[part]
		current.mu.RUnlock()

		if !exists {
			return nil, fmt.Errorf("path not found: %s", filepath)
		}
		current = child
	}

	return current, nil
}

func randint64Memory() uint64 {
	var b [8]byte
	rand.Read(b[:])
	return uint64(binary.LittleEndian.Uint64(b[:]))
}

// nodeToAttributesLocked converts a MemoryNode to vfs.Attributes without acquiring lock
// Caller must hold node.mu lock (read or write)
func nodeToAttributesLocked(node *MemoryNode) *vfs.Attributes {
	a := vfs.Attributes{}
	a.SetInodeNumber(node.inode)
	a.SetSizeBytes(node.size)
	a.SetDiskSizeBytes((node.size + 511) / 512 * 512) // Round up to 512-byte blocks
	a.SetUnixMode(node.mode)
	a.SetPermissions(vfs.NewPermissionsFromMode(node.mode & 0777))
	a.SetAccessTime(node.atime)
	a.SetLastDataModificationTime(node.mtime)
	a.SetBirthTime(node.btime)
	a.SetLastStatusChangeTime(node.ctime)
	a.SetUID(node.uid)
	a.SetGID(node.gid)

	if node.isSymlink {
		a.SetFileType(vfs.FileTypeSymlink)
	} else if node.isDir {
		a.SetFileType(vfs.FileTypeDirectory)
	} else {
		a.SetFileType(vfs.FileTypeRegularFile)
	}

	return &a
}

// nodeToAttributes converts a MemoryNode to vfs.Attributes
func nodeToAttributes(node *MemoryNode) *vfs.Attributes {
	node.mu.RLock()
	defer node.mu.RUnlock()
	return nodeToAttributesLocked(node)
}

// GetAttr implements vfs.VFSFileSystem
func (fs *MemoryFS) GetAttr(handle vfs.VfsHandle) (*vfs.Attributes, error) {
	if handle == 0 {
		return nodeToAttributes(fs.root), nil
	}

	v, ok := fs.openFiles.Load(handle)
	if !ok {
		return nil, fmt.Errorf("invalid handle")
	}
	open := v.(*OpenMemoryFile)

	return nodeToAttributes(open.node), nil
}

// SetAttr implements vfs.VFSFileSystem
func (fs *MemoryFS) SetAttr(handle vfs.VfsHandle, a *vfs.Attributes) (*vfs.Attributes, error) {
	if handle == 0 {
		return nil, fmt.Errorf("invalid handle")
	}

	v, ok := fs.openFiles.Load(handle)
	if !ok {
		return nil, fmt.Errorf("invalid handle")
	}
	open := v.(*OpenMemoryFile)
	node := open.node

	node.mu.Lock()
	defer node.mu.Unlock()

	if atime, ok := a.GetAccessTime(); ok {
		node.atime = atime
	}
	if mtime, ok := a.GetLastDataModificationTime(); ok {
		node.mtime = mtime
	}
	if ctime, ok := a.GetLastStatusChangeTime(); ok {
		node.ctime = ctime
	}
	if btime, ok := a.GetBirthTime(); ok {
		node.btime = btime
	}
	if mode, ok := a.GetUnixMode(); ok {
		node.mode = mode
	}

	return nodeToAttributesLocked(node), nil
}

// StatFS implements vfs.VFSFileSystem
func (fs *MemoryFS) StatFS(handle vfs.VfsHandle) (*vfs.FSAttributes, error) {
	a := vfs.FSAttributes{}
	// Report a virtual 1TB filesystem with plenty of space
	blockSize := uint64(4096)
	totalBlocks := uint64(1024 * 1024 * 1024 / 4096) // 1TB
	freeBlocks := uint64(900 * 1024 * 1024 / 4096)   // 900GB free

	a.SetAvailableBlocks(freeBlocks)
	a.SetBlockSize(blockSize)
	a.SetBlocks(totalBlocks)
	a.SetFiles(1000000)
	a.SetFreeBlocks(freeBlocks)
	a.SetFreeFiles(999999)
	a.SetIOSize(blockSize)

	return &a, nil
}

// FSync implements vfs.VFSFileSystem
func (fs *MemoryFS) FSync(handle vfs.VfsHandle) error {
	// Memory filesystem, nothing to sync
	return nil
}

// Flush implements vfs.VFSFileSystem
func (fs *MemoryFS) Flush(handle vfs.VfsHandle) error {
	// Memory filesystem, nothing to flush
	return nil
}

// Open implements vfs.VFSFileSystem
func (fs *MemoryFS) Open(filepath string, flags int, mode int) (vfs.VfsHandle, error) {
	fs.mu.RLock()
	node, err := fs.resolvePath(filepath)
	fs.mu.RUnlock()

	// Check if file doesn't exist and needs to be created
	isCreating := err != nil && (flags&0x40 != 0) // os.O_CREATE = 0x40

	// If file doesn't exist, check ghost files registry (honeypot mode)
	if err != nil && !isCreating {
		if ghostFile := fs.getGhostFile(filepath); ghostFile != nil {
			// Found in ghost files - reopen it
			fs.logf("Reopening ghost file: %s", filepath)
			ghostFile.lastAccess = time.Now() // Update access time

			h := vfs.VfsHandle(randint64Memory())
			fs.openFiles.Store(h, &OpenMemoryFile{
				node:   ghostFile.node,
				handle: h,
				path:   filepath,
				isDir:  ghostFile.node.isDir,
			})
			return h, nil
		}
	}

	if isCreating {
		// File doesn't exist, check if we have a create callback
		fs.callbackMu.RLock()
		callback := fs.createCallback
		fs.callbackMu.RUnlock()

		if callback != nil {
			ctx := fs.getOperationContext()
			// Call the callback before creating
			result := callback(ctx, filepath, false, mode)

			// Check if callback returned an error
			if result.Error != nil {
				fs.logf("[WARN] File creation rejected by callback - path: %s, remoteAddr: %s, error: %v",
					filepath, ctx.RemoteAddr, result.Error)
				return 0, result.Error
			}

			// Check if we should block the creation
			if result.Block {
				fs.logf("[INFO] File creation blocked by callback - path: %s, remoteAddr: %s",
					filepath, ctx.RemoteAddr)
				// Return a dummy handle
				h := vfs.VfsHandle(randint64Memory())
				now := time.Now()
				dummyNode := &MemoryNode{
					name:    path.Base(filepath),
					isDir:   false,
					content: []byte{},
					size:    0,
					inode:   999999,
					mode:    uint32(mode),
					atime:   now,
					mtime:   now,
					ctime:   now,
					btime:   now,
					xattrs:  make(map[string][]byte),
				}
				fs.openFiles.Store(h, &OpenMemoryFile{
					node:   dummyNode,
					handle: h,
					path:   filepath,
					isDir:  false,
				})
				return h, nil
			}
		}

		// Check if discard writes is enabled (honeypot mode)
		if fs.discardWrites {
			ctx := fs.getOperationContext()
			if ctx != nil {
				fs.logf("[INFO] File creation discarded (honeypot mode) - path: %s, remoteAddr: %s",
					filepath, ctx.RemoteAddr)
			}
			// Return a dummy handle
			h := vfs.VfsHandle(randint64Memory())
			now := time.Now()
			dummyNode := &MemoryNode{
				name:    path.Base(filepath),
				isDir:   false,
				content: []byte{},
				size:    0,
				inode:   999999,
				mode:    uint32(mode),
				atime:   now,
				mtime:   now,
				ctime:   now,
				btime:   now,
				xattrs:  make(map[string][]byte),
			}
			fs.openFiles.Store(h, &OpenMemoryFile{
				node:   dummyNode,
				handle: h,
				path:   filepath,
				isDir:  false,
			})
			return h, nil
		}

		// Create the file
		if err := fs.AddFile(filepath, []byte{}, uint32(mode)); err != nil {
			return 0, err
		}
		// Resolve the newly created file
		fs.mu.RLock()
		node, err = fs.resolvePath(filepath)
		fs.mu.RUnlock()
		if err != nil {
			return 0, err
		}
	} else if err != nil {
		return 0, err
	}

	h := vfs.VfsHandle(randint64Memory())
	fs.openFiles.Store(h, &OpenMemoryFile{
		node:   node,
		handle: h,
		path:   filepath,
		isDir:  node.isDir,
	})

	fs.logf("Opened file: %s (handle: %d)", filepath, h)
	return h, nil
}

// Close implements vfs.VFSFileSystem
func (fs *MemoryFS) Close(handle vfs.VfsHandle) error {
	v, ok := fs.openFiles.Load(handle)
	if !ok {
		return fmt.Errorf("invalid handle")
	}

	open := v.(*OpenMemoryFile)

	// In honeypot mode, save as ghost file for 3 minutes
	if fs.discardWrites {
		// Check if file is already a ghost - if so, just update access time
		if ghost := fs.getGhostFile(open.path); ghost != nil {
			ghost.lastAccess = time.Now()
			fs.logf("Closed handle %d, updated ghost file access time: %s", handle, open.path)
		} else {
			fs.addGhostFile(open.path, open.node)
			fs.logf("Closed handle %d, saved as ghost file: %s", handle, open.path)
		}
	} else {
		fs.logf("Closed handle: %d", handle)
	}

	fs.openFiles.Delete(handle)
	return nil
}

// Lookup implements vfs.VFSFileSystem
func (fs *MemoryFS) Lookup(handle vfs.VfsHandle, name string) (*vfs.Attributes, error) {
	var basePath string

	if handle == 0 {
		basePath = "/"
	} else {
		v, ok := fs.openFiles.Load(handle)
		if !ok {
			return nil, fmt.Errorf("invalid handle")
		}
		open := v.(*OpenMemoryFile)
		basePath = open.path
	}

	fullPath := path.Join(basePath, name)
	fs.mu.RLock()
	node, err := fs.resolvePath(fullPath)
	fs.mu.RUnlock()

	if err != nil {
		return nil, err
	}

	return nodeToAttributes(node), nil
}

// Mkdir implements vfs.VFSFileSystem
func (fs *MemoryFS) Mkdir(filepath string, mode int) (*vfs.Attributes, error) {
	// Check if we have a create callback
	fs.callbackMu.RLock()
	callback := fs.createCallback
	fs.callbackMu.RUnlock()

	if callback != nil {
		ctx := fs.getOperationContext()
		// Call the callback before creating
		result := callback(ctx, filepath, true, mode)

		// Check if callback returned an error
		if result.Error != nil {
			fs.logf("[WARN] Mkdir operation rejected by callback - path: %s, remoteAddr: %s, error: %v",
				filepath, ctx.RemoteAddr, result.Error)
			return nil, result.Error
		}

		// Check if we should block the creation
		if result.Block {
			fs.logf("[INFO] Mkdir operation blocked by callback - path: %s, remoteAddr: %s",
				filepath, ctx.RemoteAddr)
			// Return dummy attributes to make it look successful
			now := time.Now()
			attrs := &vfs.Attributes{}
			attrs.SetFileType(vfs.FileTypeDirectory)
			attrs.SetInodeNumber(999999)
			attrs.SetUnixMode(uint32(mode))
			attrs.SetAccessTime(now)
			attrs.SetLastDataModificationTime(now)
			attrs.SetBirthTime(now)
			attrs.SetLastStatusChangeTime(now)
			return attrs, nil
		}
	}

	// Check if discard writes is enabled (honeypot mode)
	if fs.discardWrites {
		ctx := fs.getOperationContext()
		if ctx != nil {
			fs.logf("[INFO] Mkdir operation discarded (honeypot mode) - path: %s, remoteAddr: %s",
				filepath, ctx.RemoteAddr)
		}
		// Return dummy attributes
		now := time.Now()
		attrs := &vfs.Attributes{}
		attrs.SetFileType(vfs.FileTypeDirectory)
		attrs.SetInodeNumber(999999)
		attrs.SetUnixMode(uint32(mode))
		attrs.SetAccessTime(now)
		attrs.SetLastDataModificationTime(now)
		attrs.SetBirthTime(now)
		attrs.SetLastStatusChangeTime(now)
		return attrs, nil
	}

	fs.mu.Lock()
	defer fs.mu.Unlock()

	filepath = path.Clean(filepath)
	dir := path.Dir(filepath)
	dirname := path.Base(filepath)

	parentNode, err := fs.resolvePath(dir)
	if err != nil {
		return nil, err
	}

	parentNode.mu.Lock()
	defer parentNode.mu.Unlock()

	if _, exists := parentNode.children[dirname]; exists {
		return nil, fmt.Errorf("directory already exists")
	}

	now := time.Now()
	node := &MemoryNode{
		name:     dirname,
		isDir:    true,
		children: make(map[string]*MemoryNode),
		parent:   parentNode,
		inode:    fs.nextInode,
		mode:     uint32(mode),
		atime:    now,
		mtime:    now,
		ctime:    now,
		btime:    now,
		xattrs:   make(map[string][]byte),
	}
	fs.nextInode++

	parentNode.children[dirname] = node
	return nodeToAttributes(node), nil
}

// Read implements vfs.VFSFileSystem
func (fs *MemoryFS) Read(handle vfs.VfsHandle, buf []byte, offset uint64, flags int) (int, error) {
	v, ok := fs.openFiles.Load(handle)
	if !ok {
		return 0, fmt.Errorf("invalid handle")
	}
	open := v.(*OpenMemoryFile)
	node := open.node

	node.mu.RLock()
	defer node.mu.RUnlock()

	if node.isDir {
		return 0, fmt.Errorf("cannot read directory")
	}

	if offset >= node.size {
		return 0, io.EOF
	}

	n := copy(buf, node.content[offset:])
	if n == 0 {
		return 0, io.EOF
	}

	return n, nil
}

// Write implements vfs.VFSFileSystem
func (fs *MemoryFS) Write(handle vfs.VfsHandle, buf []byte, offset uint64, flags int) (int, error) {
	v, ok := fs.openFiles.Load(handle)
	if !ok {
		return 0, fmt.Errorf("invalid handle")
	}
	open := v.(*OpenMemoryFile)
	node := open.node

	// Check if we have a write callback (call it first, even in discard mode)
	fs.callbackMu.RLock()
	callback := fs.writeCallback
	fs.callbackMu.RUnlock()

	if callback != nil {
		ctx := fs.getOperationContext()
		// Make a copy of the data to pass to the callback
		dataCopy := make([]byte, len(buf))
		copy(dataCopy, buf)

		// Call the callback before writing
		result := callback(ctx, handle, open.path, dataCopy, offset)

		// Check if callback returned an error
		if result.Error != nil {
			fs.logf("[WARN] Write operation rejected by callback - path: %s, remoteAddr: %s, error: %v",
				open.path, ctx.RemoteAddr, result.Error)
			return 0, result.Error
		}

		// Check if we should block the write
		if result.Block {
			fs.logf("[INFO] Write operation blocked by callback - path: %s, remoteAddr: %s, size: %d, offset: %d",
				open.path, ctx.RemoteAddr, len(buf), offset)
			return len(buf), nil // Return success but don't actually write
		}
	}

	// Check if discard writes is enabled (honeypot mode)
	if fs.discardWrites {
		ctx := fs.getOperationContext()
		if ctx != nil {
			fs.logf("[INFO] Write operation discarded (honeypot mode) - path: %s, remoteAddr: %s, size: %d, offset: %d",
				open.path, ctx.RemoteAddr, len(buf), offset)
		}

		// Update node size even in honeypot mode for accurate ghost file memory tracking
		// but don't allocate actual content
		node.mu.Lock()
		endOffset := offset + uint64(len(buf))
		if endOffset > node.size {
			node.size = endOffset
		}
		node.mtime = time.Now()
		node.mu.Unlock()

		return len(buf), nil // Return success but don't write actual content
	}

	node.mu.Lock()
	defer node.mu.Unlock()

	if node.isDir {
		return 0, fmt.Errorf("cannot write to directory")
	}

	endOffset := offset + uint64(len(buf))
	if endOffset > node.size {
		// Expand content
		newContent := make([]byte, endOffset)
		copy(newContent, node.content)
		node.content = newContent
		node.size = endOffset
	}

	n := copy(node.content[offset:], buf)
	node.mtime = time.Now()

	return n, nil
}

// OpenDir implements vfs.VFSFileSystem
func (fs *MemoryFS) OpenDir(filepath string) (vfs.VfsHandle, error) {
	fs.mu.RLock()
	node, err := fs.resolvePath(filepath)
	fs.mu.RUnlock()

	if err != nil {
		return 0, err
	}

	if !node.isDir {
		return 0, fmt.Errorf("not a directory")
	}

	h := vfs.VfsHandle(randint64Memory())
	fs.openFiles.Store(h, &OpenMemoryFile{
		node:   node,
		handle: h,
		path:   filepath,
		isDir:  true,
	})

	fs.logf("Opened directory: %s (handle: %d)", filepath, h)
	return h, nil
}

// ReadDir implements vfs.VFSFileSystem
func (fs *MemoryFS) ReadDir(handle vfs.VfsHandle, pos int, maxEntries int) ([]vfs.DirInfo, error) {
	if handle == 0 {
		return nil, fmt.Errorf("invalid handle")
	}

	v, ok := fs.openFiles.Load(handle)
	if !ok {
		return nil, fmt.Errorf("invalid handle")
	}
	open := v.(*OpenMemoryFile)
	node := open.node

	if !node.isDir {
		return nil, fmt.Errorf("not a directory")
	}

	// If already read once, return EOF (prevent infinite loops)
	if open.dirPos > 0 {
		return nil, io.EOF
	}

	var results []vfs.DirInfo

	// Add . and .. entries
	attrs := nodeToAttributes(node)
	results = append(results,
		vfs.DirInfo{Name: ".", Attributes: *attrs},
		vfs.DirInfo{Name: "..", Attributes: *attrs},
	)

	// Add all directory entries
	node.mu.RLock()
	for name, child := range node.children {
		if maxEntries > 0 && len(results) >= maxEntries {
			break
		}
		attrs := nodeToAttributes(child)
		results = append(results, vfs.DirInfo{
			Name:       name,
			Attributes: *attrs,
		})
	}
	node.mu.RUnlock()

	// Mark as read
	open.dirPos = 1

	return results, nil
}

// Readlink implements vfs.VFSFileSystem
func (fs *MemoryFS) Readlink(handle vfs.VfsHandle) (string, error) {
	v, ok := fs.openFiles.Load(handle)
	if !ok {
		return "", fmt.Errorf("invalid handle")
	}
	open := v.(*OpenMemoryFile)
	node := open.node

	node.mu.RLock()
	defer node.mu.RUnlock()

	if !node.isSymlink {
		return "", fmt.Errorf("not a symlink")
	}

	return node.symlinkTo, nil
}

// Unlink implements vfs.VFSFileSystem
func (fs *MemoryFS) Unlink(handle vfs.VfsHandle) error {
	// Check if discard writes is enabled (honeypot mode)
	if fs.discardWrites {
		ctx := fs.getOperationContext()
		if ctx != nil {
			v, ok := fs.openFiles.Load(handle)
			if ok {
				open := v.(*OpenMemoryFile)
				fs.logf("[INFO] Unlink operation discarded (honeypot mode) - path: %s, remoteAddr: %s",
					open.path, ctx.RemoteAddr)
			}
		}
		return nil // Return success but don't delete
	}

	v, ok := fs.openFiles.Load(handle)
	if !ok {
		return fmt.Errorf("invalid handle")
	}
	open := v.(*OpenMemoryFile)
	node := open.node

	if node.parent == nil {
		return fmt.Errorf("cannot delete root")
	}

	node.parent.mu.Lock()
	defer node.parent.mu.Unlock()

	delete(node.parent.children, node.name)
	return nil
}

// Truncate implements vfs.VFSFileSystem
func (fs *MemoryFS) Truncate(handle vfs.VfsHandle, size uint64) error {
	// Check if discard writes is enabled (honeypot mode)
	if fs.discardWrites {
		ctx := fs.getOperationContext()
		if ctx != nil {
			v, ok := fs.openFiles.Load(handle)
			if ok {
				open := v.(*OpenMemoryFile)
				fs.logf("[INFO] Truncate operation discarded (honeypot mode) - path: %s, size: %d, remoteAddr: %s",
					open.path, size, ctx.RemoteAddr)
			}
		}
		return nil // Return success but don't truncate
	}

	v, ok := fs.openFiles.Load(handle)
	if !ok {
		return fmt.Errorf("invalid handle")
	}
	open := v.(*OpenMemoryFile)
	node := open.node

	node.mu.Lock()
	defer node.mu.Unlock()

	if node.isDir {
		return fmt.Errorf("cannot truncate directory")
	}

	if size < node.size {
		node.content = node.content[:size]
	} else if size > node.size {
		newContent := make([]byte, size)
		copy(newContent, node.content)
		node.content = newContent
	}
	node.size = size
	node.mtime = time.Now()

	return nil
}

// Rename implements vfs.VFSFileSystem
func (fs *MemoryFS) Rename(fromHandle vfs.VfsHandle, toPath string, flags int) error {
	v, ok := fs.openFiles.Load(fromHandle)
	if !ok {
		return fmt.Errorf("invalid handle")
	}
	open := v.(*OpenMemoryFile)

	// Check if we have a rename callback
	fs.callbackMu.RLock()
	callback := fs.renameCallback
	fs.callbackMu.RUnlock()

	if callback != nil {
		ctx := fs.getOperationContext()
		// Call the callback before renaming
		result := callback(ctx, open.path, toPath)

		// Check if callback returned an error
		if result.Error != nil {
			fs.logf("[WARN] Rename operation rejected by callback - from: %s, to: %s, remoteAddr: %s, error: %v",
				open.path, toPath, ctx.RemoteAddr, result.Error)
			return result.Error
		}

		// Check if we should block the rename
		if result.Block {
			fs.logf("[INFO] Rename operation blocked by callback - from: %s, to: %s, remoteAddr: %s",
				open.path, toPath, ctx.RemoteAddr)
			return nil // Return success but don't rename
		}
	}

	// Check if discard writes is enabled (honeypot mode)
	if fs.discardWrites {
		ctx := fs.getOperationContext()
		if ctx != nil {
			fs.logf("[INFO] Rename operation discarded (honeypot mode) - from: %s, to: %s, remoteAddr: %s",
				open.path, toPath, ctx.RemoteAddr)
		}
		return nil // Return success but don't rename
	}

	node := open.node

	fs.mu.Lock()
	defer fs.mu.Unlock()

	toPath = path.Clean(toPath)
	toDir := path.Dir(toPath)
	toName := path.Base(toPath)

	// Get destination parent directory
	toParentNode, err := fs.resolvePath(toDir)
	if err != nil {
		return err
	}

	// Check if destination exists
	toParentNode.mu.Lock()
	defer toParentNode.mu.Unlock()

	if _, exists := toParentNode.children[toName]; exists && flags == 0 {
		return fmt.Errorf("destination already exists")
	}

	// Remove from old parent
	if node.parent != nil {
		// Only lock if it's a different parent to avoid deadlock
		if node.parent != toParentNode {
			node.parent.mu.Lock()
			delete(node.parent.children, node.name)
			node.parent.mu.Unlock()
		} else {
			// Same parent, already locked
			delete(node.parent.children, node.name)
		}
	}

	// Add to new parent
	node.name = toName
	node.parent = toParentNode
	toParentNode.children[toName] = node

	return nil
}

// Symlink implements vfs.VFSFileSystem
func (fs *MemoryFS) Symlink(targetHandle vfs.VfsHandle, source string, flag int) (*vfs.Attributes, error) {
	v, ok := fs.openFiles.Load(targetHandle)
	if !ok {
		return nil, fmt.Errorf("invalid handle")
	}
	open := v.(*OpenMemoryFile)
	node := open.node

	node.mu.Lock()
	defer node.mu.Unlock()

	node.isSymlink = true
	node.symlinkTo = source

	return nodeToAttributes(node), nil
}

// Link implements vfs.VFSFileSystem
func (fs *MemoryFS) Link(oldNode vfs.VfsNode, newNode vfs.VfsNode, name string) (*vfs.Attributes, error) {
	return nil, fmt.Errorf("hard links not supported")
}

// Listxattr implements vfs.VFSFileSystem
func (fs *MemoryFS) Listxattr(handle vfs.VfsHandle) ([]string, error) {
	v, ok := fs.openFiles.Load(handle)
	if !ok {
		return nil, fmt.Errorf("invalid handle")
	}
	open := v.(*OpenMemoryFile)
	node := open.node

	node.mu.RLock()
	defer node.mu.RUnlock()

	var keys []string
	for k := range node.xattrs {
		keys = append(keys, k)
	}

	return keys, nil
}

// Getxattr implements vfs.VFSFileSystem
func (fs *MemoryFS) Getxattr(handle vfs.VfsHandle, key string, val []byte) (int, error) {
	v, ok := fs.openFiles.Load(handle)
	if !ok {
		return 0, fmt.Errorf("invalid handle")
	}
	open := v.(*OpenMemoryFile)
	node := open.node

	node.mu.RLock()
	defer node.mu.RUnlock()

	data, exists := node.xattrs[key]
	if !exists {
		return 0, fmt.Errorf("attribute not found")
	}

	if val == nil {
		return len(data), nil
	}

	n := copy(val, data)
	return n, nil
}

// Setxattr implements vfs.VFSFileSystem
func (fs *MemoryFS) Setxattr(handle vfs.VfsHandle, key string, val []byte) error {
	// Check if discard writes is enabled (honeypot mode)
	if fs.discardWrites {
		ctx := fs.getOperationContext()
		if ctx != nil {
			v, ok := fs.openFiles.Load(handle)
			if ok {
				open := v.(*OpenMemoryFile)
				fs.logf("[DEBUG] Setxattr operation discarded (honeypot mode) - path: %s, key: %s, remoteAddr: %s",
					open.path, key, ctx.RemoteAddr)
			}
		}
		return nil // Return success but don't set
	}

	v, ok := fs.openFiles.Load(handle)
	if !ok {
		return fmt.Errorf("invalid handle")
	}
	open := v.(*OpenMemoryFile)
	node := open.node

	node.mu.Lock()
	defer node.mu.Unlock()

	if val == nil {
		node.xattrs[key] = []byte{}
	} else {
		node.xattrs[key] = make([]byte, len(val))
		copy(node.xattrs[key], val)
	}

	return nil
}

// Removexattr implements vfs.VFSFileSystem
func (fs *MemoryFS) Removexattr(handle vfs.VfsHandle, key string) error {
	v, ok := fs.openFiles.Load(handle)
	if !ok {
		return fmt.Errorf("invalid handle")
	}
	open := v.(*OpenMemoryFile)
	node := open.node

	node.mu.Lock()
	defer node.mu.Unlock()

	delete(node.xattrs, key)
	return nil
}

// getGoroutineID returns the current goroutine ID
func getGoroutineID() uint64 {
	var buf [64]byte
	n := runtime.Stack(buf[:], false)
	// Format is "goroutine 123 [running]:"
	// Extract the number after "goroutine "
	idField := strings.Fields(string(buf[:n]))[1]
	id, _ := strconv.ParseUint(idField, 10, 64)
	return id
}

// SetOperationContext implements vfs.VFSWithContext
func (fs *MemoryFS) SetOperationContext(ctx *vfs.OperationContext) {
	if ctx == nil {
		fs.contextMap.Delete(getGoroutineID())
	} else {
		fs.contextMap.Store(getGoroutineID(), ctx)
	}
}

// SetWriteCallback implements vfs.VFSWithContext
func (fs *MemoryFS) SetWriteCallback(callback vfs.WriteCallback) {
	fs.callbackMu.Lock()
	defer fs.callbackMu.Unlock()
	fs.writeCallback = callback
}

// SetCreateCallback implements vfs.VFSWithContext
func (fs *MemoryFS) SetCreateCallback(callback vfs.CreateCallback) {
	fs.callbackMu.Lock()
	defer fs.callbackMu.Unlock()
	fs.createCallback = callback
}

// SetRenameCallback implements vfs.VFSWithContext
func (fs *MemoryFS) SetRenameCallback(callback vfs.RenameCallback) {
	fs.callbackMu.Lock()
	defer fs.callbackMu.Unlock()
	fs.renameCallback = callback
}

// getOperationContext retrieves the context for the current goroutine
func (fs *MemoryFS) getOperationContext() *vfs.OperationContext {
	if v, ok := fs.contextMap.Load(getGoroutineID()); ok {
		return v.(*vfs.OperationContext)
	}
	return nil
}

// SetDiscardWrites enables or disables honeypot mode where all write operations
// are silently discarded without returning errors to the client.
func (fs *MemoryFS) SetDiscardWrites(discard bool) {
	fs.discardWrites = discard
}

// SetGhostFileLimits configures the limits for ghost files in honeypot mode
func (fs *MemoryFS) SetGhostFileLimits(maxFilesPerSession int, maxMemoryPerSession, maxMemoryGlobal int64) {
	fs.maxGhostFilesPerSession = maxFilesPerSession
	fs.maxGhostMemoryPerSession = maxMemoryPerSession
	fs.maxGhostMemoryGlobal = maxMemoryGlobal
}

// SetLogger sets the logger for the filesystem
func (fs *MemoryFS) SetLogger(logger SimpleLogger) {
	fs.logMu.Lock()
	defer fs.logMu.Unlock()
	fs.logger = logger
}

// logf logs a formatted message if a logger is configured
func (fs *MemoryFS) logf(format string, args ...interface{}) {
	fs.logMu.RLock()
	logger := fs.logger
	fs.logMu.RUnlock()

	if logger != nil {
		logger.Printf(format, args...)
	}
}

// makeGhostKey creates a unique key for ghost files registry combining session ID and path
func (fs *MemoryFS) makeGhostKey(sessionID uint64, path string) string {
	return fmt.Sprintf("%d:%s", sessionID, path)
}

// addGhostFile adds a file to the ghost files registry for honeypot mode
// Enforces per-session limits and global memory limits using LRU eviction
func (fs *MemoryFS) addGhostFile(filepath string, node *MemoryNode) {
	ctx := fs.getOperationContext()
	if ctx == nil {
		return
	}

	// Calculate file size
	fileSize := int64(node.size)

	// Get current memory usage for this session
	var currentMemory int64
	if v, ok := fs.ghostMemoryUsage.Load(ctx.SessionID); ok {
		currentMemory = v.(int64)
	}

	// Count existing ghost files for this session
	sessionGhostFiles := make([]*GhostFile, 0)
	fs.ghostFiles.Range(func(key, value interface{}) bool {
		ghost := value.(*GhostFile)
		if ghost.sessionID == ctx.SessionID {
			sessionGhostFiles = append(sessionGhostFiles, ghost)
		}
		return true
	})

	// STEP 1: Evict files from THIS SESSION if we exceed per-session limits
	for len(sessionGhostFiles) >= fs.maxGhostFilesPerSession ||
		(currentMemory+fileSize > fs.maxGhostMemoryPerSession && len(sessionGhostFiles) > 0) {

		// Find oldest (least recently accessed) in this session
		var oldest *GhostFile
		for _, g := range sessionGhostFiles {
			if oldest == nil || g.lastAccess.Before(oldest.lastAccess) {
				oldest = g
			}
		}

		if oldest != nil {
			fs.removeGhostFile(oldest.sessionID, oldest.path)
			currentMemory -= oldest.size
			fs.logf("Ghost file evicted (session limit): %s (session: %d, freed: %d bytes)",
				oldest.path, oldest.sessionID, oldest.size)

			// Remove from our working list
			for i, g := range sessionGhostFiles {
				if g == oldest {
					sessionGhostFiles = append(sessionGhostFiles[:i], sessionGhostFiles[i+1:]...)
					break
				}
			}
		} else {
			break
		}
	}

	// STEP 2: Check global memory limit and evict across ALL sessions if needed
	globalMemory := atomic.LoadInt64(&fs.ghostMemoryGlobal)
	for globalMemory+fileSize > fs.maxGhostMemoryGlobal && globalMemory > 0 {
		// Collect all ghost files across all sessions
		allGhostFiles := make([]*GhostFile, 0)
		fs.ghostFiles.Range(func(key, value interface{}) bool {
			ghost := value.(*GhostFile)
			allGhostFiles = append(allGhostFiles, ghost)
			return true
		})

		if len(allGhostFiles) == 0 {
			break
		}

		// Find globally oldest file
		var oldest *GhostFile
		for _, g := range allGhostFiles {
			if oldest == nil || g.lastAccess.Before(oldest.lastAccess) {
				oldest = g
			}
		}

		if oldest != nil {
			fs.removeGhostFile(oldest.sessionID, oldest.path)
			fs.logf("Ghost file evicted (global limit): %s (session: %d, freed: %d bytes, global: %d/%d bytes)",
				oldest.path, oldest.sessionID, oldest.size,
				atomic.LoadInt64(&fs.ghostMemoryGlobal), fs.maxGhostMemoryGlobal)

			// Update session memory if it's from current session
			if oldest.sessionID == ctx.SessionID {
				currentMemory -= oldest.size
				// Remove from session list
				for i, g := range sessionGhostFiles {
					if g == oldest {
						sessionGhostFiles = append(sessionGhostFiles[:i], sessionGhostFiles[i+1:]...)
						break
					}
				}
			}

			globalMemory = atomic.LoadInt64(&fs.ghostMemoryGlobal)
		} else {
			break
		}
	}

	// Add new ghost file
	ghost := &GhostFile{
		node:       node,
		path:       filepath,
		sessionID:  ctx.SessionID,
		size:       fileSize,
		lastAccess: time.Now(),
	}

	key := fs.makeGhostKey(ctx.SessionID, filepath)
	fs.ghostFiles.Store(key, ghost)

	// Update memory usage
	currentMemory += fileSize
	fs.ghostMemoryUsage.Store(ctx.SessionID, currentMemory)
	atomic.AddInt64(&fs.ghostMemoryGlobal, fileSize)

	globalMem := atomic.LoadInt64(&fs.ghostMemoryGlobal)
	fs.logf("Ghost file registered: %s (session: %d, size: %d bytes, session: %d/%d bytes, global: %d/%d bytes, count: %d/%d)",
		filepath, ctx.SessionID, fileSize,
		currentMemory, fs.maxGhostMemoryPerSession,
		globalMem, fs.maxGhostMemoryGlobal,
		len(sessionGhostFiles)+1, fs.maxGhostFilesPerSession)
}

// getGhostFile retrieves a ghost file if it exists for the current session
func (fs *MemoryFS) getGhostFile(filepath string) *GhostFile {
	ctx := fs.getOperationContext()
	if ctx == nil {
		return nil
	}

	key := fs.makeGhostKey(ctx.SessionID, filepath)
	if v, ok := fs.ghostFiles.Load(key); ok {
		ghost := v.(*GhostFile)
		ghost.lastAccess = time.Now() // Update access time
		return ghost
	}
	return nil
}

// removeGhostFile removes a ghost file from the registry and updates memory usage
func (fs *MemoryFS) removeGhostFile(sessionID uint64, filepath string) {
	key := fs.makeGhostKey(sessionID, filepath)

	// Get the ghost file to know its size
	if v, ok := fs.ghostFiles.Load(key); ok {
		ghost := v.(*GhostFile)

		// Update memory usage for this session
		if currentMem, ok := fs.ghostMemoryUsage.Load(sessionID); ok {
			newMem := currentMem.(int64) - ghost.size
			if newMem <= 0 {
				fs.ghostMemoryUsage.Delete(sessionID)
			} else {
				fs.ghostMemoryUsage.Store(sessionID, newMem)
			}
		}

		// Update global memory counter (atomic subtraction)
		atomic.AddInt64(&fs.ghostMemoryGlobal, -ghost.size)
	}

	fs.ghostFiles.Delete(key)
}

// cleanupGhostFiles is a background goroutine that removes ghost files older than 3 minutes
func (fs *MemoryFS) cleanupGhostFiles() {
	ticker := time.NewTicker(30 * time.Second) // Check every 30 seconds
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			now := time.Now()
			expirationTime := 3 * time.Minute

			// Iterate through all ghost files
			fs.ghostFiles.Range(func(key, value interface{}) bool {
				ghost := value.(*GhostFile)
				age := now.Sub(ghost.lastAccess)

				if age > expirationTime {
					fs.removeGhostFile(ghost.sessionID, ghost.path)
					fs.logf("Ghost file expired and removed: %s (session: %d, age: %v, size: %d bytes)",
						ghost.path, ghost.sessionID, age, ghost.size)
				}
				return true // Continue iteration
			})

		case <-fs.ghostCleanupCh:
			// Shutdown signal
			return
		}
	}
}

// GetGhostFileStats returns current statistics about ghost files
func (fs *MemoryFS) GetGhostFileStats() GhostFileStats {
	stats := GhostFileStats{
		SessionFileCounts:    make(map[uint64]int),
		SessionVirtualMemory: make(map[uint64]int64),
		VirtualMemoryLimit:   fs.maxGhostMemoryGlobal,
	}

	// Count files and track sessions
	sessionSet := make(map[uint64]bool)
	fs.ghostFiles.Range(func(key, value interface{}) bool {
		ghost := value.(*GhostFile)
		stats.TotalFiles++
		stats.SessionFileCounts[ghost.sessionID]++
		sessionSet[ghost.sessionID] = true
		return true
	})

	stats.ActiveSessions = len(sessionSet)

	// Get virtual memory usage per session
	fs.ghostMemoryUsage.Range(func(key, value interface{}) bool {
		sessionID := key.(uint64)
		virtualMem := value.(int64)
		stats.SessionVirtualMemory[sessionID] = virtualMem
		return true
	})

	// Get total virtual memory (atomic read)
	stats.VirtualMemoryTotal = atomic.LoadInt64(&fs.ghostMemoryGlobal)

	// Estimate real memory usage
	// Each ghost file uses approximately:
	// - GhostFile struct: ~80 bytes
	// - MemoryNode struct: ~200 bytes (minimal, no content in honeypot)
	// - Map overhead, keys, etc: ~50 bytes
	// Total: ~330 bytes per file
	const bytesPerGhostFile = 330
	stats.RealMemoryEstimate = int64(stats.TotalFiles) * bytesPerGhostFile

	return stats
}

// Shutdown stops the ghost files cleanup goroutine
func (fs *MemoryFS) Shutdown() {
	close(fs.ghostCleanupCh)
}
