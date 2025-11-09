package main

import (
	"fmt"
	"log"

	"github.com/pablolagos/smb-mem-lib/memoryshare"
)

// CustomLogger implements memoryshare.SimpleLogger
type CustomLogger struct {
	prefix string
}

func (l *CustomLogger) Printf(format string, args ...interface{}) {
	log.Printf("[%s] "+format, append([]interface{}{l.prefix}, args...)...)
}

func (l *CustomLogger) Print(args ...interface{}) {
	log.Print(append([]interface{}{fmt.Sprintf("[%s]", l.prefix)}, args...)...)
}

func main() {
	// Example 1: Using a custom logger
	fmt.Println("=== Example 1: Custom Logger ===")
	customLogger := &CustomLogger{prefix: "CUSTOM-SMB"}

	share1, err := memoryshare.New(memoryshare.Options{
		Port:       8445,
		Address:    "127.0.0.1",
		ShareName:  "CustomLoggerShare",
		Hostname:   "CustomSMB",
		AllowGuest: true,
		Logger:     customLogger, // Use custom logger
	})
	if err != nil {
		log.Fatalf("Failed to create MemoryShare: %v", err)
	}

	share1.AddFile("/test.txt", []byte("Test file"))
	fmt.Println("Created share with custom logger on port 8445")
	fmt.Println("All logs will be prefixed with [CUSTOM-SMB]")

	// Example 2: Disabling all logging with NoOpLogger
	fmt.Println("\n=== Example 2: No Logging (Silent Mode) ===")

	share2, err := memoryshare.New(memoryshare.Options{
		Port:       8446,
		Address:    "127.0.0.1",
		ShareName:  "SilentShare",
		Hostname:   "SilentSMB",
		AllowGuest: true,
		Logger:     memoryshare.NoOpLogger, // Disable all logging
	})
	if err != nil {
		log.Fatalf("Failed to create MemoryShare: %v", err)
	}

	share2.AddFile("/silent.txt", []byte("Silent file"))
	fmt.Println("Created share with no logging on port 8446")
	fmt.Println("This share will not produce any log output")

	// Example 3: Default logger (logrus)
	fmt.Println("\n=== Example 3: Default Logger (Logrus) ===")

	share3, err := memoryshare.New(memoryshare.Options{
		Port:       8447,
		Address:    "127.0.0.1",
		ShareName:  "DefaultShare",
		Hostname:   "DefaultSMB",
		AllowGuest: true,
		Console:    true,
		Debug:      false,
		// Logger not specified - will use default logrus
	})
	if err != nil {
		log.Fatalf("Failed to create MemoryShare: %v", err)
	}

	share3.AddFile("/default.txt", []byte("Default file"))
	fmt.Println("Created share with default logger on port 8447")
	fmt.Println("This share uses the default logrus logger")

	fmt.Println("\n=== Available Shares ===")
	fmt.Println("1. Custom Logger:  smbclient //127.0.0.1/CustomLoggerShare -p 8445 -N")
	fmt.Println("2. Silent Mode:    smbclient //127.0.0.1/SilentShare -p 8446 -N")
	fmt.Println("3. Default Logger: smbclient //127.0.0.1/DefaultShare -p 8447 -N")
	fmt.Println("\nStarting custom logger share (press Ctrl+C to stop)...")

	// Start the first share (you can start others in goroutines if needed)
	if err := share1.Serve(); err != nil {
		log.Fatalf("Server error: %v", err)
	}
}
