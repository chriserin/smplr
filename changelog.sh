#!/bin/bash

set -e

startTag=$(git tag --list --sort=version:refname | tail -n 1)
lastTag=$(git tag --list --sort=version:refname | tail -n 2 | head -n 1)

if [ -z "$startTag" ]; then
    echo "No tags found. Please create a tag first."
    exit 1
fi

if [ -z "$lastTag" ]; then
    echo "Only one tag found: $startTag. No changes to show."
    exit 0
fi

# if script argument of "WITHINSTALL" is present
if [ "$1" == "WITHINSTALL" ]; then
    cat <<EOF
## Install

### macOS (arm64)

1. Download smplr-macos-arm64.tar.gz
2. Run xattr -c ./smplr-macos-arm64.tar.gz (to avoid "unknown developer" warning)
3. Extract: tar xzvf smplr-macos-arm64.tar.gz
4. Run ./smplr-macos-arm64/bin/smplr

### macOS (x86_64)

1. Download smplr-macos-x86_64.tar.gz
2. Run xattr -c ./smplr-macos-x86_64.tar.gz (to avoid "unknown developer" warning)
3. Extract: tar xzvf smplr-macos-x86_64.tar.gz
4. Run ./smplr-macos-x86_64/bin/smplr

EOF
fi

date=$(git log -1 --format=%cd --date=short "$startTag")

echo "## [${startTag}](https://github.com/chriserin/smplr/compare/${lastTag}...${startTag}) ($date)"
echo ""

features=$(git log --oneline "${startTag}"..."${lastTag}" | awk ' $2 ~ /^feat/ {print}')

if [ -n "$features" ]; then
    echo "### Features"
    echo ""
    while IFS= read -r line; do
        commit_hash=$(echo "$line" | awk '{print $1}')
        commit_message=$(echo "$line" | cut -d' ' -f3-)
        echo "* $commit_message [${commit_hash}](https://github.com/chriserin/smplr/commit/${commit_hash}) "
    done <<<"$features"
else
    echo "### Features"
    echo ""
    echo "No new features."
    echo ""
fi

fixes=$(git log --oneline "${startTag}"..."${lastTag}" | awk ' $2 ~ /^fix/ {print}')

if [ -n "$fixes" ]; then
    echo ""
    echo "### Fixes"
    echo ""
    while IFS= read -r line; do
        commit_hash=$(echo "$line" | awk '{print $1}')
        commit_type=$(echo "$line" | awk '{print $2}')
        commit_message=$(echo "$line" | cut -d' ' -f3-)

        # Extract scope if present (e.g., fix(system) -> system)
        regex='fix\(([^)]+)\)'
        if [[ $commit_type =~ $regex ]]; then
            scope="${BASH_REMATCH[1]}"
            commit_message="${scope}: ${commit_message}"
        fi

        echo "* $commit_message [${commit_hash}](https://github.com/chriserin/smplr/commit/${commit_hash}) "
    done <<<"$fixes"
else
    echo "### Fixes"
    echo ""
    echo "No bug fixes."
    echo ""
fi
