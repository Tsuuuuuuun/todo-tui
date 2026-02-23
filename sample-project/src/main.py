#!/usr/bin/env python3
"""Main application module."""

# TODO: Add proper argument parsing with argparse
import sys


def main():
    """Entry point."""
    # FIXME: This crashes when no arguments are provided
    name = sys.argv[1]
    print(f"Hello, {name}!")

    # TODO: Implement configuration file loading
    config = {}

    # HACK: Temporary workaround for encoding issues
    result = name.encode("utf-8").decode("ascii", errors="ignore")
    return result


if __name__ == "__main__":
    main()
