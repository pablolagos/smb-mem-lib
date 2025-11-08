package main

import (
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/pablolagos/smb-mem-lib/memoryshare"
	"github.com/pablolagos/smb-mem-lib/vfs"
)

// FileWriteInterceptor demonstrates how to intercept file writes
// This is useful for honeypots, malware analysis, or security monitoring
func fileWriteInterceptor(ctx *vfs.OperationContext, handle vfs.VfsHandle, filename string, data []byte, offset uint64) vfs.WriteInterceptResult {
	// Log all write operations with client information
	log.Printf("[WRITE INTERCEPTOR] Client %s writing to %s (offset: %d, size: %d bytes)",
		ctx.RemoteAddr, filename, offset, len(data))

	// Log username if authenticated
	if ctx.Username != "" {
		log.Printf("[WRITE INTERCEPTOR] Authenticated as: %s", ctx.Username)
	}

	// Example: Block writes to sensitive files
	if strings.Contains(filename, "blocked") || strings.Contains(filename, "sensitive") {
		log.Printf("[WRITE INTERCEPTOR] ⛔ BLOCKED write to sensitive file: %s", filename)
		return vfs.WriteInterceptResult{
			Block: true, // Don't actually save the file
			Error: nil,  // Return success to client (honeypot behavior)
		}
	}

	// Example: Detect malicious patterns
	content := string(data)
	if strings.Contains(content, "malware") || strings.Contains(content, "ransomware") {
		log.Printf("[WRITE INTERCEPTOR] ⚠️  ALERT: Suspicious content detected!")
		log.Printf("[WRITE INTERCEPTOR] Content preview: %s", content[:min(100, len(content))])

		// Save suspicious content for analysis
		analysisFile := fmt.Sprintf("suspicious_%s_%d.bin", filename, offset)
		if err := os.WriteFile(analysisFile, data, 0644); err == nil {
			log.Printf("[WRITE INTERCEPTOR] Saved suspicious content to: %s", analysisFile)
		}

		// Allow the write but log it
		return vfs.WriteInterceptResult{
			Block: false,
			Error: nil,
		}
	}

	// Example: Detect executable files by magic bytes
	if len(data) >= 2 && data[0] == 'M' && data[1] == 'Z' {
		log.Printf("[WRITE INTERCEPTOR] 🔍 Executable (PE) file detected: %s", filename)
		// You could save this for malware analysis
	}

	// Example: Detect ransomware behavior (encrypting files)
	if strings.HasSuffix(filename, ".encrypted") || strings.HasSuffix(filename, ".locked") {
		log.Printf("[WRITE INTERCEPTOR] 🚨 RANSOMWARE PATTERN: File extension suggests encryption: %s", filename)
	}

	// Allow the write to proceed normally
	log.Printf("[WRITE INTERCEPTOR] ✅ Allowing write to: %s", filename)
	return vfs.WriteInterceptResult{
		Block: false,
		Error: nil,
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func main() {
	// Create in-memory SMB server in honeypot mode
	share, err := memoryshare.New(memoryshare.Options{
		Port:       8445,
		Address:    "127.0.0.1",
		ShareName:  "HoneypotShare",
		Hostname:   "HoneypotSMB",
		AllowGuest: true,  // Allow guest access for honeypot
		Advertise:  false, // Don't advertise (stealth mode)
		Debug:      false,
		Console:    true,

		// HONEYPOT MODE: Discard all write operations
		// Client thinks operations succeeded but nothing is actually saved
		// Perfect for deception - attackers won't know they're being monitored
		DiscardWrites: true,
	})
	if err != nil {
		log.Fatalf("Failed to create MemoryShare: %v", err)
	}

	// Add some sample files
	share.AddFile("/readme.txt", []byte("Welcome to the SMB honeypot!\nThis server monitors all file operations."))
	share.AddDirectory("/documents")
	share.AddFile("/documents/report.txt", []byte("Sample document"))
	share.AddDirectory("/blocked")
	share.AddFile("/blocked/sensitive.txt", []byte("This file cannot be modified"))

	// Register the write callback
	log.Println("🎯 Setting up write interceptor...")
	share.SetWriteCallback(fileWriteInterceptor)

	log.Println("🚀 SMB Server with write interceptor starting on 127.0.0.1:8445")
	log.Println("📝 All file writes will be logged and intercepted")
	log.Println("🕵️  HONEYPOT MODE: DiscardWrites is ENABLED")
	log.Println("   → All writes/renames/deletes will be discarded")
	log.Println("   → Callback will still be invoked to log activity")
	log.Println("   → Attackers will think operations succeeded")
	log.Println("\nTry connecting with:")
	log.Println("  smbclient //127.0.0.1/HoneypotShare -p 8445 -N  (guest)")
	log.Println("\nPress Ctrl+C to stop")

	// Start server (blocking call)
	if err := share.Serve(); err != nil {
		log.Fatalf("Server error: %v", err)
	}
}
