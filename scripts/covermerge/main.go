// covermerge reads one or more Go cover profiles and emits a single
// merged profile on stdout. For each statement block (keyed by file
// plus line range) the merged value is the MAX count across inputs,
// which is the correct reduction for atomic-mode profiles where a
// statement is covered if any test binary saw it.
//
// Usage:
//
//	covermerge path/to/a.out path/to/b.out ... > coverage/coverage.out
//
// All inputs must share the same mode header (the first non-blank
// line, e.g. `mode: atomic`). Empty inputs are skipped. Duplicate
// keys within a single file take MAX to stay idempotent against
// quirky inputs.
package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"
)

// statementKey uniquely identifies a statement block in a cover profile.
// Two profile lines refer to the same block exactly when their file and
// line-range prefix match, which is what Go's cover tool guarantees.
type statementKey struct {
	file  string
	block string
}

// statementRecord carries the statement-count alongside its block key.
// `numStmt` is part of the line but must be consistent across inputs;
// we track it so the output preserves the tool's expected format.
type statementRecord struct {
	numStmt int
	count   int64
}

// mergeProfiles reads every profile in the input slice and returns the
// shared mode header plus the merged (max-per-statement) record map.
// Returns an error on mode mismatch or malformed profile lines.
func mergeProfiles(readers []io.Reader) (mode string, merged map[statementKey]statementRecord, err error) {
	merged = map[statementKey]statementRecord{}
	for i, r := range readers {
		fileMode, records, perr := parseProfile(r)
		if perr != nil {
			return "", nil, fmt.Errorf("profile %d: %w", i, perr)
		}
		if fileMode == "" {
			// An empty profile (tool writes nothing when no tests ran).
			continue
		}
		if mode == "" {
			mode = fileMode
		} else if mode != fileMode {
			return "", nil, fmt.Errorf("profile %d: mode %q conflicts with earlier mode %q", i, fileMode, mode)
		}
		for k, v := range records {
			if prev, ok := merged[k]; ok {
				if v.count > prev.count {
					prev.count = v.count
				}
				merged[k] = prev
				continue
			}
			merged[k] = v
		}
	}
	return mode, merged, nil
}

// parseProfile reads one cover profile into a mode string and a
// record map. It validates that each body line has the expected four
// whitespace-separated tokens; anything else is an error so we don't
// silently swallow corrupt input.
func parseProfile(r io.Reader) (string, map[statementKey]statementRecord, error) {
	records := map[statementKey]statementRecord{}
	var mode string
	s := bufio.NewScanner(r)
	// Cover profiles can have long lines on large packages; lift the
	// default 64KiB scanner buffer.
	buf := make([]byte, 0, 1<<20)
	s.Buffer(buf, 1<<20)

	for s.Scan() {
		line := strings.TrimSpace(s.Text())
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "mode:") {
			m := strings.TrimSpace(strings.TrimPrefix(line, "mode:"))
			if mode != "" && mode != m {
				return "", nil, fmt.Errorf("multiple mode headers: %q vs %q", mode, m)
			}
			mode = m
			continue
		}
		fields := strings.Fields(line)
		if len(fields) != 3 {
			return "", nil, fmt.Errorf("malformed profile line: %q", line)
		}
		// fields[0] is "file.go:start.col,end.col" which we split into
		// file prefix (up to the last colon before the digits) and the
		// block range; we treat the whole thing opaquely as the key's
		// block portion.
		block := fields[0]
		colon := strings.LastIndex(block, ":")
		if colon <= 0 {
			return "", nil, fmt.Errorf("malformed block key: %q", block)
		}
		key := statementKey{file: block[:colon], block: block[colon+1:]}

		numStmt, err := strconv.Atoi(fields[1])
		if err != nil {
			return "", nil, fmt.Errorf("malformed numStmt in %q: %w", line, err)
		}
		count, err := strconv.ParseInt(fields[2], 10, 64)
		if err != nil {
			return "", nil, fmt.Errorf("malformed count in %q: %w", line, err)
		}

		if prev, ok := records[key]; ok {
			if count > prev.count {
				prev.count = count
			}
			records[key] = prev
			continue
		}
		records[key] = statementRecord{numStmt: numStmt, count: count}
	}
	if err := s.Err(); err != nil {
		return "", nil, err
	}
	return mode, records, nil
}

// writeMerged emits the merged profile in the canonical Go format:
// one `mode: <mode>` line followed by sorted block records.
func writeMerged(w io.Writer, mode string, merged map[statementKey]statementRecord) error {
	if mode == "" {
		// No inputs contributed data — still emit a valid empty profile.
		_, err := fmt.Fprintln(w, "mode: atomic")
		return err
	}
	if _, err := fmt.Fprintf(w, "mode: %s\n", mode); err != nil {
		return err
	}
	keys := make([]statementKey, 0, len(merged))
	for k := range merged {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].file != keys[j].file {
			return keys[i].file < keys[j].file
		}
		return keys[i].block < keys[j].block
	})
	for _, k := range keys {
		v := merged[k]
		if _, err := fmt.Fprintf(w, "%s:%s %d %d\n", k.file, k.block, v.numStmt, v.count); err != nil {
			return err
		}
	}
	return nil
}

// run is the testable entry point. main() calls it with os.Args and
// the process stdout.
func run(args []string, out io.Writer) error {
	if len(args) == 0 {
		return errors.New("usage: covermerge profile1.out [profile2.out ...]")
	}
	files := make([]*os.File, 0, len(args))
	readers := make([]io.Reader, 0, len(args))
	defer func() {
		for _, f := range files {
			_ = f.Close()
		}
	}()
	for _, p := range args {
		f, err := os.Open(p)
		if err != nil {
			return fmt.Errorf("open %s: %w", p, err)
		}
		files = append(files, f)
		readers = append(readers, f)
	}
	mode, merged, err := mergeProfiles(readers)
	if err != nil {
		return err
	}
	return writeMerged(out, mode, merged)
}

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "covermerge:", err)
		os.Exit(1)
	}
}
