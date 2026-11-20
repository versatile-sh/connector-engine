package console

import (
	"bufio"
	"bytes"
	"io"
	"slices"
)

// ReadEvents calls fn with the data of each server-sent event read from r, joining multi-line data with newlines, until r ends.
func ReadEvents(r io.Reader, fn func(data []byte)) error {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64<<10), 64<<20)
	var data [][]byte
	flush := func() {
		if len(data) > 0 {
			fn(bytes.Join(data, []byte("\n")))
			data = nil
		}
	}
	for sc.Scan() {
		line := sc.Bytes()
		if len(line) == 0 {
			flush()
			continue
		}
		if v, ok := bytes.CutPrefix(line, []byte("data:")); ok {
			data = append(data, slices.Clone(bytes.TrimPrefix(v, []byte(" "))))
		}
	}
	flush()
	return sc.Err()
}
