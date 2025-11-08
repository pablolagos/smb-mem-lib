package vfs

type VfsNode uint64
type VfsHandle uint64

const VFS_ROOT_NODE = VfsNode(1)

type EntryReply struct {
	Node       VfsNode
	Generation uint64
	Attrs      Attributes
}

type DirInfo struct {
	Name string
	Attributes
}

type VFSFileSystem interface {
	GetAttr(VfsHandle) (*Attributes, error)
	SetAttr(VfsHandle, *Attributes) (*Attributes, error)

	StatFS(VfsHandle) (*FSAttributes, error)

	FSync(VfsHandle) error

	Flush(VfsHandle) error

	//Mknod(VfsNode, string, int, int) (*EntryReply, error)

	//Create(VfsNode, string, int, int) (*EntryReply, VfsHandle, error)
	Open(string, int, int) (VfsHandle, error)
	Close(VfsHandle) error

	Lookup(VfsHandle, string) (*Attributes, error)

	Mkdir(string, int) (*Attributes, error)

	Read(VfsHandle, []byte, uint64, int) (int, error)
	Write(VfsHandle, []byte, uint64, int) (int, error)

	OpenDir(string) (VfsHandle, error)
	ReadDir(VfsHandle, int, int) ([]DirInfo, error)

	Readlink(VfsHandle) (string, error)

	Unlink(VfsHandle) error

	//Rmdir(VfsNode, string) error

	Truncate(VfsHandle, uint64) error

	Rename(VfsHandle, string, int) error

	Symlink(VfsHandle, string, int) (*Attributes, error)
	Link(VfsNode, VfsNode, string) (*Attributes, error)

	Listxattr(VfsHandle) ([]string, error)
	Getxattr(VfsHandle, string, []byte) (int, error)
	Setxattr(VfsHandle, string, []byte) error
	Removexattr(VfsHandle, string) error

	//RegisterNotify(VfsHandle, chan *NotifyEvent) error
	//RemoveNotify(VfsHandle) error
}

// ShareMask is a bitmask of operations that are permitted
type ShareMask uint32

const (
	// ShareMaskRead permits calls to VirtualRead().
	ShareMaskRead ShareMask = 1 << iota
	// ShareMaskWrite permits calls to VirtualWrite().
	ShareMaskWrite
)

type NotifyEvent struct {
	EvType uint8
	Handle VfsHandle
	Name   string
}

// OperationContext contains metadata about the current operation
type OperationContext struct {
	// RemoteAddr is the IP address of the client making the request
	RemoteAddr string
	// LocalAddr is the server IP address
	LocalAddr string
	// SessionID is the SMB session identifier
	SessionID uint64
	// Username is the authenticated username (if available)
	Username string
}

// WriteInterceptResult controls what happens after the write callback
type WriteInterceptResult struct {
	// Block prevents the write from being saved if true
	Block bool
	// Error if set, will be returned to the client instead of performing write
	Error error
}

// WriteCallback is called before a file write operation is committed
// Parameters:
//   - ctx: Connection and session context
//   - handle: VFS file handle
//   - filename: Full path of the file being written
//   - data: Content being written
//   - offset: Offset in the file where data will be written
//
// Return: WriteInterceptResult to control the operation
type WriteCallback func(ctx *OperationContext, handle VfsHandle, filename string, data []byte, offset uint64) WriteInterceptResult

// VFSWithContext is an optional interface that VFS implementations can implement
// to receive operation context and support callbacks
type VFSWithContext interface {
	VFSFileSystem

	// SetOperationContext sets the context for subsequent operations
	// This is called by the server before each operation
	SetOperationContext(ctx *OperationContext)

	// SetWriteCallback registers a callback to be invoked before writes
	SetWriteCallback(callback WriteCallback)
}
