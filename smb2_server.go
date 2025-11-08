package main

import (
	"os"
	"strconv"
	"strings"

	"github.com/pablolagos/smb-mem-lib/config"
	"github.com/pablolagos/smb-mem-lib/memoryshare"

	log "github.com/sirupsen/logrus"
)

func main() {
	homeDir, _ := os.UserHomeDir()
	cfg := config.NewConfig([]string{
		"smb2.ini",
		homeDir + "/.fuse-t/smb2.ini",
	})

	// Extract port from listen address
	parts := strings.Split(cfg.ListenAddr, ":")
	port := 8082
	address := "127.0.0.1"
	if len(parts) == 2 {
		address = parts[0]
		if p, err := strconv.Atoi(parts[1]); err == nil {
			port = p
		}
	}

	// Create MemoryShare server using the new library
	share, err := memoryshare.New(memoryshare.Options{
		Port:        port,
		Address:     address,
		ShareName:   cfg.ShareName,
		Hostname:    cfg.Hostname,
		AllowGuest:  cfg.AllowGuest,
		Advertise:   cfg.Advertise,
		MaxIOReads:  cfg.MaxIOReads,
		MaxIOWrites: cfg.MaxIOWrites,
		Xattrs:      cfg.Xatrrs,
		Debug:       cfg.Debug,
		Console:     cfg.Console,
	})
	if err != nil {
		log.Fatalf("Failed to create MemoryShare: %v", err)
	}

	// Add sample documents
	share.AddFile("/README.txt", []byte("Bienvenido al servidor SMB en memoria!\n\nEste es un sistema de archivos completamente en memoria.\n"))
	share.AddFile("/document1.txt", []byte("Este es el contenido del primer documento.\nPuedes leer y escribir en estos archivos.\n"))
	share.AddFile("/document2.txt", []byte("Contenido del segundo documento embebido en memoria.\n"))

	// Create a directory with files
	share.AddDirectory("/docs")
	share.AddFile("/docs/ejemplo1.txt", []byte("Documento de ejemplo en el subdirectorio docs.\n"))
	share.AddFile("/docs/ejemplo2.txt", []byte("Otro documento en el subdirectorio.\n"))

	// Create another directory
	share.AddDirectory("/data")
	share.AddFile("/data/info.txt", []byte("Información almacenada en memoria.\n"))

	log.Infof("Sistema de archivos en memoria inicializado con documentos de ejemplo")

	// Start server (blocking)
	if err := share.Serve(); err != nil {
		log.Fatalf("Server error: %v", err)
	}
}
