#!/bin/bash
# PAM helper script for mounting gpgfs on login
# This script is called by pam_exec during authentication
#
# The login password is passed via stdin by PAM when using expose_authtok
# This assumes your gpgfs container uses the same passphrase as your login password
#
# Installation:
#   1. Copy this script to /usr/local/bin/pam_gpgfs_mount.sh
#   2. chmod +x /usr/local/bin/pam_gpgfs_mount.sh
#   3. Add to /etc/pam.d/system-login (or appropriate PAM config):
#      auth optional pam_exec.so expose_authtok /usr/local/bin/pam_gpgfs_mount.sh
#
# Configuration: Edit these variables or set them as environment variables

# Path to gpgfs binary
GPGFS_BIN="${GPGFS_BIN:-/usr/local/bin/gpgfs}"

# Container file location (use $PAM_USER for per-user containers)
GPGFS_CONTAINER="${GPGFS_CONTAINER:-/home/${PAM_USER}/.gpgfs/container.gpgfs}"

# Mount point (typically the user's home directory or a subdirectory)
GPGFS_MOUNT="${GPGFS_MOUNT:-/home/${PAM_USER}}"

# Log file for debugging (set to /dev/null to disable)
GPGFS_LOG="${GPGFS_LOG:-/tmp/gpgfs-pam.log}"

# Only run for the actual user, not root
if [ "$PAM_USER" = "root" ]; then
    exit 0
fi

# Only run during authentication phase
if [ "$PAM_TYPE" != "auth" ]; then
    exit 0
fi

# Check if container exists
if [ ! -f "$GPGFS_CONTAINER" ]; then
    echo "$(date): Container not found: $GPGFS_CONTAINER" >> "$GPGFS_LOG"
    exit 0  # Exit successfully to not block login
fi

# Check if already mounted
if mountpoint -q "$GPGFS_MOUNT" 2>/dev/null; then
    echo "$(date): Already mounted: $GPGFS_MOUNT" >> "$GPGFS_LOG"
    exit 0
fi

# Read password from stdin (provided by pam_exec with expose_authtok)
read -r password

# Mount the filesystem
# The password is piped to gpgfs via stdin
echo "$password" | "$GPGFS_BIN" \
    -mount "$GPGFS_MOUNT" \
    -container "$GPGFS_CONTAINER" \
    -passphrase-stdin \
    -daemonize \
    -allow-other \
    >> "$GPGFS_LOG" 2>&1

result=$?

if [ $result -eq 0 ]; then
    echo "$(date): Successfully mounted $GPGFS_MOUNT for $PAM_USER" >> "$GPGFS_LOG"
else
    echo "$(date): Failed to mount $GPGFS_MOUNT for $PAM_USER (exit code: $result)" >> "$GPGFS_LOG"
fi

# Always exit 0 to not block login even if mount fails
exit 0
