// Package safe turns a panic into an error so that a long-lived loop logs it and keeps running.
package safe

import (
	"fmt"
	"runtime/debug"
)

// Panic is the error Run returns when fn panicked: the recovered value and the stack.
type Panic struct {
	Value any
	Stack []byte
}

// Error reads as the panic value followed by the stack.
func (p *Panic) Error() string { return fmt.Sprintf("panic: %v\n%s", p.Value, p.Stack) }

// Run returns the error of fn, or a *Panic when fn panics.
func Run(fn func() error) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = &Panic{Value: r, Stack: debug.Stack()}
		}
	}()
	return fn()
}
