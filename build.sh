#!/bin/bash

set -e

# If executable exists, remove it
if [ -f ./bubbletea-poc ]; then
  echo "Removing existing executable..."
  rm ./bubbletea-poc
fi

echo "Compiling Swift audio bridge..."
cd audio
swiftc -c -parse-as-library AudioBridge.swift -o AudioBridge.o
cd ..

echo "Building Go project..."
go build

echo "Build complete!"
