package main

import (
	"os"
	"os/signal"

	"github.com/macos-fuse-t/go-smb2/bonjour"
	"github.com/macos-fuse-t/go-smb2/config"
	smb2 "github.com/macos-fuse-t/go-smb2/server"
	"github.com/macos-fuse-t/go-smb2/vfs"

	log "github.com/sirupsen/logrus"
	"gopkg.in/natefinch/lumberjack.v2"
)

var cfg config.AppConfig

func main() {

	homeDir, _ := os.UserHomeDir()
	cfg = config.NewConfig([]string{
		"smb2.ini",
		homeDir + "/.fuse-t/smb2.ini",
	})
	initLogs()

	// Create in-memory filesystem with sample documents
	memFS := NewMemoryFS()

	// Add sample documents
	memFS.AddFile("/README.txt", []byte("Bienvenido al servidor SMB en memoria!\n\nEste es un sistema de archivos completamente en memoria.\n"), 0644)
	memFS.AddFile("/document1.txt", []byte("Este es el contenido del primer documento.\nPuedes leer y escribir en estos archivos.\n"), 0644)
	memFS.AddFile("/document2.txt", []byte("Contenido del segundo documento embebido en memoria.\n"), 0644)

	// Create a directory with files
	memFS.AddDirectory("/docs", 0755)
	memFS.AddFile("/docs/ejemplo1.txt", []byte("Documento de ejemplo en el subdirectorio docs.\n"), 0644)
	memFS.AddFile("/docs/ejemplo2.txt", []byte("Otro documento en el subdirectorio.\n"), 0644)

	// Create another directory
	memFS.AddDirectory("/data", 0755)
	memFS.AddFile("/data/info.txt", []byte("Información almacenada en memoria.\n"), 0644)

	log.Infof("Sistema de archivos en memoria inicializado con documentos de ejemplo")

	srv := smb2.NewServer(
		&smb2.ServerConfig{
			AllowGuest:  cfg.AllowGuest,
			MaxIOReads:  cfg.MaxIOReads,
			MaxIOWrites: cfg.MaxIOWrites,
			Xatrrs:      cfg.Xatrrs,
		},
		&smb2.NTLMAuthenticator{
			TargetSPN:    "",
			NbDomain:     cfg.Hostname,
			NbName:       cfg.Hostname,
			DnsName:      cfg.Hostname + ".local",
			DnsDomain:    ".local",
			UserPassword: map[string]string{"a": "a"},
			AllowGuest:   cfg.AllowGuest,
		},
		map[string]vfs.VFSFileSystem{cfg.ShareName: memFS},
	)

	log.Infof("Starting server at %s", cfg.ListenAddr)
	go srv.Serve(cfg.ListenAddr)
	if cfg.Advertise {
		go bonjour.Advertise(cfg.ListenAddr, cfg.Hostname, cfg.Hostname, cfg.ShareName, true)
	}

	//go stats.StatServer(":9092")

	waitSignal()
}

func initLogs() {
	log.Infof("debug level %v", cfg.Debug)
	if cfg.Debug {
		log.SetLevel(log.DebugLevel)
	} else {
		log.SetLevel(log.InfoLevel)
	}

	homeDir, _ := os.UserHomeDir()
	if !cfg.Console {
		log.SetOutput(&lumberjack.Logger{
			Filename:   homeDir + "smb2-go.log",
			MaxSize:    100, // megabytes
			MaxBackups: 3,
			MaxAge:     28,   //days
			Compress:   true, // disabled by default
		})
	} else {
		log.SetOutput(os.Stdout)
	}
}

func waitSignal() {
	// Ctrl+C handling
	handler := make(chan os.Signal, 1)
	signal.Notify(handler, os.Interrupt)
	for sig := range handler {
		if sig == os.Interrupt {
			bonjour.Shutdown()
			break
		}
	}
}
