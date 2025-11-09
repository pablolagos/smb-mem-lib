# Anti-Ransomware SMB2 Honeypot Library

A lightweight, high-performance Go library for creating SMB2/3 honeypots designed to detect and analyze ransomware activity. Features in-memory filesystem, zero-storage honeypot mode, and advanced behavioral detection capabilities.

## 🎯 Key Features

- **In-Memory SMB2/3 Server**: Full SMB2/3 protocol implementation with no disk I/O
- **Honeypot Mode**: Silently discards writes while appearing successful to attackers
- **Ghost Files System**: Tracks closed files temporarily to detect ransomware encryption patterns
- **Behavioral Callbacks**: Intercept file creation, writes, and renames in real-time
- **Scalable Architecture**: Handles 1000+ concurrent sessions with minimal RAM (~1MB)
- **Session Isolation**: Ghost files isolated per SMB session for accurate tracking
- **Configurable Limits**: Fine-grained control over memory usage and file limits
- **Real-time Monitoring**: Built-in statistics for detection and analysis
- **Zero Dependencies**: Pure Go implementation, easy to integrate

## Quick Start

### Installation

```bash
go get github.com/pablolagos/smb-mem-lib
```

### Basic Honeypot Example

```go
package main

import (
    "log"
    "github.com/pablolagos/smb-mem-lib/memoryshare"
)

func main() {
    share, err := memoryshare.New(memoryshare.Options{
        Port:          8445,
        Address:       "0.0.0.0",
        ShareName:     "HoneypotShare",
        AllowGuest:    true,
        DiscardWrites: true, // Enable honeypot mode
        Console:       true,
    })
    if err != nil {
        log.Fatal(err)
    }

    // Add bait files
    share.AddDirectory("/Documents")
    share.AddFile("/Documents/Financial_Report_2024.xlsx", []byte("Q1 Revenue: $1.2M"))
    share.AddFile("/Documents/Customer_Database.xlsx", []byte("CustomerID,Name,Email"))

    // Start server
    log.Fatal(share.Serve())
}
```

### Test Connection

```bash
smbclient //127.0.0.1/HoneypotShare -p 8445 -N
```

## Scalability & Performance

### Memory Usage (Honeypot Mode)

The library distinguishes between **virtual** and **real** memory:

- **Virtual Memory**: Tracked file sizes (for limiting attacker engagement)
- **Real Memory**: Actual RAM consumed (~330 bytes per ghost file)

#### Example: 1000 Concurrent Sessions

```go
Configuration:
  MaxGhostFilesPerSession:  3 files
  MaxGhostMemoryPerSession: 10 MB (virtual)
  MaxGhostMemoryGlobal:     1 GB (virtual)

Real RAM Usage:
  1000 sessions × 3 files × 330 bytes = ~1 MB RAM

Virtual Tracking:
  Up to 1 GB of "pretend" file sizes across all sessions
```

### Recommended Configurations

#### Small Deployment (100 sessions)
```go
memoryshare.Options{
    MaxGhostFilesPerSession:  5,
    MaxGhostMemoryPerSession: 50 * 1024 * 1024,   // 50MB virtual
    MaxGhostMemoryGlobal:     2 * 1024 * 1024 * 1024, // 2GB virtual
}
// Real RAM: ~165 KB
```

#### Production (1,000 sessions)
```go
memoryshare.Options{
    MaxGhostFilesPerSession:  3,
    MaxGhostMemoryPerSession: 10 * 1024 * 1024,   // 10MB virtual
    MaxGhostMemoryGlobal:     500 * 1024 * 1024,  // 500MB virtual
}
// Real RAM: ~1 MB
```

#### Enterprise (10,000 sessions)
```go
memoryshare.Options{
    MaxGhostFilesPerSession:  2,
    MaxGhostMemoryPerSession: 5 * 1024 * 1024,    // 5MB virtual
    MaxGhostMemoryGlobal:     200 * 1024 * 1024,  // 200MB virtual
}
// Real RAM: ~6.6 MB
```

## Ransomware Detection

### Behavioral Callbacks

Intercept suspicious operations in real-time:

