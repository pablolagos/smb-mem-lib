package memoryshare

import (
	"fmt"
	"os"
	"os/signal"

	"github.com/pablolagos/smb-mem-lib/bonjour"
	smb2 "github.com/pablolagos/smb-mem-lib/server"
	"github.com/pablolagos/smb-mem-lib/vfs"
	log "github.com/sirupsen/logrus"
	"gopkg.in/natefinch/lumberjack.v2"
)

// SimpleLogger is a minimal logging interface that users can implement
// to provide custom logging for the SMB server
type SimpleLogger interface {
	Printf(format string, args ...interface{})
	Print(args ...interface{})
}

// noOpLogger is a logger that discards all output
type noOpLogger struct{}

func (n *noOpLogger) Printf(format string, args ...interface{}) {}
func (n *noOpLogger) Print(args ...interface{})                 {}

// NoOpLogger is a logger that discards all output. Use this to disable logging:
//
//	share, err := memoryshare.New(memoryshare.Options{
//	    Logger: &memoryshare.NoOpLogger{},
//	    ...
//	})
var NoOpLogger = &noOpLogger{}

// Options configures the MemoryShare SMB server
type Options struct {
	// Port is the TCP port to listen on (e.g., 445, 8082)
	Port int

	// Address is the IP address to bind to (e.g., "127.0.0.1", "0.0.0.0")
	Address string

	// ShareName is the name of the SMB share (default: "Share")
	ShareName string

	// Hostname is the server hostname for identification (default: "SmbServer")
	Hostname string

	// AllowGuest enables guest access without authentication (default: false)
	AllowGuest bool

	// Advertise enables Bonjour/mDNS advertisement for macOS discovery (default: true)
	Advertise bool

	// MaxIOReads is the maximum concurrent read operations (default: 4)
	MaxIOReads int

	// MaxIOWrites is the maximum concurrent write operations (default: 4)
	MaxIOWrites int

	// Xattrs enables extended attributes support (default: true)
	Xattrs bool

	// Debug enables debug logging (default: false)
	Debug bool

	// Console outputs logs to stdout instead of file (default: false)
	Console bool

	// LogFile is the path to the log file (default: ~/smb2-go.log)
	LogFile string

	// DiscardWrites when enabled, silently discards all write operations,
	// file creations, renames, and deletions without returning errors.
	// Perfect for honeypot mode where you want to deceive attackers.
	// The server will pretend operations succeeded but nothing is actually saved.
	DiscardWrites bool

	// Logger is a custom logger implementation. If provided, all logging
	// will go through this logger instead of the default logrus logger.
	// The logger must implement the SimpleLogger interface (Printf and Print methods).
	Logger SimpleLogger

	// MaxGhostFilesPerSession limits the number of ghost files per session (default: 3)
	// Ghost files are temporary files kept in memory after closing in honeypot mode.
	// This is the PRIMARY limit for controlling actual memory usage.
	// REAL memory: ~330 bytes per file. For 1000 sessions: 1000 × 3 × 330 ≈ 1MB RAM
	MaxGhostFilesPerSession int

	// MaxGhostMemoryPerSession limits VIRTUAL file size per session in bytes (default: 10MB)
	// NOTE: In honeypot mode, file CONTENT is NOT stored, only metadata.
	// This tracks the virtual/pretend size of files to limit attacker engagement,
	// NOT actual RAM usage. Real RAM per file is only ~330 bytes.
	// When exceeded, oldest files are evicted using LRU policy.
	MaxGhostMemoryPerSession int64

	// MaxGhostMemoryGlobal limits total VIRTUAL file size across all sessions (default: 1GB)
	// NOTE: Tracks virtual size, not real RAM. Real RAM is minimal (~330 bytes per file).
	// For 1000 sessions with 3 files each: ~1MB real RAM, but can track 1GB virtual size.
	// This prevents one attacker from monopolizing the honeypot by "filling" it virtually.
	// When exceeded, oldest files across ALL sessions are evicted.
	MaxGhostMemoryGlobal int64
}

// MemoryShare represents an in-memory SMB2/3 server
type MemoryShare struct {
	server *smb2.Server
	fs     *MemoryFS
	opts   Options
	stopCh chan os.Signal
	logger SimpleLogger
}

