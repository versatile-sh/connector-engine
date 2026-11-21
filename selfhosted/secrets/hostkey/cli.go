package hostkey

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"
	"time"
)

// Main is the unlock command of the engine and console images: it reads the hex unlock key on stdin for unlock and migrate, sends one request and prints the protection.
func Main(args []string, socket string, stdin io.Reader, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("unlock", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.StringVar(&socket, "socket", socket, "the unlock socket")
	op := fs.String("op", string(OpUnlock), "unlock, plaintext, migrate, revert or status")
	wait := fs.Duration("wait", time.Minute, "how long to wait for the socket")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	req := Request{Op: Op(*op)}
	if req.Op == OpUnlock || req.Op == OpMigrate {
		line, err := bufio.NewReader(io.LimitReader(stdin, 256)).ReadString('\n')
		if err != nil && err != io.EOF {
			fmt.Fprintln(stderr, err)
			return 1
		}
		req.Key = strings.TrimSpace(line)
	}
	ctx, cancel := context.WithTimeout(context.Background(), *wait)
	defer cancel()
	r, err := Send(ctx, socket, req)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	fmt.Fprintln(stdout, r.Protection)
	return 0
}
