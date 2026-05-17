package detect

import (
	"bufio"
	"fmt"
	"io"
)

// defaultStreamLookback is the per-line buffer ceiling used by
// scanStream. 4 KB comfortably covers env files, log lines, and tool
// stdout; anything longer is split at the byte boundary and scanned
// in halves. The scanner emits a warning-style error if a single
// "line" exceeds the hard cap defined below — practical inputs never
// do, so the cap is generous (1 MiB).
const (
	defaultStreamLookback = 4 * 1024
	maxStreamLineSize     = 1 * 1024 * 1024
)

// scanStream reads r line-by-line, runs d.Scan over each line, and
// emits findings with byte offsets translated into the original
// stream's coordinate space (i.e. byte 0 = first byte read).
//
// Line-buffered (not chunk-buffered) is the right primitive for
// Phase 1: every real secret we care about — env-file values, JSON
// fields on a single line, log-line tokens — lives on one line, and
// a line-oriented scanner gives us trivial offset arithmetic and no
// cross-chunk match-splitting bugs.
//
// initialBuf seeds the bufio.Scanner buffer. Lines longer than
// initialBuf grow the buffer up to maxStreamLineSize.
func scanStream(r io.Reader, d Detector, emit func(Finding), initialBuf int) error {
	if initialBuf <= 0 {
		initialBuf = defaultStreamLookback
	}
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, initialBuf), maxStreamLineSize)
	// Custom split keeps the trailing newline visible so our offset
	// counter advances by the true number of bytes consumed.
	scanner.Split(scanLinesKeepNewline)

	var offset int
	for scanner.Scan() {
		line := scanner.Bytes()
		stripped, nlBytes := trimTrailingNewline(line)
		findings, err := d.Scan(stripped)
		if err != nil {
			return fmt.Errorf("detect: scan line at offset %d: %w", offset, err)
		}
		for _, f := range findings {
			f.Start += offset
			f.End += offset
			emit(f)
		}
		offset += len(stripped) + nlBytes
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("detect: stream read: %w", err)
	}
	return nil
}

// scanLinesKeepNewline is a bufio.SplitFunc that returns each line
// together with its trailing newline byte(s). The standard
// bufio.ScanLines strips them, which breaks our offset accounting on
// the trailing line of the input.
func scanLinesKeepNewline(data []byte, atEOF bool) (advance int, token []byte, err error) {
	if atEOF && len(data) == 0 {
		return 0, nil, nil
	}
	for i, b := range data {
		if b == '\n' {
			return i + 1, data[:i+1], nil
		}
	}
	if atEOF {
		return len(data), data, nil
	}
	return 0, nil, nil
}

// trimTrailingNewline returns line without its trailing "\r\n" or
// "\n", plus how many bytes were trimmed. It is the inverse of the
// extra bytes scanLinesKeepNewline included so callers can advance
// the running offset correctly.
func trimTrailingNewline(line []byte) ([]byte, int) {
	n := len(line)
	if n >= 2 && line[n-2] == '\r' && line[n-1] == '\n' {
		return line[:n-2], 2
	}
	if n >= 1 && line[n-1] == '\n' {
		return line[:n-1], 1
	}
	return line, 0
}
