package emacs

import (
	"fmt"
	"strings"
)

func basic(fos ...*fileOpts) []string {
	r := make([]string, 0, 1+2*len(fos))
	r = append(r, "emacs", "--no-window-system")
	// Reverse order.
	for i := len(fos) - 1; i >= 0; i-- {
		f := fos[i]
		if f.lineNumber != 0 {
			r = append(r, fmt.Sprintf("+%d", f.lineNumber))
		}
		r = append(r, f.name)
	}

	return r
}

func daemon(fos ...*fileOpts) []string {
	var eCmds []string
	for _, fo := range fos {
		eCmds = append(eCmds, fmt.Sprintf(`(find-file "%s")`, fo.name))
		if fo.lineNumber != 0 {
			eCmds = append(eCmds, fmt.Sprintf(`(goto-line %d)`, fo.lineNumber))
		}
	}
	if len(fos) == 2 {
		eCmds = append(eCmds, `(other-window)`)
	}
	return []string{
		// TODO: add daemon initializer code.
		"emacsclient",
		"-t",
		"-e",
		fmt.Sprintf("'%s'", strings.Join(eCmds, "")),
	}
}
