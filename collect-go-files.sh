#!/usr/bin/env bash

set -euo pipefail

OUTPUT_FILE="all-go-files.txt"

# فایل خروجی قبلی را حذف کن
: > "$OUTPUT_FILE"

find . -type f -name "*.go" \
    ! -path "./.git/*" \
    -print0 |
while IFS= read -r -d '' file; do

    {
        echo "================================================================================"
        echo "FILE: $file"
        echo "================================================================================"
        cat "$file"
        echo
        echo
    } >> "$OUTPUT_FILE"

done

echo "Done."
echo "Output: $OUTPUT_FILE"