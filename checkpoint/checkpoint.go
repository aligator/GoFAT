// Package checkpoint provides a way to decorate errors by some additional caller information
// which results in something similar to a stacktrace.
// Each error added to a checkpoint can be checked by errors.Is and retrieved by errors.As.
package checkpoint

import (
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"runtime"
	"strings"
)

// From just wraps an error by a new checkpoint which adds some caller information to the error.
// It returns nil, if err == nil.
func From(err error) error {
	// io.EOF must be returned as io.EOF directly
	// https://github.com/golang/go/issues/39155
	if err == io.EOF {
		return io.EOF
	}
	if err == io.ErrUnexpectedEOF {
		return io.ErrUnexpectedEOF
	}

	if err == nil {
		return nil
	}

	// Get the caller information.
	_, file, line, ok := runtime.Caller(1)

	return &checkpoint{
		err:  err,
		prev: nil,

		callerOk: ok,
		file:     filepath.Base(file),
		line:     line,
	}
}

// Wrap adds a checkpoint with some caller information from an error and accepts
// also another error which can further describe the checkpoint.
// Returns nil if prev == nil.
// If err is nil, it still creates a checkpoint.
// This allows for example to predefine some errors and use them later:
//  var(
//  		ErrSomethingSpecialWentWrong = errors.New("a very bad error")
//  )
//  func someFunction() error {
//  	err := somethingOtherThatThrowsErrors()
//  	return checkpoint.Wrap(err, ErrSomethingSpecialWentWrong)
//  }
//
//  err := someFunction()
// If used that way, you can still check with errors.Is() for the ErrSomethingSpecialWentWrong
//  if errors.Is(err, ErrSomethingSpecialWentWrong) {
//  	fmt.Println("The special error was thrown")
//  } else {
//  	fmt.Println(err)
//  }
// but also for the error returned by somethingOtherThatThrowsErrors() (if you know what error it is).
// If the error in this example is nil, no checkpoint gets created.
func Wrap(prev, err error) error {
	// io.EOF must be returned as io.EOF directly
	// https://github.com/golang/go/issues/39155
	if prev == io.EOF {
		return io.EOF
	}

	if prev == nil {
		return nil
	}

	// Get the caller information.
	_, file, line, ok := runtime.Caller(1)

	return &checkpoint{
		err:  err,
		prev: prev,

		callerOk: ok,
		file:     filepath.Base(file),
		line:     line,
	}
}

type checkpoint struct {
	err  error
	prev error

	callerOk bool
	file     string
	line     int
}

func (e *checkpoint) Error() string {
	// Use different formatting for the prev error if it was not also a checkpoint.
	prevErrString := e.prev.Error()
	_, ok := e.prev.(*checkpoint)
	if !ok {
		prevErrString = "File: unknown\n\t" + strings.ReplaceAll(prevErrString, "\n", "\n\t")
	}

	// Format different based on existing caller information.
	if e.callerOk {
		return fmt.Sprintf("File: %s:%d\n\t%v\n%v", e.file, e.line, e.err, prevErrString)
	}
	return fmt.Sprintf("File: unknown\n\t%v\n%v", e.err, prevErrString)
}

func (e *checkpoint) Unwrap() error {
	return e.prev
}

func (e *checkpoint) Is(target error) bool {
	return errors.Is(e.err, target)
}

func (e *checkpoint) As(target interface{}) bool {
	return errors.As(e.err, target)
}
