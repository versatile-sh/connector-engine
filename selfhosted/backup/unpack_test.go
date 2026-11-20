package backup

import (
	"bytes"
	"errors"
	"fmt"
	"testing"
)

func tarHeader(name string, flag byte, size int) []byte {
	h := make([]byte, 512)
	copy(h[0:], name)
	copy(h[100:], "0000600\x00")
	copy(h[108:], "0000000\x00")
	copy(h[116:], "0000000\x00")
	copy(h[124:], fmt.Sprintf("%011o\x00", size))
	copy(h[136:], "00000000000\x00")
	h[156] = flag
	copy(h[257:], "ustar\x0000")
	copy(h[148:], "        ")
	sum := 0
	for _, c := range h {
		sum += int(c)
	}
	copy(h[148:], fmt.Sprintf("%06o\x00 ", sum))
	return h
}

func pad(b *bytes.Buffer) {
	for b.Len()%512 != 0 {
		b.WriteByte(0)
	}
}

func TestASparseEntryThatExpandsPastTheArchiveIsRefused(t *testing.T) {
	record := func(k, v string) string {
		s := " " + k + "=" + v + "\n"
		n := len(s) + 1
		for len(fmt.Sprint(n))+len(s) != n {
			n++
		}
		return fmt.Sprint(n) + s
	}
	pax := record("GNU.sparse.major", "1") + record("GNU.sparse.minor", "0") + record("GNU.sparse.name", "console/activity/a") + record("GNU.sparse.realsize", "2000000000")
	var plain, data bytes.Buffer
	plain.Write(tarHeader("pax", 'x', len(pax)))
	plain.WriteString(pax)
	pad(&plain)
	data.WriteString("1\n1999999999\n1\n")
	pad(&data)
	data.WriteString("A")
	pad(&data)
	plain.Write(tarHeader("x", '0', 513))
	plain.Write(data.Bytes())
	plain.Write(make([]byte, 1024))
	if _, _, err := unpack(plain.Bytes()); !errors.Is(err, ErrTampered) {
		t.Fatalf("a 2 GB sparse entry in a %d-byte archive: %v", plain.Len(), err)
	}
}
