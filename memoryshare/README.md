# MemoryShare - In-Memory SMB2/3 Server Library

A lightweight Go library for creating in-memory SMB2/3 file servers. Perfect for testing, honeypots, or temporary file sharing.

## Features

- **In-Memory Storage**: All files and directories are stored entirely in RAM
- **SMB2/3 Protocol**: Full SMB2 and SMB3 protocol support
- **macOS Compatible**: Supports extended attributes, Bonjour advertisement, and Apple extensions
- **Simple API**: Easy-to-use interface with minimal configuration
- **Thread-Safe**: Concurrent file operations are handled safely
- **Guest Access**: Optional guest authentication for easy testing

## Installation

```bash
go get github.com/pablolagos/smb-mem-lib/memoryshare
```

## Quick Start

```go
package main

import (
    "log"
    "github.com/pablolagos/smb-mem-lib/memoryshare"
)

func main() {
    // Create a new MemoryShare server
    share, err := memoryshare.New(memoryshare.Options{
        Port:       8082,
        Address:    "127.0.0.1",
        ShareName:  "MyShare",
        AllowGuest: true,
    })
    if err != nil {
        log.Fatal(err)
    }

    // Add files to the in-memory filesystem
    share.AddFile("/welcome.txt", []byte("Hello, SMB!"))
    share.AddDirectory("/docs")
    share.AddFile("/docs/readme.md", []byte("# Documentation"))

    // Start the server (blocking)
    if err := share.Serve(); err != nil {
        log.Fatal(err)
    }
}
```

## API Reference

### Creating a Server

```go
func New(opts Options) (*MemoryShare, error)
```

Creates a new MemoryShare instance with the provided options.

### Options

```go
type Options struct {
    Port        int    // TCP port (default: 8082)
    Address     string // IP address (default: "127.0.0.1")
    ShareName   string // SMB share name (default: "Share")
    Hostname    string // Server hostname (default: "SmbServer")
    AllowGuest  bool   // Allow guest access (default: false)
    Advertise   bool   // Enable Bonjour/mDNS (default: true)
    MaxIOReads  int    // Max concurrent reads (default: 4)
    MaxIOWrites int    // Max concurrent writes (default: 4)
    Xattrs      bool   // Extended attributes support (default: true)
    Debug       bool   // Debug logging (default: false)
    Console     bool   // Log to console (default: false)
    LogFile     string // Log file path (default: ~/smb2-go.log)
}
```

### Methods

#### Serve()

```go
func (ms *MemoryShare) Serve() error
```

Starts the SMB server. This is a **blocking call** that runs until the server stops or encounters an error.

#### AddFile()

```go
func (ms *MemoryShare) AddFile(filepath string, content []byte) error
```

Adds a file with content to the memory filesystem with default permissions (0644).

**Example:**
```go
share.AddFile("/hello.txt", []byte("Hello, World!"))
```

#### AddFileWithMode()

```go
func (ms *MemoryShare) AddFileWithMode(filepath string, content []byte, mode uint32) error
```

Adds a file with custom Unix permissions.

**Example:**
```go
share.AddFileWithMode("/script.sh", []byte("#!/bin/bash\necho hello"), 0755)
```

#### AddDirectory()

```go
func (ms *MemoryShare) AddDirectory(dirpath string) error
```

Creates a directory in the memory filesystem with default permissions (0755).

**Example:**
```go
share.AddDirectory("/documents")
share.AddFile("/documents/file.txt", []byte("content"))
```

#### AddDirectoryWithMode()

```go
func (ms *MemoryShare) AddDirectoryWithMode(dirpath string, mode uint32) error
```

Creates a directory with custom Unix permissions.

**Example:**
```go
share.AddDirectoryWithMode("/private", 0700)
```

#### Shutdown()

```go
func (ms *MemoryShare) Shutdown() error
```

Gracefully stops the SMB server.

## Usage Examples

### Basic File Server

```go
share, _ := memoryshare.New(memoryshare.Options{
    Port:       445,
    Address:    "0.0.0.0",
    ShareName:  "Files",
    AllowGuest: true,
})

share.AddFile("/readme.txt", []byte("Welcome!"))
share.Serve()
```

### macOS Time Machine Compatible

```go
share, _ := memoryshare.New(memoryshare.Options{
    Port:       8082,
    Address:    "0.0.0.0",
    ShareName:  "TimeMachine",
    Hostname:   "MyBackupServer",
    Advertise:  true,  // Enable Bonjour
    Xattrs:     true,  // Extended attributes
})

share.Serve()
```

### Honeypot Server

```go
share, _ := memoryshare.New(memoryshare.Options{
    Port:       445,
    Address:    "0.0.0.0",
    ShareName:  "ADMIN$",
    AllowGuest: true,
    Debug:      true,  // Log all access
})

// Add fake files to attract attackers
share.AddDirectory("/Windows/System32")
share.AddFile("/Windows/System32/config.sys", []byte("fake"))
share.Serve()
```

### Development/Testing Server

```go
share, _ := memoryshare.New(memoryshare.Options{
    Port:       8082,
    Address:    "127.0.0.1",
    ShareName:  "TestShare",
    AllowGuest: true,
    Console:    true,  // Log to console
    Debug:      true,
})

// Populate with test data
for i := 1; i <= 100; i++ {
    share.AddFile(fmt.Sprintf("/file%d.txt", i), []byte(fmt.Sprintf("Test file %d", i)))
}

share.Serve()
```

## Connecting to the Server

### From macOS

```bash
# Using Finder
open smb://127.0.0.1:8082/MyShare

# Using command line
smbutil view //127.0.0.1:8082/MyShare
mount -t smbfs //guest@127.0.0.1:8082/MyShare /mnt/share
```

### From Linux

```bash
# Using smbclient
smbclient //127.0.0.1/MyShare -p 8082 -U guest

# Using mount
mount -t cifs //127.0.0.1/MyShare /mnt/share -o port=8082,guest
```

### From Windows

```powershell
# Map network drive
net use Z: \\127.0.0.1@8082\MyShare
```

## Notes

- All data is stored in memory and will be lost when the server stops
- Parent directories are created automatically when adding files
- The server supports SMB2/3 encryption and signing
- Guest access bypasses authentication (useful for testing)
- Bonjour advertisement helps macOS clients discover the server automatically

## License

Dual licensed: AGPL or proprietary commercial license
