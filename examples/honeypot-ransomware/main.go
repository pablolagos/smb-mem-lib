package main

import (
	"log"
	"strings"
	"sync"

	"github.com/pablolagos/smb-mem-lib/memoryshare"
	"github.com/pablolagos/smb-mem-lib/vfs"
)

// RansomwareDetector tracks suspicious file operations
type RansomwareDetector struct {
	mu                  sync.Mutex
	encryptedExtensions map[string]int  // Track files renamed to encrypted extensions
	rapidRenames        map[string]int  // Track rapid renames from same IP
	suspiciousIPs       map[string]bool // IPs showing ransomware behavior
	fileCreations       map[string]int  // Track file creations by IP
}

func NewRansomwareDetector() *RansomwareDetector {
	return &RansomwareDetector{
		encryptedExtensions: make(map[string]int),
		rapidRenames:        make(map[string]int),
		suspiciousIPs:       make(map[string]bool),
		fileCreations:       make(map[string]int),
	}
}

func (rd *RansomwareDetector) OnCreate(ctx *vfs.OperationContext, filepath string, isDir bool, mode int) vfs.CreateInterceptResult {
	rd.mu.Lock()
	defer rd.mu.Unlock()

	fileType := "file"
	if isDir {
		fileType = "directory"
	}

	log.Printf("[CREATE] %s creating %s: %s (user: %s)",
		ctx.RemoteAddr, fileType, filepath, ctx.Username)

	// Track file creation rate
	rd.fileCreations[ctx.RemoteAddr]++

	// Detect ransomware note files
	lowerPath := strings.ToLower(filepath)
	if strings.Contains(lowerPath, "readme") ||
		strings.Contains(lowerPath, "decrypt") ||
		strings.Contains(lowerPath, "ransom") ||
		strings.Contains(lowerPath, "recover") {
		log.Printf("🚨 [ALERT] Potential ransomware note detected: %s from %s", filepath, ctx.RemoteAddr)
		rd.suspiciousIPs[ctx.RemoteAddr] = true
	}

	// Detect executable creation in suspicious locations
	if strings.HasSuffix(lowerPath, ".exe") ||
		strings.HasSuffix(lowerPath, ".dll") ||
		strings.HasSuffix(lowerPath, ".bat") {
		log.Printf("⚠️  [WARNING] Executable created: %s from %s", filepath, ctx.RemoteAddr)
	}

	// Allow operation (honeypot will discard it)
	return vfs.CreateInterceptResult{
		Block: false,
		Error: nil,
	}
}

func (rd *RansomwareDetector) OnRename(ctx *vfs.OperationContext, fromPath string, toPath string) vfs.RenameInterceptResult {
	rd.mu.Lock()
	defer rd.mu.Unlock()

	log.Printf("[RENAME] %s renaming: %s -> %s (user: %s)",
		ctx.RemoteAddr, fromPath, toPath, ctx.Username)

	// Track rename rate per IP
	rd.rapidRenames[ctx.RemoteAddr]++

	// Check for ransomware encryption patterns
	fromExt := getExtension(fromPath)
	toExt := getExtension(toPath)

	// Detect common ransomware encrypted extensions
	ransomwareExtensions := []string{
		".encrypted", ".locked", ".crypto", ".crypt",
		".enc", ".lock", ".crypted", ".cerber",
		".locky", ".zepto", ".thor", ".aaa",
		".abc", ".xyz", ".zzz", ".micro",
		".encrypted", ".weapologize", ".wcry",
	}

	for _, ext := range ransomwareExtensions {
		if strings.HasSuffix(strings.ToLower(toPath), ext) {
			rd.encryptedExtensions[toExt]++
			log.Printf("🚨🚨🚨 [RANSOMWARE DETECTED] File encrypted: %s -> %s from IP %s",
				fromPath, toPath, ctx.RemoteAddr)
			log.Printf("    Original extension: %s, New extension: %s", fromExt, toExt)
			rd.suspiciousIPs[ctx.RemoteAddr] = true

			// Check if this IP is showing mass encryption behavior
			if rd.rapidRenames[ctx.RemoteAddr] > 5 {
				log.Printf("🚨🚨🚨 [MASS ENCRYPTION DETECTED] IP %s has renamed %d files!",
					ctx.RemoteAddr, rd.rapidRenames[ctx.RemoteAddr])
			}
			break
		}
	}

	// Detect document files being renamed to random extensions
	if isDocumentFile(fromPath) && toExt != fromExt && len(toExt) > 4 {
		log.Printf("⚠️  [SUSPICIOUS] Document file extension changed: %s -> %s",
			fromPath, toPath)
		rd.suspiciousIPs[ctx.RemoteAddr] = true
	}

	// Allow operation (honeypot will discard it)
	return vfs.RenameInterceptResult{
		Block: false,
		Error: nil,
	}
}

func (rd *RansomwareDetector) OnWrite(ctx *vfs.OperationContext, handle vfs.VfsHandle, filename string, data []byte, offset uint64) vfs.WriteInterceptResult {
	// Detect PE executables
	if len(data) >= 2 && data[0] == 'M' && data[1] == 'Z' {
		log.Printf("🔍 [EXECUTABLE] PE file detected: %s from %s", filename, ctx.RemoteAddr)
	}

	// Allow write (honeypot will discard it)
	return vfs.WriteInterceptResult{
		Block: false,
		Error: nil,
	}
}

