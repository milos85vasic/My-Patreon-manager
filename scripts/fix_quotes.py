#!/usr/bin/env python3
import os
import sys


def fix_file(filepath):
    with open(filepath, "r") as f:
        lines = f.readlines()
    changed = False
    new_lines = []
    for line in lines:
        # Count double quotes (ignore escaped quotes for simplicity)
        if line.count('"') % 2 == 1:
            # line missing a closing quote
            # add a double quote at end before newline (if not already there)
            if not line.rstrip().endswith('"'):
                line = line.rstrip() + '"' + "\n"
                changed = True
        new_lines.append(line)
    if changed:
        with open(filepath, "w") as f:
            f.writelines(new_lines)
        print(f"Fixed {filepath}")
    return changed


def main():
    root = "."
    for dirpath, dirnames, filenames in os.walk(root):
        # Skip .git, vendor, etc.
        if ".git" in dirpath or "vendor" in dirpath:
            continue
        for fname in filenames:
            if fname.endswith("_test.go"):
                fix_file(os.path.join(dirpath, fname))


if __name__ == "__main__":
    main()
