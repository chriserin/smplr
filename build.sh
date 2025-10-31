#!/bin/bash

set -e

# If executable exists, remove it
if [ -f ./smplr ]; then
  echo "Removing existing executable..."
  rm ./smplr
fi

echo "Compiling Swift audio bridge..."
cd audio
swiftc -c -parse-as-library AudioBridge.swift -o AudioBridge.o
cd ..

echo "Building Go project..."
go build

echo "Build complete!"