func (rd *RansomwareDetector) PrintStats() {
	rd.mu.Lock()
	defer rd.mu.Unlock()

	log.Println("\n" + strings.Repeat("=", 70))
	log.Println("RANSOMWARE DETECTOR STATISTICS")
	log.Println(strings.Repeat("=", 70))

	if len(rd.suspiciousIPs) > 0 {
		log.Println("\n🚨 SUSPICIOUS IPs:")
		for ip := range rd.suspiciousIPs {
			log.Printf("  - %s (renames: %d, creates: %d)",
				ip, rd.rapidRenames[ip], rd.fileCreations[ip])
		}
	}

	if len(rd.encryptedExtensions) > 0 {
		log.Println("\n🔒 ENCRYPTED FILE EXTENSIONS DETECTED:")
		for ext, count := range rd.encryptedExtensions {
			log.Printf("  - %s: %d files", ext, count)
		}
	}

	if len(rd.rapidRenames) > 0 {
		log.Println("\n📊 RENAME ACTIVITY BY IP:")
		for ip, count := range rd.rapidRenames {
			status := "Normal"
			if count > 10 {
				status = "🚨 SUSPICIOUS"
			} else if count > 5 {
				status = "⚠️  Warning"
			}
			log.Printf("  - %s: %d renames [%s]", ip, count, status)
		}
	}

	log.Println(strings.Repeat("=", 70) + "\n")
}

func getExtension(path string) string {
	parts := strings.Split(path, ".")
	if len(parts) > 1 {
		return "." + parts[len(parts)-1]
	}
	return ""
}

func isDocumentFile(path string) bool {
	lowerPath := strings.ToLower(path)
	documentExtensions := []string{".doc", ".docx", ".xls", ".xlsx", ".ppt", ".pptx", ".pdf", ".txt", ".odt", ".ods"}
	for _, ext := range documentExtensions {
		if strings.HasSuffix(lowerPath, ext) {
			return true
		}
	}
	return false
}

func main() {
	detector := NewRansomwareDetector()

	// Create honeypot share with ransomware detection
	share, err := memoryshare.New(memoryshare.Options{
		Port:       8445,
		Address:    "127.0.0.1",
		ShareName:  "HoneypotShare",
		Hostname:   "FileServer",
		AllowGuest: true,
		Advertise:  false,
		Debug:      false,
		Console:    true,

		// 🍯 HONEYPOT MODE: All operations are discarded but logged
		DiscardWrites: true,
	})
	if err != nil {
		log.Fatalf("Failed to create MemoryShare: %v", err)
	}

	// Add realistic bait files to attract ransomware
	log.Println("Setting up honeypot with bait files...")
	share.AddDirectory("/Documents")
	share.AddFile("/Documents/Financial_Report_2024.xlsx", []byte("Q1 Revenue: $1.2M\nQ2 Revenue: $1.5M"))
	share.AddFile("/Documents/Customer_Database.xlsx", []byte("CustomerID,Name,Email\n1,John Doe,john@example.com"))
	share.AddFile("/Documents/Contract_Template.docx", []byte("CONFIDENTIAL CONTRACT"))
	share.AddFile("/Documents/Budget_2024.xlsx", []byte("Department,Budget\nIT,$500k\nSales,$800k"))

	share.AddDirectory("/Projects")
	share.AddFile("/Projects/Source_Code.txt", []byte("// Application source code"))
	share.AddFile("/Projects/Database_Backup.sql", []byte("-- Database backup"))

	share.AddDirectory("/HR")
	share.AddFile("/HR/Employee_Salaries.xlsx", []byte("Name,Salary\nAlice,$80k\nBob,$75k"))
	share.AddFile("/HR/Performance_Reviews.docx", []byte("2024 Performance Reviews"))

	// Register all three callbacks for comprehensive monitoring
	share.SetCreateCallback(detector.OnCreate)
	share.SetRenameCallback(detector.OnRename)
	share.SetWriteCallback(detector.OnWrite)

	log.Println(strings.Repeat("=", 70))
	log.Println("🍯 RANSOMWARE HONEYPOT ACTIVE")
	log.Println(strings.Repeat("=", 70))
	log.Println("Server: 127.0.0.1:8445")
	log.Println("Share: HoneypotShare")
	log.Println("Mode: HONEYPOT (all operations discarded)")
	log.Println()
	log.Println("Monitoring for:")
	log.Println("  ✓ File creation (onCreate)")
	log.Println("  ✓ File rename/encryption (onRename)")
	log.Println("  ✓ File writes (onWrite)")
	log.Println()
	log.Println("Bait files planted:")
	log.Println("  • Financial_Report_2024.xlsx")
	log.Println("  • Customer_Database.xlsx")
	log.Println("  • Employee_Salaries.xlsx")
	log.Println("  • And more...")
	log.Println()
	log.Println("Connect with:")
	log.Println("  smbclient //127.0.0.1/HoneypotShare -p 8445 -N")
	log.Println()
	log.Println("Test ransomware simulation:")
	log.Println("  1. put copy.file")
	log.Println("  2. rename copy.file file.doc.encrypted")
	log.Println("  3. put README_DECRYPT.txt")
	log.Println()
	log.Println("Press Ctrl+C to stop and view statistics")
	log.Println(strings.Repeat("=", 70))

	// Serve (blocking)
	if err := share.Serve(); err != nil {
		log.Printf("Server stopped: %v", err)
	}

	// Print final statistics
	detector.PrintStats()
}
