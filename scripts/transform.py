#!/usr/bin/env python3
import re
import sys


def transform(content):
    # Change package line
    content = re.sub(
        r"^package patreon_test$", "package patreon", content, flags=re.MULTILINE
    )
    # Remove import of patreon package
    lines = content.split("\n")
    in_import = False
    new_lines = []
    for i, line in enumerate(lines):
        if line.strip() == "import (":
            in_import = True
            new_lines.append(line)
            continue
        if in_import and line.strip() == ")":
            in_import = False
            new_lines.append(line)
            continue
        if (
            in_import
            and "github.com/milos85vasic/My-Patreon-Manager/internal/providers/patreon"
            in line
        ):
            # skip this import line
            continue
        new_lines.append(line)
    content = "\n".join(new_lines)
    # Replace patreon. with empty (but not inside strings/comments)
    # Simple approach: replace all occurrences of 'patreon.' when preceded by whitespace or punctuation
    # This might break string literals but unlikely.
    content = re.sub(r"\bpatreon\.", "", content)
    return content


if __name__ == "__main__":
    if len(sys.argv) != 3:
        print("Usage: transform.py <input> <output>")
        sys.exit(1)
    with open(sys.argv[1], "r") as f:
        data = f.read()
    transformed = transform(data)
    with open(sys.argv[2], "w") as f:
        f.write(transformed)
    print(f"Transformed {sys.argv[1]} -> {sys.argv[2]}")
