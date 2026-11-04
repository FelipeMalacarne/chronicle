package content

import (
	"errors"
	"fmt"
	"strings"
)

// Issue is one validation problem located by file and JSON pointer.
type Issue struct {
	File    string `json:"file"`
	Pointer string `json:"pointer"`
	Message string `json:"message"`
}

func (i Issue) Error() string {
	p := i.Pointer
	if p == "" {
		p = "/"
	}
	if i.File == "" {
		return fmt.Sprintf("%s: %s", p, i.Message)
	}
	return fmt.Sprintf("%s: %s: %s", i.File, p, i.Message)
}

// Errors is the list of issues found while loading or validating content.
// It is returned as a plain error; recover it with errors.As.
type Errors []Issue

func (e Errors) Error() string {
	switch len(e) {
	case 0:
		return "content: no errors"
	case 1:
		return "content: " + e[0].Error()
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "content: %d errors:", len(e))
	for _, i := range e {
		sb.WriteString("\n  ")
		sb.WriteString(i.Error())
	}
	return sb.String()
}

// AsErrors extracts the issue list from err, if it carries one.
func AsErrors(err error) (Errors, bool) {
	var e Errors
	if errors.As(err, &e) {
		return e, true
	}
	return nil, false
}

// errOrNil returns e as an error only when it holds issues.
func (e Errors) errOrNil() error {
	if len(e) == 0 {
		return nil
	}
	return e
}
