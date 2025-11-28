// GPGFS - A GPG-encrypted FUSE filesystem
package main

import (
	"bufio"
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"strings"
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
		mountPoint      string
		containerPath   string
		debug           bool
		passphraseStdin bool
		daemonize       bool
		allowOther      bool
		childProcess    bool
	)

	flag.StringVar(&mountPoint, "mount", "", "Mount point for the filesystem")
	flag.StringVar(&containerPath, "container", "", "Path to encrypted container file")
	flag.BoolVar(&debug, "debug", false, "Enable debug logging")
	flag.BoolVar(&passphraseStdin, "passphrase-stdin", false, "Read passphrase from stdin (no prompt)")
	flag.BoolVar(&daemonize, "daemonize", false, "Fork to background after mounting")
	flag.BoolVar(&allowOther, "allow-other", false, "Allow other users to access the mount")
	flag.BoolVar(&childProcess, "child", false, "Internal flag: indicates this is a daemonized child process")
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
	var passphrase []byte
	var err error
	if passphraseStdin {
		// Read from stdin without prompt (for PAM/scripted use)
		reader := bufio.NewReader(os.Stdin)
		line, err := reader.ReadString('\n')
		if err != nil {
			log.Fatalf("Failed to read passphrase from stdin: %v", err)
		}
		passphrase = []byte(strings.TrimRight(line, "\r\n"))
	} else {
		// Interactive mode with prompt
		fmt.Print("Enter passphrase: ")
		passphrase, err = term.ReadPassword(int(syscall.Stdin))
		if err != nil {
			log.Fatalf("Failed to read passphrase: %v", err)
		}
		fmt.Println()
	}

	// Handle daemonization: re-exec as a child process that runs in background
	if daemonize && !childProcess {
		// Build args for child process
		args := []string{
			"-mount", mountPoint,
			"-container", containerPath,
			"-passphrase-stdin",
			"-child",
		}
		if debug {
			args = append(args, "-debug")
		}
		if allowOther {
			args = append(args, "-allow-other")
		}

		cmd := exec.Command(os.Args[0], args...)

		// Create a pipe to pass the passphrase to the child
		stdinPipe, err := cmd.StdinPipe()
		if err != nil {
			log.Fatalf("Failed to create stdin pipe: %v", err)
		}

		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr

		// Start child
		if err := cmd.Start(); err != nil {
			log.Fatalf("Failed to daemonize: %v", err)
		}

		// Write passphrase to child's stdin and close the pipe
		_, err = stdinPipe.Write([]byte(string(passphrase) + "\n"))
		if err != nil {
			log.Fatalf("Failed to write passphrase to child: %v", err)
		}
		stdinPipe.Close()

		fmt.Printf("GPGFS daemonizing (pid %d)...\n", cmd.Process.Pid)
		os.Exit(0)
	}

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
			AllowOther:    allowOther,
		},
	}

	// Mount the filesystem
	server, err := fs.Mount(mountPoint, root, opts)
	if err != nil {
		log.Fatalf("Failed to mount filesystem: %v", err)
	}

	if !childProcess {
		fmt.Printf("GPGFS mounted at %s\n", mountPoint)
		fmt.Println("Press Ctrl+C to unmount and exit")
	}

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
	if !childProcess {
		fmt.Println("Filesystem unmounted")
	}
}
