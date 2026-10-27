#!/usr/bin/env bash

set -euo pipefail

OUTPUT_FILE="all-md-files.txt"

# فایل خروجی قبلی را حذف کن
: > "$OUTPUT_FILE"

find . -type f -name "*.md" \
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
