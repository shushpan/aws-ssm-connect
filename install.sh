#!/bin/bash

set -e

# Define variables
BINARY_NAME="aws-ssm-connect"
INSTALL_DIR="/usr/local/bin"
GITHUB_REPO="shushpan/aws-ssm-connect"
LATEST_RELEASE_URL="https://api.github.com/repos/$GITHUB_REPO/releases/latest"

# Detect OS and architecture
OS=$(uname -s | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m)

if [ "$ARCH" = "x86_64" ]; then
    ARCH="x86_64"
elif [ "$ARCH" = "arm64" ] || [ "$ARCH" = "aarch64" ]; then
    ARCH="arm64"
else
    echo "Unsupported architecture: $ARCH"
    exit 1
fi

# Get the latest release tag
echo "Fetching latest release information..."
RELEASE_TAG=$(curl -s $LATEST_RELEASE_URL | grep "tag_name" | cut -d '"' -f 4)

if [ -z "$RELEASE_TAG" ]; then
    echo "Failed to fetch latest release information."
    exit 1
fi

echo "Latest release: $RELEASE_TAG"

# Construct download URL
if [ "$OS" = "darwin" ]; then
    OS="Darwin"
elif [ "$OS" = "linux" ]; then
    OS="Linux"
else
    echo "Unsupported OS: $OS"
    exit 1
fi

DOWNLOAD_URL="https://github.com/$GITHUB_REPO/releases/download/$RELEASE_TAG/${BINARY_NAME}_${OS}_${ARCH}.tar.gz"
echo "Download URL: $DOWNLOAD_URL"

# Create temporary directory
TMP_DIR=$(mktemp -d)
echo "Created temporary directory: $TMP_DIR"

# Download and extract
echo "Downloading $BINARY_NAME..."
curl -L "$DOWNLOAD_URL" -o "$TMP_DIR/$BINARY_NAME.tar.gz"
echo "Extracting..."
tar -xzf "$TMP_DIR/$BINARY_NAME.tar.gz" -C "$TMP_DIR"

# Install
echo "Installing to $INSTALL_DIR..."
sudo mv "$TMP_DIR/$BINARY_NAME" "$INSTALL_DIR/"
sudo chmod +x "$INSTALL_DIR/$BINARY_NAME"

# Clean up
echo "Cleaning up..."
rm -rf "$TMP_DIR"

echo "$BINARY_NAME $RELEASE_TAG has been installed to $INSTALL_DIR/$BINARY_NAME"
echo "You can now run it using: $BINARY_NAME" 