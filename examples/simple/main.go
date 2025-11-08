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
		Hostname:   "MyServer",
		AllowGuest: true,  // Allow connections without authentication
		Advertise:  true,  // Enable Bonjour/mDNS for macOS discovery
		Debug:      false, // Set to true for debug logging
		Console:    true,  // Log to console instead of file
	})
	if err != nil {
		log.Fatalf("Failed to create MemoryShare: %v", err)
	}

	// Add files to the in-memory filesystem
	share.AddFile("/welcome.txt", []byte("Welcome to the SMB server!\n"))
	share.AddFile("/readme.md", []byte("# MemoryShare\n\nThis is an in-memory SMB2/3 server.\n"))

	// Create directories
	share.AddDirectory("/documents")
	share.AddDirectory("/data")

	// Add files to directories
	share.AddFile("/documents/file1.txt", []byte("Content of file 1\n"))
	share.AddFile("/documents/file2.txt", []byte("Content of file 2\n"))
	share.AddFile("/data/config.json", []byte(`{"setting": "value"}`))

	log.Println("MemoryShare server initialized")
	log.Printf("Connect to: smb://%s:%d/%s", "127.0.0.1", 8082, "MyShare")

	// Start the server (blocking call)
	if err := share.Serve(); err != nil {
		log.Fatalf("Server error: %v", err)
	}
}
