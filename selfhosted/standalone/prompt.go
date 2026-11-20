package standalone

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"golang.org/x/term"
)

type prompt struct {
	tty *os.File
	in  *bufio.Reader
	out io.Writer
}

func newPrompt(stdin io.Reader, stderr io.Writer) prompt {
	p := prompt{in: bufio.NewReader(stdin), out: stderr}
	if f, ok := stdin.(*os.File); ok && term.IsTerminal(int(f.Fd())) {
		p.tty = f
	}
	return p
}

func (p prompt) say(text string) { fmt.Fprint(p.out, text) }

func (p prompt) line() (string, error) {
	s, err := p.in.ReadString('\n')
	if err == io.EOF && s != "" {
		err = nil
	}
	return strings.TrimRight(s, "\r\n"), err
}

func (p prompt) secret(label string) ([]byte, error) {
	fmt.Fprint(p.out, label+": ")
	defer fmt.Fprintln(p.out)
	if p.tty != nil {
		return term.ReadPassword(int(p.tty.Fd()))
	}
	s, err := p.line()
	return []byte(s), err
}

func (p prompt) twice(label string) ([]byte, error) {
	first, err := p.secret(label)
	if err != nil {
		return nil, err
	}
	second, err := p.secret(label + " again")
	if err != nil {
		return nil, err
	}
	if !bytes.Equal(first, second) {
		return nil, errors.New("the two entries differ")
	}
	return first, nil
}

func (p prompt) confirm(question string, yes bool) error {
	if yes {
		return nil
	}
	fmt.Fprint(p.out, question+" [y/N] ")
	answer, _ := p.line()
	switch strings.ToLower(strings.TrimSpace(answer)) {
	case "y", "yes":
		return nil
	}
	return errors.New("nothing changed")
}
