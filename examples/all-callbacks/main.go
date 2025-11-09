package main

import (
	"log"
	"strings"

	"github.com/pablolagos/smb-mem-lib/memoryshare"
	"github.com/pablolagos/smb-mem-lib/vfs"
)

// FileCreateInterceptor demonstrates how to intercept file/directory creation
func fileCreateInterceptor(ctx *vfs.OperationContext, filepath string, isDir bool, mode int) vfs.CreateInterceptResult {
	fileType := "file"
	if isDir {
		fileType = "directory"
	}

	// Log all create operations with client information
	log.Printf("[CREATE INTERCEPTOR] Client %s creating %s: %s (mode: %o)",
		ctx.RemoteAddr, fileType, filepath, mode)

	// Log username if authenticated
	if ctx.Username != "" {
		log.Printf("[CREATE INTERCEPTOR] Authenticated as: %s", ctx.Username)
	}

	// Example: Block creation of files/dirs with "blocked" in the name
	if strings.Contains(filepath, "blocked") {
		log.Printf("[CREATE INTERCEPTOR] ⛔ BLOCKED creation of: %s", filepath)
		return vfs.CreateInterceptResult{
			Block: true, // Don't actually create
			Error: nil,  // Return success to client (honeypot behavior)
		}
	}

	// Example: Detect ransomware patterns
	if strings.HasSuffix(filepath, ".encrypted") || strings.HasSuffix(filepath, ".locked") {
		log.Printf("[CREATE INTERCEPTOR] 🚨 RANSOMWARE PATTERN: Suspicious file extension: %s", filepath)
	}

	// Example: Monitor executable creation
	if strings.HasSuffix(filepath, ".exe") || strings.HasSuffix(filepath, ".dll") {
		log.Printf("[CREATE INTERCEPTOR] 🔍 Executable file created: %s", filepath)
	}

	// Allow the creation to proceed normally
	log.Printf("[CREATE INTERCEPTOR] ✅ Allowing creation of: %s", filepath)
	return vfs.CreateInterceptResult{
		Block: false,
		Error: nil,
	}
}

// FileRenameInterceptor demonstrates how to intercept file/directory rename/move operations
func fileRenameInterceptor(ctx *vfs.OperationContext, fromPath string, toPath string) vfs.RenameInterceptResult {
	// Log all rename operations with client information
	log.Printf("[RENAME INTERCEPTOR] Client %s renaming: %s -> %s",
		ctx.RemoteAddr, fromPath, toPath)

	// Log username if authenticated
	if ctx.Username != "" {
		log.Printf("[RENAME INTERCEPTOR] Authenticated as: %s", ctx.Username)
	}

	// Example: Detect ransomware behavior (renaming to encrypted extensions)
	if strings.HasSuffix(toPath, ".encrypted") || strings.HasSuffix(toPath, ".locked") || strings.HasSuffix(toPath, ".crypto") {
		log.Printf("[RENAME INTERCEPTOR] 🚨 RANSOMWARE ALERT: File being renamed to encrypted extension!")
		log.Printf("[RENAME INTERCEPTOR] Original: %s -> New: %s", fromPath, toPath)
		// You could trigger alerts, block the operation, etc.
	}

	// Example: Block renames to sensitive directories
	if strings.Contains(toPath, "/protected/") || strings.Contains(toPath, "/sensitive/") {
		log.Printf("[RENAME INTERCEPTOR] ⛔ BLOCKED rename to protected directory: %s", toPath)
		return vfs.RenameInterceptResult{
			Block: true, // Don't actually rename
			Error: nil,  // Return success to client (honeypot behavior)
		}
	}

	// Example: Detect mass renaming (potential ransomware)
	// In a real implementation, you would track rename operations over time
	if strings.HasSuffix(fromPath, ".doc") && !strings.HasSuffix(toPath, ".doc") {
		log.Printf("[RENAME INTERCEPTOR] ⚠️  Document file extension changed: %s -> %s", fromPath, toPath)
	}

	// Allow the rename to proceed normally
	log.Printf("[RENAME INTERCEPTOR] ✅ Allowing rename: %s -> %s", fromPath, toPath)
	return vfs.RenameInterceptResult{
		Block: false,
		Error: nil,
	}
}

