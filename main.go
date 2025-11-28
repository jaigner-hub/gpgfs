// GPGFS - A GPG-encrypted FUSE filesystem
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"github.com/hanwen/go-fuse/v2/fs"
	"github.com/hanwen/go-fuse/v2/fuse"
	"golang.org/x/term"

	gpgfs "gpgfs/fs"
	"gpgfs/storage"
)

func main() {
	var (
		mountPoint    string
		containerPath string
		debug         bool
	)

	flag.StringVar(&mountPoint, "mount", "", "Mount point for the filesystem")
	flag.StringVar(&containerPath, "container", "", "Path to encrypted container file")
	flag.BoolVar(&debug, "debug", false, "Enable debug logging")
	flag.Parse()

	if mountPoint == "" || containerPath == "" {
		fmt.Fprintf(os.Stderr, "Usage: %s -mount <mountpoint> -container <container_file>\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "\nOptions:\n")
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\nExample:\n")
		fmt.Fprintf(os.Stderr, "  %s -mount /mnt/encrypted -container ~/vault.gpgfs\n", os.Args[0])
		os.Exit(1)
	}

	// Read passphrase
	fmt.Print("Enter passphrase: ")
	passphrase, err := term.ReadPassword(int(syscall.Stdin))
	if err != nil {
		log.Fatalf("Failed to read passphrase: %v", err)
	}
	fmt.Println()

	// Initialize storage (opens or creates the container file)
	store, err := storage.NewStorage(containerPath, string(passphrase))
	if err != nil {
		log.Fatalf("Failed to open container: %v", err)
	}
	defer store.Close()

	// Create mount point if it doesn't exist
	if err := os.MkdirAll(mountPoint, 0755); err != nil {
		log.Fatalf("Failed to create mount point: %v", err)
	}

	// Create the filesystem
	root := gpgfs.NewGPGFS(store).Root()

	// Mount options
	opts := &fs.Options{
		MountOptions: fuse.MountOptions{
			FsName:        "gpgfs",
			Name:          "gpgfs",
			DisableXAttrs: true,
			Debug:         debug,
		},
	}

	// Mount the filesystem
	server, err := fs.Mount(mountPoint, root, opts)
	if err != nil {
		log.Fatalf("Failed to mount filesystem: %v", err)
	}

	fmt.Printf("GPGFS mounted at %s\n", mountPoint)
	fmt.Println("Press Ctrl+C to unmount and exit")

	// Handle signals for clean shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	var unmountOnce sync.Once
	go func() {
		<-sigChan
		// Stop receiving more signals
		signal.Stop(sigChan)

		unmountOnce.Do(func() {
			fmt.Println("\nUnmounting...")
			if err := server.Unmount(); err != nil {
				log.Printf("Warning: unmount error: %v", err)
				log.Println("Try: fusermount -uz", mountPoint)
			}
		})
	}()

	// Wait for the server to finish
	server.Wait()
	fmt.Println("Filesystem unmounted")
}