```go
package main

import (
    "log"
    "strings"
    "github.com/pablolagos/smb-mem-lib/memoryshare"
    "github.com/pablolagos/smb-mem-lib/vfs"
)

func main() {
    share, _ := memoryshare.New(memoryshare.Options{
        Port:          8445,
        DiscardWrites: true,
    })

    // Detect file creation
    share.SetCreateCallback(func(ctx *vfs.OperationContext, filepath string, isDir bool, mode int) vfs.CreateInterceptResult {
        if strings.Contains(strings.ToLower(filepath), "readme") ||
           strings.Contains(strings.ToLower(filepath), "decrypt") {
            log.Printf("🚨 [RANSOMWARE] Note detected: %s from %s", filepath, ctx.RemoteAddr)
        }
        return vfs.CreateInterceptResult{Block: false}
    })

    // Detect encryption (file renames)
    share.SetRenameCallback(func(ctx *vfs.OperationContext, fromPath, toPath string) vfs.RenameInterceptResult {
        ransomwareExts := []string{".encrypted", ".locked", ".crypto", ".wcry"}
        for _, ext := range ransomwareExts {
            if strings.HasSuffix(strings.ToLower(toPath), ext) {
                log.Printf("🚨🚨🚨 [ENCRYPTION] %s -> %s from %s",
                    fromPath, toPath, ctx.RemoteAddr)
            }
        }
        return vfs.RenameInterceptResult{Block: false}
    })

    // Detect malware writes
    share.SetWriteCallback(func(ctx *vfs.OperationContext, handle vfs.VfsHandle, filename string, data []byte, offset uint64) vfs.WriteInterceptResult {
        // Detect PE executable (MZ header)
        if len(data) >= 2 && data[0] == 'M' && data[1] == 'Z' {
            log.Printf("🔍 [EXECUTABLE] PE file detected: %s from %s", filename, ctx.RemoteAddr)
        }
        return vfs.WriteInterceptResult{Block: false}
    })

    share.Serve()
}
```

### Ghost Files: Advanced Ransomware Detection

Ghost files enable detection of the classic ransomware workflow:

1. **Open** file.docx
2. **Write** encrypted data
3. **Close** file.docx
4. **Reopen** file.docx (from ghost registry)
5. **Rename** file.docx → file.docx.encrypted

```go
// In honeypot mode, files closed within 3 minutes remain accessible
// This allows detecting ransomware that encrypts then renames
share, _ := memoryshare.New(memoryshare.Options{
    DiscardWrites: true,  // Ghost files automatically enabled
})

// Workflow that works:
// 1. Create file.test
// 2. Write data
// 3. Close file.test (becomes ghost file)
// 4. Reopen file.test (from ghost registry - still works!)
// 5. Rename to file.test.encrypted (rename callback triggered)
```

## 📈 Monitoring & Statistics

### Real-time Statistics

```go
stats := share.GetGhostFileStats()

log.Printf("=== Honeypot Statistics ===")
log.Printf("Total ghost files: %d", stats.TotalFiles)
log.Printf("Active sessions: %d", stats.ActiveSessions)
log.Printf("Virtual memory: %.2f MB / %.2f MB",
    float64(stats.VirtualMemoryTotal)/(1024*1024),
    float64(stats.VirtualMemoryLimit)/(1024*1024))
log.Printf("Real RAM used: %.2f KB",
    float64(stats.RealMemoryEstimate)/1024)

// Per-session breakdown
for sessionID, count := range stats.SessionFileCounts {
    virtualMem := stats.SessionVirtualMemory[sessionID]
    log.Printf("  Session %d: %d files, %.2f MB virtual",
        sessionID, count, float64(virtualMem)/(1024*1024))
}
```

### Statistics Output Example

```
=== Honeypot Statistics ===
Total ghost files: 127
Active sessions: 42
Virtual memory: 87.34 MB / 1024.00 MB
Real RAM used: 41.91 KB
  Session 1001: 3 files, 12.50 MB virtual
  Session 1002: 2 files, 8.00 MB virtual
  ...
```

## Configuration Options

### Complete Options Reference

```go
type Options struct {
    // Network
    Port     int    // TCP port (default: 8082)
    Address  string // Bind address (default: "127.0.0.1")

    // Share configuration
    ShareName  string // Share name (default: "Share")
    Hostname   string // Server hostname (default: "SmbServer")
    AllowGuest bool   // Enable guest access (default: false)

    // Performance
    MaxIOReads  int // Max concurrent reads (default: 4)
    MaxIOWrites int // Max concurrent writes (default: 4)

    // Features
    Advertise bool // Bonjour/mDNS advertisement (default: true)
    Xattrs    bool // Extended attributes (default: true)

    // Honeypot mode
    DiscardWrites bool // Enable honeypot mode (default: false)

    // Ghost files limits (honeypot mode)
    MaxGhostFilesPerSession  int   // Files per session (default: 3)
    MaxGhostMemoryPerSession int64 // Virtual size/session (default: 10MB)
    MaxGhostMemoryGlobal     int64 // Virtual size global (default: 1GB)

    // Logging
    Debug   bool          // Debug logging (default: false)
    Console bool          // Log to stdout (default: false)
    LogFile string        // Log file path (default: ~/smb2-go.log)
    Logger  SimpleLogger  // Custom logger (default: logrus)
}
```

### Custom Logger Example

```go
type MyLogger struct{}

func (l *MyLogger) Printf(format string, args ...interface{}) {
    log.Printf("[SMB] "+format, args...)
}

func (l *MyLogger) Print(args ...interface{}) {
    log.Print(append([]interface{}{"[SMB]"}, args...)...)
}

share, _ := memoryshare.New(memoryshare.Options{
    Logger: &MyLogger{},
    // ... other options
})
```

### Silent Mode (No Logging)

```go
share, _ := memoryshare.New(memoryshare.Options{
    Logger: memoryshare.NoOpLogger,
})
```