// FileWriteInterceptor demonstrates how to intercept file writes
func fileWriteInterceptor(ctx *vfs.OperationContext, handle vfs.VfsHandle, filename string, data []byte, offset uint64) vfs.WriteInterceptResult {
	// Log write operations
	log.Printf("[WRITE INTERCEPTOR] Client %s writing to %s (offset: %d, size: %d bytes)",
		ctx.RemoteAddr, filename, offset, len(data))

	// Log username if authenticated
	if ctx.Username != "" {
		log.Printf("[WRITE INTERCEPTOR] Authenticated as: %s", ctx.Username)
	}

	// Example: Detect executable files by magic bytes
	if len(data) >= 2 && data[0] == 'M' && data[1] == 'Z' {
		log.Printf("[WRITE INTERCEPTOR] 🔍 Executable (PE) file detected: %s", filename)
	}

	// Allow the write to proceed normally
	log.Printf("[WRITE INTERCEPTOR] ✅ Allowing write to: %s", filename)
	return vfs.WriteInterceptResult{
		Block: false,
		Error: nil,
	}
}

func main() {
	// Create in-memory SMB server with all callbacks enabled
	share, err := memoryshare.New(memoryshare.Options{
		Port:       8445,
		Address:    "127.0.0.1",
		ShareName:  "MonitoredShare",
		Hostname:   "MonitorSMB",
		AllowGuest: true,
		Advertise:  false,
		Debug:      false,
		Console:    true,

		// OPTIONAL: Enable honeypot mode to discard all changes
		// DiscardWrites: true,
	})
	if err != nil {
		log.Fatalf("Failed to create MemoryShare: %v", err)
	}

	// Add some sample files and directories
	share.AddDirectory("/documents")
	share.AddFile("/documents/report.txt", []byte("Sample document"))
	share.AddFile("/documents/financial.xlsx", []byte("Financial data"))

	share.AddDirectory("/protected")
	share.AddFile("/protected/sensitive.txt", []byte("Sensitive data"))

	share.AddDirectory("/temp")

	// Register all three callbacks
	log.Println("🎯 Setting up file operation interceptors...")
	share.SetCreateCallback(fileCreateInterceptor)
	share.SetRenameCallback(fileRenameInterceptor)
	share.SetWriteCallback(fileWriteInterceptor)

	log.Println("🚀 SMB Server with full monitoring starting on 127.0.0.1:8445")
	log.Println("📝 All file operations will be intercepted and logged:")
	log.Println("   ✓ File/directory creation (onCreate)")
	log.Println("   ✓ File/directory rename/move (onRename)")
	log.Println("   ✓ File writes (onWrite)")
	log.Println()
	log.Println("🔍 Monitored behaviors:")
	log.Println("   • Ransomware patterns (encrypted extensions, mass renames)")
	log.Println("   • Executable file creation/upload")
	log.Println("   • Access to protected directories")
	log.Println()
	log.Println("Try connecting with:")
	log.Println("  smbclient //127.0.0.1/MonitoredShare -p 8445 -N")
	log.Println()
	log.Println("Example operations to test:")
	log.Println("  put myfile.txt             (triggers onCreate + onWrite)")
	log.Println("  mkdir testdir              (triggers onCreate)")
	log.Println("  rename report.txt old.txt  (triggers onRename)")
	log.Println("  put malware.exe            (triggers onCreate + onWrite with PE detection)")
	log.Println()
	log.Println("Press Ctrl+C to stop")

	// Start server (blocking call)
	if err := share.Serve(); err != nil {
		log.Fatalf("Server error: %v", err)
	}
}
