package main

import (
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"io"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/macos-fuse-t/go-smb2/vfs"
	log "github.com/sirupsen/logrus"
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

// MemoryFS implements VFSFileSystem interface with in-memory storage
type MemoryFS struct {
	root      *MemoryNode
	openFiles sync.Map
	nextInode uint64
	mu        sync.RWMutex
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
		root:      root,
		nextInode: 2,
	}

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

	log.Debugf("Added file: %s (size: %d bytes)", filepath, len(content))
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

// nodeToAttributes converts a MemoryNode to vfs.Attributes
func nodeToAttributes(node *MemoryNode) *vfs.Attributes {
	node.mu.RLock()
	defer node.mu.RUnlock()

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

	return nodeToAttributes(node), nil
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

	if err != nil {
		return 0, err
	}

	h := vfs.VfsHandle(randint64Memory())
	fs.openFiles.Store(h, &OpenMemoryFile{
		node:   node,
		handle: h,
		path:   filepath,
		isDir:  node.isDir,
	})

	log.Debugf("Opened file: %s (handle: %d)", filepath, h)
	return h, nil
}

// Close implements vfs.VFSFileSystem
func (fs *MemoryFS) Close(handle vfs.VfsHandle) error {
	_, ok := fs.openFiles.Load(handle)
	if !ok {
		return fmt.Errorf("invalid handle")
	}

	fs.openFiles.Delete(handle)
	log.Debugf("Closed handle: %d", handle)
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

	log.Debugf("Opened directory: %s (handle: %d)", filepath, h)
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
		node.parent.mu.Lock()
		delete(node.parent.children, node.name)
		node.parent.mu.Unlock()
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
