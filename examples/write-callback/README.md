# Write Callback & DiscardWrites Example

This example demonstrates two powerful honeypot features:

1. **Write Callbacks** - Intercept and control file write operations
2. **DiscardWrites Mode** - Silently discard all modifications while appearing successful

Perfect for:
- **Honeypot implementations** - Monitor and log malicious activity without risk
- **Malware analysis** - Detect and save suspicious files safely
- **Security monitoring** - Track attacker behavior in a controlled environment
- **Deception technology** - Make attackers think they're succeeding

## Features Demonstrated

### 1. DiscardWrites Mode (Honeypot Mode)

When `DiscardWrites: true` is enabled, **ALL** write operations are silently discarded:
- ✅ File writes → Discarded (client sees success)
- ✅ File renames → Discarded (client sees success)
- ✅ File deletions → Discarded (client sees success)
- ✅ File truncations → Discarded (client sees success)
- ✅ Extended attributes → Discarded (client sees success)

**Key Benefits:**
- **Complete Deception**: Attackers believe their actions are working
- **Zero Risk**: No actual files are modified on your system
- **Full Logging**: Callbacks still fire so you capture all activity
- **Perfect for Honeypots**: Monitor attackers without letting them cause damage

```go
share, _ := memoryshare.New(memoryshare.Options{
    DiscardWrites: true,  // Enable honeypot mode
    AllowGuest: true,     // Allow anonymous access
})
```

### 2. Write Interception with Callbacks
Every write operation calls your callback function **before** the data is saved, allowing you to:
- Inspect the content being written
- Know who is writing (IP address, username)
- Block or allow the write
- Save copies for analysis

### 2. Context Information
The callback receives rich context about the operation:
```go
type OperationContext struct {
    RemoteAddr string   // Client IP address
    LocalAddr  string   // Server IP address
    SessionID  uint64   // SMB session ID
    Username   string   // Authenticated username (if available)
}
```

### 3. Blocking Capabilities
Control the write operation by returning:
```go
type WriteInterceptResult struct {
    Block bool   // If true, prevents the write (but can still return success to client)
    Error error  // If set, returns this error to the client
}
```

## Usage

### 1. Define Your Callback Function
```go
func myWriteCallback(
    ctx *vfs.OperationContext,
    handle vfs.VfsHandle,
    filename string,
    data []byte,
    offset uint64,
) vfs.WriteInterceptResult {
    // Your logic here
    log.Printf("Client %s writing %d bytes to %s",
        ctx.RemoteAddr, len(data), filename)

    // Block writes to sensitive files
    if strings.Contains(filename, "blocked") {
        return vfs.WriteInterceptResult{
            Block: true,  // Don't save
            Error: nil,   // Return success (honeypot behavior)
        }
    }

    // Allow the write
    return vfs.WriteInterceptResult{
        Block: false,
        Error: nil,
    }
}
```

### 2. Register the Callback
```go
fs := memoryshare.NewMemoryFS()
fs.SetWriteCallback(myWriteCallback)
```

### 3. Use the Filesystem with Server
```go
srv := server.NewServer(cfg, auth, map[string]vfs.VFSFileSystem{
    "SHARE": fs,
})
```

## Example Scenarios

### Honeypot Behavior
Block malicious writes but return success to deceive attackers:
```go
if isMalicious(data) {
    log.Printf("ALERT: Malicious activity from %s", ctx.RemoteAddr)
    return vfs.WriteInterceptResult{
        Block: true,  // Don't actually save the file
        Error: nil,   // But tell the attacker it worked
    }
}
```

### Malware Analysis
Save suspicious files for later analysis:
```go
if isExecutable(data) || isSuspicious(data) {
    filename := fmt.Sprintf("captured_%s_%d.bin", filename, time.Now().Unix())
    os.WriteFile(filename, data, 0644)
    log.Printf("Captured suspicious file: %s", filename)
}
```

### Content Filtering
Detect ransomware patterns:
```go
if strings.HasSuffix(filename, ".encrypted") {
    log.Printf("RANSOMWARE ALERT: File encryption detected!")
    // Send alert, block operation, etc.
}
```

## Running the Example

```bash
cd examples/write-callback
go run main.go
```

Then connect with an SMB client:
```bash
# With authentication
smbclient //127.0.0.1/SHARE -p 8445 -U testuser%password123

# As guest
smbclient //127.0.0.1/SHARE -p 8445 -N
```

## Try These Commands

Once connected, try these operations to see the callback in action:

```bash
# See callback for normal write
put localfile.txt remotefile.txt

# See callback block sensitive files
put test.txt /blocked/test.txt

# Create file with malicious pattern
echo "malware detected" > test.txt
put test.txt malware.txt

# Try ransomware pattern
echo "encrypted" > test.encrypted
put test.encrypted encrypted_file.txt
```

## Implementation Notes

- The callback runs in the same goroutine as the write operation
- For high-throughput scenarios, consider async logging
- The callback receives a **copy** of the data, so modifying it won't affect the write
- Context is automatically cleaned up after each operation
- Thread-safe: multiple concurrent writes are handled correctly

## Security Considerations

- Callback errors are logged but won't crash the server
- Blocking writes still consumes bandwidth (client sends the data)
- Consider rate limiting if used for DoS protection
- Save captured files to secure storage with access controls

## See Also

- `vfs/vfs.go` - Full VFS interface documentation
- `memoryshare/memory_fs.go` - MemoryFS implementation
- `server/file_tree.go` - Server-side write handling