## Examples

See the `examples/` directory for complete working examples:

- **`honeypot-ransomware/`**: Full ransomware detection honeypot
- **`custom-logger/`**: Custom logging implementations
- **`all-callbacks/`**: Demonstrates all callback types

### Run Examples

```bash
# Ransomware detection honeypot
cd examples/honeypot-ransomware
go run main.go

# Custom logger demo
cd examples/custom-logger
go run main.go
```

## Architecture

### Components

```
┌─────────────────────────────────────────┐
│         memoryshare.MemoryShare         │
│  (Public API / High-level interface)    │
└─────────────────┬───────────────────────┘
                  │
                  ▼
┌─────────────────────────────────────────┐
│           memoryshare.MemoryFS          │
│     (In-memory VFS implementation)      │
│  • Ghost files registry (per-session)   │
│  • LRU eviction (session + global)      │
│  • Virtual vs real memory tracking      │
└─────────────────┬───────────────────────┘
                  │
                  ▼
┌─────────────────────────────────────────┐
│            server.Server                │
│      (SMB2/3 protocol handling)         │
│  • Session management                   │
│  • Tree connections                     │
│  • Encryption/signing                   │
└─────────────────────────────────────────┘
```

### Ghost Files System

```
Session 1                     Session 2
  │                              │
  ├─ file1.doc (2MB virtual)     ├─ data.xlsx (5MB virtual)
  ├─ file2.pdf (3MB virtual)     └─ report.pdf (3MB virtual)
  └─ file3.txt (1MB virtual)

  Per-session limit: 10MB        Per-session limit: 10MB
  Files per session: 3           Files per session: 2

  ────────────────────────────────────────────

  Global limit: 1GB virtual
  Real RAM usage: ~1.65 KB (5 files × 330 bytes)
```

### Eviction Policy

1. **Per-session eviction** (LRU): When session exceeds limits
2. **Global eviction** (LRU): When total exceeds global limit
3. **Time-based expiration**: Files older than 3 minutes
4. **Cleanup interval**: Every 30 seconds

## Security Considerations

### Honeypot Best Practices

1. **Isolation**: Run honeypot in isolated network segment
2. **Monitoring**: Enable logging and alerting
3. **Rate Limiting**: Consider adding connection rate limits
4. **False Positives**: Tune detection thresholds for your environment
5. **Legal Compliance**: Ensure honeypot use complies with local laws

### Production Deployment

```go
// Conservative configuration for production
share, _ := memoryshare.New(memoryshare.Options{
    Port:          445,
    Address:       "0.0.0.0",
    AllowGuest:    true,
    DiscardWrites: true,
    Console:       false, // Log to file
    Debug:         false,

    // Conservative limits
    MaxGhostFilesPerSession:  2,
    MaxGhostMemoryPerSession: 5 * 1024 * 1024,    // 5MB
    MaxGhostMemoryGlobal:     100 * 1024 * 1024,  // 100MB
})
```

## Testing

```bash
# Run all tests
go test ./...

# Run specific tests
go test ./memoryshare -v -run TestGhostFiles

# Run with coverage
go test ./memoryshare -cover

# Skip slow tests
go test ./memoryshare -short
```

## API Reference

### MemoryShare Methods

```go
// File operations
AddFile(filepath string, content []byte) error
AddFileWithMode(filepath string, content []byte, mode uint32) error
AddDirectory(dirpath string) error
AddDirectoryWithMode(dirpath string, mode uint32) error

// Callbacks
SetWriteCallback(callback vfs.WriteCallback)
SetCreateCallback(callback vfs.CreateCallback)
SetRenameCallback(callback vfs.RenameCallback)

// Statistics
GetGhostFileStats() GhostFileStats

// Server control
Serve() error
Shutdown() error
```

### Callback Types

```go
type CreateCallback func(
    ctx *OperationContext,
    filepath string,
    isDir bool,
    mode int,
) CreateInterceptResult

type RenameCallback func(
    ctx *OperationContext,
    fromPath string,
    toPath string,
) RenameInterceptResult

type WriteCallback func(
    ctx *OperationContext,
    handle VfsHandle,
    filename string,
    data []byte,
    offset uint64,
) WriteInterceptResult
```

### OperationContext

```go
type OperationContext struct {
    RemoteAddr string  // Client IP:port
    LocalAddr  string  // Server IP:port
    SessionID  uint64  // SMB session ID
    Username   string  // Authenticated username
}
```

## License

Dual licensed:
- **AGPL-3.0** for open source use
- **Commercial license** available for proprietary use

Contact for commercial licensing options.

## Acknowledgments

- SMB2/3 protocol implementation based on community research
- Inspired by real-world ransomware behavior analysis

## Support

- Issues: [GitHub Issues](https://github.com/pablolagos/smb-mem-lib/issues)
- Discussions: [GitHub Discussions](https://github.com/pablolagos/smb-mem-lib/discussions)

---

**Note**: This is a honeypot/research tool. Use responsibly and in compliance with applicable laws and regulations.