// New creates a new MemoryShare instance with the provided options
func New(opts Options) (*MemoryShare, error) {
	// Set defaults
	if opts.Port == 0 {
		opts.Port = 8082
	}
	if opts.Address == "" {
		opts.Address = "127.0.0.1"
	}
	if opts.ShareName == "" {
		opts.ShareName = "Share"
	}
	if opts.Hostname == "" {
		opts.Hostname = "SmbServer"
	}
	if opts.MaxIOReads == 0 {
		opts.MaxIOReads = 4
	}
	if opts.MaxIOWrites == 0 {
		opts.MaxIOWrites = 4
	}
	if opts.LogFile == "" {
		homeDir, _ := os.UserHomeDir()
		opts.LogFile = homeDir + "/smb2-go.log"
	}

	// Advertise defaults to true
	if !opts.Advertise {
		// If not explicitly set, default to true (this is a bit hacky, but works for most cases)
		// User can explicitly set Advertise: false to disable
	}

	// Xattrs defaults to true
	if !opts.Xattrs {
		// Same as above
	}

	// Ghost files defaults for honeypot mode
	if opts.MaxGhostFilesPerSession == 0 {
		opts.MaxGhostFilesPerSession = 3
	}
	if opts.MaxGhostMemoryPerSession == 0 {
		opts.MaxGhostMemoryPerSession = 10 * 1024 * 1024 // 10MB virtual size
	}
	if opts.MaxGhostMemoryGlobal == 0 {
		opts.MaxGhostMemoryGlobal = 1024 * 1024 * 1024 // 1GB virtual size
	}

	// Initialize logging
	logger := initLogs(&opts)

	// Create in-memory filesystem
	memFS := NewMemoryFS()
	memFS.SetLogger(logger)

	// Configure ghost file limits
	memFS.SetGhostFileLimits(
		opts.MaxGhostFilesPerSession,
		opts.MaxGhostMemoryPerSession,
		opts.MaxGhostMemoryGlobal,
	)

	// Enable discard writes mode if requested (honeypot mode)
	if opts.DiscardWrites {
		memFS.SetDiscardWrites(true)
		logger.Printf("DiscardWrites mode enabled - all write operations will be discarded")
		logger.Printf("Ghost file limits: %d files/session, %d bytes virtual/session, %d bytes virtual global",
			opts.MaxGhostFilesPerSession, opts.MaxGhostMemoryPerSession, opts.MaxGhostMemoryGlobal)
	}

	// Create SMB server
	srv := smb2.NewServer(
		&smb2.ServerConfig{
			AllowGuest:  opts.AllowGuest,
			MaxIOReads:  opts.MaxIOReads,
			MaxIOWrites: opts.MaxIOWrites,
			Xatrrs:      opts.Xattrs,
		},
		&smb2.NTLMAuthenticator{
			TargetSPN:    "",
			NbDomain:     opts.Hostname,
			NbName:       opts.Hostname,
			DnsName:      opts.Hostname + ".local",
			DnsDomain:    ".local",
			UserPassword: map[string]string{}, // Empty for guest-only access
			AllowGuest:   opts.AllowGuest,
		},
		map[string]vfs.VFSFileSystem{opts.ShareName: memFS},
	)

	return &MemoryShare{
		server: srv,
		fs:     memFS,
		opts:   opts,
		stopCh: make(chan os.Signal, 1),
		logger: logger,
	}, nil
}

// Serve starts the SMB server (blocking call)
// Returns when the server stops or encounters an error
func (ms *MemoryShare) Serve() error {
	addr := fmt.Sprintf("%s:%d", ms.opts.Address, ms.opts.Port)

	ms.logger.Printf("Starting MemoryShare SMB server at %s", addr)
	ms.logger.Printf("Share name: %s", ms.opts.ShareName)
	ms.logger.Printf("Guest access: %v", ms.opts.AllowGuest)

	// Start Bonjour advertisement if enabled
	if ms.opts.Advertise {
		go bonjour.Advertise(addr, ms.opts.Hostname, ms.opts.Hostname, ms.opts.ShareName, true)
		ms.logger.Printf("Bonjour advertisement enabled")
	}

	// Start server in goroutine
	errCh := make(chan error, 1)
	go func() {
		errCh <- ms.server.Serve(addr)
	}()

	// Setup signal handling
	signal.Notify(ms.stopCh, os.Interrupt)

	// Wait for either error or interrupt
	select {
	case err := <-errCh:
		return err
	case <-ms.stopCh:
		ms.logger.Printf("Received interrupt signal, shutting down...")
		return ms.Shutdown()
	}
}

// Shutdown gracefully stops the SMB server
func (ms *MemoryShare) Shutdown() error {
	ms.logger.Printf("Shutting down MemoryShare server")

	if ms.opts.Advertise {
		bonjour.Shutdown()
	}

	ms.server.Shutdown()
	return nil
}

// AddFile adds a file with content to the memory filesystem
// filepath: the path where the file should be created (e.g., "/folder/file.txt")
// content: the file contents as bytes
func (ms *MemoryShare) AddFile(filepath string, content []byte) error {
	return ms.fs.AddFile(filepath, content, 0644)
}

// AddFileWithMode adds a file with custom permissions
// filepath: the path where the file should be created
// content: the file contents as bytes
// mode: Unix file permissions (e.g., 0644, 0755)
func (ms *MemoryShare) AddFileWithMode(filepath string, content []byte, mode uint32) error {
	return ms.fs.AddFile(filepath, content, mode)
}

