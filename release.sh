#!/bin/bash
set -e

tag="$1"

if [ -z "$tag" ]; then
    echo "Usage: $0 <tag>"
    exit 1
fi

sed -i '' "s/const VERSION = \"[^\"]*\"/const VERSION = \"$tag\"/" main.go

git tag "$tag"
changeLogEntry=$(./changelog.sh)

currentChangeLog=$(tail -n +2 CHANGELOG.md)

{
    echo "# CHANGELOG"
    echo ""
    echo "$changeLogEntry"
    echo ""
    echo "$currentChangeLog"
} >CHANGELOG.md

git add CHANGELOG.md main.go
git commit -m "chore: Update CHANGELOG.md and version for release $tag"

git tag --delete "$tag"

git tag "$tag" -m "Release $tag"

if [ $? -ne 0 ]; then
    echo "Failed to create tag $tag. Please check if the tag already exists."
    exit 1
fi

echo "Tag $tag created successfully."

git push && git push --tags

if [ $? -ne 0 ]; then
    echo "Failed to push changes and tags to the remote repository."
    exit 1
fi

echo "Changes and tags pushed successfully."
