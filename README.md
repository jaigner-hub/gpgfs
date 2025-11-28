# gpgfs

A GPG-encrypted FUSE filesystem written in Go. Mount a single encrypted container file as a fully-featured filesystem with transparent encryption and decryption.

## Features

- **Transparent encryption** - All files and metadata are automatically encrypted using GPG (AES-256)
- **Single container file** - Your entire encrypted filesystem lives in one portable file
- **Full filesystem support** - Create, read, write, delete, and rename files and directories
- **Passphrase protected** - Container is secured with a passphrase that protects the GPG key
- **Persistent storage** - Data persists across mounts; reopen with the same passphrase
- **POSIX semantics** - Supports permissions, timestamps, and nested directory structures

## Installation

### Prerequisites

- Go 1.21 or later
- FUSE installed on your system
  - **Debian/Ubuntu:** `sudo apt install fuse3`
  - **Fedora:** `sudo dnf install fuse3`
  - **Arch:** `sudo pacman -S fuse3`
  - **macOS:** Install [macFUSE](https://osxfuse.github.io/)

### Build from source

```bash
git clone https://github.com/jaigner-hub/gpgfs.git
cd gpgfs
go build
```

## Usage

```bash
./gpgfs -mount <mountpoint> -container <container_file>
```

### Options

| Flag | Description |
|------|-------------|
| `-mount` | Directory where the filesystem will be mounted (required) |
| `-container` | Path to the encrypted container file (required, created if doesn't exist) |
| `-debug` | Enable debug logging for FUSE operations |

### Example

```bash
# Create mount directory
mkdir ~/encrypted

# Mount a new or existing container
./gpgfs -mount ~/encrypted -container ~/vault.gpgfs

# Enter your passphrase when prompted
# Your encrypted filesystem is now available at ~/encrypted

# Use it like a normal filesystem
echo "secret data" > ~/encrypted/secret.txt
mkdir ~/encrypted/documents
cp important.pdf ~/encrypted/documents/

# Unmount when done (Ctrl+C or close the terminal)
```

### Unmounting

- Press `Ctrl+C` in the terminal running gpgfs
- Or run: `fusermount -u <mountpoint>`

## How It Works

```
┌─────────────────────────────────────────────────────────┐
│                    Your Applications                     │
└─────────────────────────────────────────────────────────┘
                            │
                            ▼
┌─────────────────────────────────────────────────────────┐
│                   FUSE Mount Point                       │
│                   (e.g., ~/encrypted)                    │
└─────────────────────────────────────────────────────────┘
                            │
                            ▼
┌─────────────────────────────────────────────────────────┐
│                      gpgfs                               │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────────┐  │
│  │  FUSE Layer │→ │ GPG Crypto  │→ │ BoltDB Storage  │  │
│  │   (fs/)     │  │  (crypto/)  │  │   (storage/)    │  │
│  └─────────────┘  └─────────────┘  └─────────────────┘  │
└─────────────────────────────────────────────────────────┘
                            │
                            ▼
┌─────────────────────────────────────────────────────────┐
│              Encrypted Container File                    │
│                  (e.g., vault.gpgfs)                     │
└─────────────────────────────────────────────────────────┘
```

1. **FUSE Layer** - Intercepts filesystem operations and translates them to gpgfs calls
2. **GPG Crypto** - Encrypts data on write, decrypts on read using AES-256
3. **BoltDB Storage** - Stores encrypted metadata and file contents in a single container file

## Security

- **Encryption:** AES-256 via GPG/OpenPGP
- **Key Protection:** GPG private key is encrypted with your passphrase
- **At-rest Encryption:** All data and metadata stored encrypted in the container
- **Memory:** Decrypted data only exists in memory while being accessed

### Best Practices

- Use a strong, unique passphrase
- Keep backups of your container file
- Unmount the filesystem when not in use
- Store the container file on encrypted disk for defense in depth

## Dependencies

- [go-crypto](https://github.com/ProtonMail/go-crypto) - GPG/OpenPGP implementation
- [go-fuse](https://github.com/hanwen/go-fuse) - FUSE bindings for Go
- [bbolt](https://github.com/etcd-io/bbolt) - Embedded key-value database

## Testing

```bash
go test ./...
```

## License

MIT License
