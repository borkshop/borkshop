package anansi

import (
	"os"
)

// NewTerm creates a new Term attached to the given file, and with optional
// associated context.
func NewTerm(f *os.File, cs ...Context) *Term {
	term := &Term{File: f}
	term.initContext()
	term.ctx = Contexts(term.ctx, Contexts(cs...))
	return term
}

// Term combines a terminal file handle with attribute control and further
// Context-ual state.
type Term struct {
	*os.File
	Attr
	Mode

	active bool
	ctx    Context
}

func (term *Term) initContext() {
	if term.ctx == nil {
		term.ctx = Contexts(
			&term.Attr,
			&term.Mode)
	}
}

// RunWith runs the given function within the terminal's context, Enter()ing it
// if necessary, and Exit()ing it if Enter() was called after the given
// function returns. Exit() is called even if the within function returns an
// error or panics.
func (term *Term) RunWith(within func(*Term) error) (err error) {
	if term.active {
		return within(term)
	}
	term.initContext()
	defer func() {
		if cerr := term.ctx.Exit(term); cerr == nil {
			err = cerr
		}
		term.active = false
	}()
	term.active = true
	if err = term.ctx.Enter(term); err == nil {
		err = within(term)
	}
	return err
}

// RunWithout runs the given function without the terminal's context, Exit()ing
// it if necessary, and Enter()ing it if deactivation was necessary.
// Re-Enter() is not called is not done if a non-nil error is returned, or if
// the without function panics.
func (term *Term) RunWithout(without func(*Term) error) (err error) {
	if !term.active {
		return without(term)
	}
	if err = term.ctx.Exit(term); err == nil {
		term.active = false
		if err = without(term); err == nil {
			if err = term.ctx.Enter(term); err == nil {
				term.active = true
			}
		}
	}
	return err
}
