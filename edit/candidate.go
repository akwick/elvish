package edit

import (
	"fmt"
	"os"

	"github.com/elves/elvish/edit/ui"
	"github.com/elves/elvish/eval"
	"github.com/elves/elvish/parse"
)

type candidate struct {
	code string    // This is what will be substitued on the command line.
	menu ui.Styled // This is what is displayed in the completion menu.
}

// rawCandidate is what can be converted to a candidate.
type rawCandidate interface {
	eval.Value
	text() string
	cook(q parse.PrimaryType) *candidate
}

// plainCandidate is a minimal implementation of rawCandidate.
type plainCandidate string

func (plainCandidate) Kind() string        { return "string" }
func (p plainCandidate) Repr(l int) string { return eval.String(p).Repr(l) }

func (p plainCandidate) text() string { return string(p) }

func (p plainCandidate) cook(q parse.PrimaryType) *candidate {
	s := string(p)
	quoted, _ := parse.QuoteAs(s, q)
	return &candidate{code: quoted, menu: ui.Unstyled(s)}
}

// complexCandidate is an implementation of rawCandidate that offers
// customization options.
type complexCandidate struct {
	stem          string    // Used in the code and the menu.
	codeSuffix    string    // Appended to the code.
	displaySuffix string    // Appended to the display.
	style         ui.Styles // Used in the menu.
}

func (c *complexCandidate) Kind() string { return "map" }

func (c *complexCandidate) Repr(indent int) string {
	// TODO(xiaq): Pretty-print when indent >= 0
	return fmt.Sprintf("(edit:complex-candidate %s &code-suffix=%s &display-suffix=%s style=%s)",
		parse.Quote(c.stem), parse.Quote(c.codeSuffix),
		parse.Quote(c.displaySuffix), parse.Quote(c.style.String()))
}

func (c *complexCandidate) text() string { return c.stem }

func (c *complexCandidate) cook(q parse.PrimaryType) *candidate {
	quoted, _ := parse.QuoteAs(c.stem, q)
	return &candidate{
		code: quoted + c.codeSuffix,
		menu: ui.Styled{c.stem + c.displaySuffix, c.style},
	}
}

// outputComplexCandidate composes a complexCandidate.
func outputComplexCandidate(ec *eval.EvalCtx,
	args []eval.Value, opts map[string]eval.Value) {

	var style string
	c := &complexCandidate{}

	eval.ScanArgs(args, &c.stem)
	eval.ScanOpts(opts,
		eval.Opt{"code-suffix", &c.codeSuffix, eval.String("")},
		eval.Opt{"display-suffix", &c.displaySuffix, eval.String("")},
		eval.Opt{"style", &style, eval.String("")},
	)
	if style != "" {
		c.style = ui.StylesFromString(style)
	}

	ec.OutputChan() <- c
}

func (ed *Editor) filterAndCookCandidates(ev *eval.Evaler, completer string, pattern string,
	cands []rawCandidate, q parse.PrimaryType) ([]*candidate, error) {

	var filtered []*candidate
	matcher := ed.variables["-matcher"].Get().(eval.Map).IndexOne(eval.String(completer))
	m, ok := matcher.(eval.CallableValue)
	if !ok {
		return nil, errMatcherMustBeFn
	}

	input := make(chan eval.Value, len(cands))
	ports := []*eval.Port{
		{Chan: input}, {File: os.Stdout}, {File: os.Stderr}}
	ec := eval.NewTopEvalCtx(ev, "[editor matcher]", "", ports)

	args := []eval.Value{
		eval.String(pattern),
	}

	for _, cand := range cands {
		input <- eval.String(cand.text())
	}
	close(input)

	values, err := ec.PCaptureOutput(m, args, eval.NoOpts)
	if err != nil {
		return nil, err
	} else if len(values) != len(cands) {
		return nil, errIncorrectNumOfResults
	}
	for i, value := range values {
		if eval.ToBool(value) {
			filtered = append(filtered, cands[i].cook(q))
		}
	}
	return filtered, nil
}
