#!/usr/bin/env python3
import subprocess
import sys
import os


def get_package_coverage():
    # Run go tool cover -func on existing coverage.out
    coverage_file = "coverage.out"
    if not os.path.exists(coverage_file):
        print("coverage.out not found. Run coverage script first.")
        sys.exit(1)

    result = subprocess.run(
        ["go", "tool", "cover", "-func", coverage_file], capture_output=True, text=True
    )
    if result.returncode != 0:
        print("Failed to run go tool cover:", result.stderr)
        sys.exit(1)

    # Parse output
    package_data = {}
    for line in result.stdout.strip().split("\n"):
        if not line.strip():
            continue
        # Example line: github.com/milos85vasic/My-Patreon-Manager/internal/config/config.go:43:			NewConfig			0.0%
        # We want package path and percentage
        if line.startswith("total"):
            continue
        tabs = line.split("\t")
        if len(tabs) < 3:
            continue
        file_part = tabs[0].strip()
        perc_part = tabs[-1].strip()
        if not perc_part.endswith("%"):
            continue
        perc = float(perc_part[:-1])
        # Extract package: remove filename
        if "/" not in file_part:
            continue
        package = "/".join(file_part.split("/")[:-1])
        if not package.startswith("github.com/milos85vasic/My-Patreon-Manager/"):
            continue
        # Strip prefix
        prefix = "github.com/milos85vasic/My-Patreon-Manager/"
        rel_package = package[len(prefix) :]
        # Only consider internal/ and cmd/ packages
        if not (rel_package.startswith("internal/") or rel_package.startswith("cmd/")):
            continue
        if rel_package not in package_data:
            package_data[rel_package] = {"total": 0.0, "count": 0}
        package_data[rel_package]["total"] += perc
        package_data[rel_package]["count"] += 1

    # Compute averages
    print("Package Coverage (internal/ and cmd/ only):")
    all_passed = True
    for pkg in sorted(package_data.keys()):
        avg = package_data[pkg]["total"] / package_data[pkg]["count"]
        status = "✅" if avg >= 100.0 else "❌"
        if avg < 100.0:
            all_passed = False
        print(f"{status} {pkg}: {avg:.1f}%")

    if not all_passed:
        print("\n❌ Some packages have coverage below 100%.")
        sys.exit(1)
    else:
        print("\n✅ All packages have 100% coverage.")
        sys.exit(0)


if __name__ == "__main__":
    get_package_coverage()
