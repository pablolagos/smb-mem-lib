# Testing Guide

## Running Tests

### Run all tests
```bash
cd memoryshare
go test -v
```

### Run specific test
```bash
go test -v -run TestAddFile
go test -v -run TestMemoryFS_VFS_Operations
```

### Run with coverage
```bash
go test -v -cover
go test -coverprofile=coverage.out
go tool cover -html=coverage.out
```

### Run benchmarks
```bash
go test -bench=.
go test -bench=BenchmarkAddFile -benchmem
```

### Run benchmarks with profiling
```bash
go test -bench=. -cpuprofile=cpu.prof
go tool pprof cpu.prof
```

## Test Structure

### memoryshare_test.go
Tests for the high-level MemoryShare API:
- `TestNew` - Server creation with various options
- `TestAddFile` - Adding files to the filesystem
- `TestAddDirectory` - Creating directories
- `TestAddFileWithMode` - Custom file permissions
- `TestMemoryFSOperations` - Basic filesystem operations
- `TestAutoCreateParentDirectories` - Auto-creation of parent dirs
- `TestOptionsDefaults` - Default option values

### memory_fs_test.go
Tests for the low-level VFS operations:
- `TestMemoryFS_VFS_Operations` - Open, Read, Close, GetAttr
- `TestMemoryFS_Write` - Writing and appending to files
- `TestMemoryFS_Directory_Operations` - Directory listing
- `TestMemoryFS_Mkdir` - Creating directories via VFS
- `TestMemoryFS_Truncate` - File truncation
- `TestMemoryFS_Rename` - Renaming files
- `TestMemoryFS_Unlink` - Deleting files
- `TestMemoryFS_Lookup` - Finding files in directories
- `TestMemoryFS_ExtendedAttributes` - Xattr operations
- `TestMemoryFS_SetAttr` - Modifying file attributes
- `TestMemoryFS_StatFS` - Filesystem statistics
- `TestMemoryFS_ConcurrentAccess` - Thread-safety

## Benchmarks

Available benchmarks:
- `BenchmarkAddFile` - Adding a single file
- `BenchmarkAddManyFiles` - Adding 100 files
- `BenchmarkResolvePathDeep` - Resolving deeply nested paths
- `BenchmarkMemoryFS_Open` - Opening files
- `BenchmarkMemoryFS_Read` - Reading files
- `BenchmarkMemoryFS_Write` - Writing files

Example benchmark output:
```
BenchmarkAddFile-8              500000      3245 ns/op
BenchmarkMemoryFS_Read-8       2000000       756 ns/op
BenchmarkMemoryFS_Write-8      1000000      1234 ns/op
```

## Coverage Goals

Current test coverage areas:
- ✅ Server creation and initialization
- ✅ File operations (add, read, write, delete)
- ✅ Directory operations (create, list)
- ✅ VFS interface compliance
- ✅ Extended attributes
- ✅ File permissions
- ✅ Concurrent access
- ✅ Error handling

## Test Data

Tests use minimal in-memory data:
- Small text files (< 1KB)
- Simple directory structures
- No external dependencies
- Fast execution (< 1 second for all tests)

## Continuous Integration

Recommended CI commands:
```bash
# Lint
go vet ./...
golangci-lint run

# Test with race detector
go test -race -v ./...

# Test with coverage
go test -coverprofile=coverage.out ./...
go tool cover -func=coverage.out

# Benchmarks
go test -bench=. -benchmem
```

## Manual Testing

To manually test the server:

```bash
# Terminal 1 - Start server
cd examples/simple
go run main.go

# Terminal 2 - Connect with smbclient
smbclient //127.0.0.1/MyShare -p 8082 -U guest

# Try commands
smb: \> ls
smb: \> get welcome.txt
smb: \> put local_file.txt
smb: \> quit
```

## Debugging Tests

Enable debug logging in tests:
```go
share, err := memoryshare.New(memoryshare.Options{
    Debug:   true,
    Console: true,  // Log to stdout
})
```

View test output:
```bash
go test -v 2>&1 | tee test.log
```

## Known Limitations

Tests do **not** cover:
- Actual SMB protocol negotiation (requires client)
- Network transmission
- Authentication flows
- Encryption/signing
- Durable handles
- Oplocks/leases
- Multiple concurrent connections

These require integration tests with actual SMB clients.
