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
}

// MemoryShare represents an in-memory SMB2/3 server
type MemoryShare struct {
	server *smb2.Server
	fs     *MemoryFS
	opts   Options
	stopCh chan os.Signal
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

	// Initialize logging
	initLogs(&opts)

	// Create in-memory filesystem
	memFS := NewMemoryFS()

	// Enable discard writes mode if requested (honeypot mode)
	if opts.DiscardWrites {
		memFS.SetDiscardWrites(true)
		log.Infof("DiscardWrites mode enabled - all write operations will be discarded")
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
	}, nil
}

// Serve starts the SMB server (blocking call)
// Returns when the server stops or encounters an error
func (ms *MemoryShare) Serve() error {
	addr := fmt.Sprintf("%s:%d", ms.opts.Address, ms.opts.Port)

	log.Infof("Starting MemoryShare SMB server at %s", addr)
	log.Infof("Share name: %s", ms.opts.ShareName)
	log.Infof("Guest access: %v", ms.opts.AllowGuest)

	// Start Bonjour advertisement if enabled
	if ms.opts.Advertise {
		go bonjour.Advertise(addr, ms.opts.Hostname, ms.opts.Hostname, ms.opts.ShareName, true)
		log.Infof("Bonjour advertisement enabled")
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
		log.Infof("Received interrupt signal, shutting down...")
		return ms.Shutdown()
	}
}

// Shutdown gracefully stops the SMB server
func (ms *MemoryShare) Shutdown() error {
	log.Infof("Shutting down MemoryShare server")

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

// initLogs configures the logging system
func initLogs(opts *Options) {
	if opts.Debug {
		log.SetLevel(log.DebugLevel)
		log.Infof("Debug logging enabled")
	} else {
		log.SetLevel(log.InfoLevel)
	}

	if opts.Console {
		log.SetOutput(os.Stdout)
	} else {
		log.SetOutput(&lumberjack.Logger{
			Filename:   opts.LogFile,
			MaxSize:    100, // megabytes
			MaxBackups: 3,
			MaxAge:     28, // days
			Compress:   true,
		})
	}
}