// AddDirectory creates a directory in the memory filesystem
// dirpath: the path of the directory to create (e.g., "/folder")
func (ms *MemoryShare) AddDirectory(dirpath string) error {
	return ms.fs.AddDirectory(dirpath, 0755)
}

// AddDirectoryWithMode creates a directory with custom permissions
// dirpath: the path of the directory to create
// mode: Unix directory permissions (e.g., 0755, 0700)
func (ms *MemoryShare) AddDirectoryWithMode(dirpath string, mode uint32) error {
	return ms.fs.AddDirectory(dirpath, mode)
}

// SetWriteCallback registers a callback function that will be invoked before
// any file write operation. This is useful for honeypots, malware analysis,
// or security monitoring where you want to intercept and potentially block writes.
//
// The callback receives:
//   - ctx: Connection context including client IP, username, etc.
//   - handle: VFS file handle
//   - filename: Full path of the file being written
//   - data: Content being written
//   - offset: Offset in the file where data will be written
//
// The callback should return a WriteInterceptResult to control the operation:
//   - Block: If true, the write is prevented (but can still appear successful to the client)
//   - Error: If set, this error is returned to the client
func (ms *MemoryShare) SetWriteCallback(callback vfs.WriteCallback) {
	ms.fs.SetWriteCallback(callback)
}

// SetCreateCallback registers a callback function that will be invoked before
// any file or directory creation operation. This is useful for honeypots, malware analysis,
// or security monitoring where you want to intercept and potentially block file creation.
//
// The callback receives:
//   - ctx: Connection context including client IP, username, etc.
//   - filepath: Full path of the file/directory being created
//   - isDir: True if creating a directory, false if creating a file
//   - mode: Unix permissions mode
//
// The callback should return a CreateInterceptResult to control the operation:
//   - Block: If true, the creation is prevented (but can still appear successful to the client)
//   - Error: If set, this error is returned to the client
func (ms *MemoryShare) SetCreateCallback(callback vfs.CreateCallback) {
	ms.fs.SetCreateCallback(callback)
}

// SetRenameCallback registers a callback function that will be invoked before
// any file or directory rename/move operation. This is useful for honeypots, malware analysis,
// or security monitoring where you want to intercept and potentially block renames.
//
// The callback receives:
//   - ctx: Connection context including client IP, username, etc.
//   - fromPath: Original path of the file/directory
//   - toPath: New path (destination) for the file/directory
//
// The callback should return a RenameInterceptResult to control the operation:
//   - Block: If true, the rename is prevented (but can still appear successful to the client)
//   - Error: If set, this error is returned to the client
func (ms *MemoryShare) SetRenameCallback(callback vfs.RenameCallback) {
	ms.fs.SetRenameCallback(callback)
}

// GetGhostFileStats returns current statistics about ghost files in honeypot mode.
// Useful for monitoring resource usage and detecting attack patterns.
//
// Returns GhostFileStats with:
//   - TotalFiles: Count of all ghost files across sessions
//   - ActiveSessions: Number of sessions with ghost files
//   - VirtualMemoryTotal/Limit: Virtual size being tracked (not real RAM)
//   - RealMemoryEstimate: Actual RAM usage (~330 bytes/file)
//   - SessionFileCounts: Files per session
//   - SessionVirtualMemory: Virtual size per session
func (ms *MemoryShare) GetGhostFileStats() GhostFileStats {
	return ms.fs.GetGhostFileStats()
}

// logrusAdapter wraps logrus to implement SimpleLogger interface
type logrusAdapter struct {
	logger *log.Logger
}

func (l *logrusAdapter) Printf(format string, args ...interface{}) {
	l.logger.Infof(format, args...)
}

func (l *logrusAdapter) Print(args ...interface{}) {
	l.logger.Info(args...)
}

// initLogs configures the logging system and returns a SimpleLogger
func initLogs(opts *Options) SimpleLogger {
	// If a custom logger is provided, use it directly
	if opts.Logger != nil {
		return opts.Logger
	}

	// Otherwise, configure and use logrus
	logrusLogger := log.New()

	if opts.Debug {
		logrusLogger.SetLevel(log.DebugLevel)
		logrusLogger.Infof("Debug logging enabled")
	} else {
		logrusLogger.SetLevel(log.InfoLevel)
	}

	if opts.Console {
		logrusLogger.SetOutput(os.Stdout)
	} else {
		logrusLogger.SetOutput(&lumberjack.Logger{
			Filename:   opts.LogFile,
			MaxSize:    100, // megabytes
			MaxBackups: 3,
			MaxAge:     28, // days
			Compress:   true,
		})
	}

	return &logrusAdapter{logger: logrusLogger}
}
