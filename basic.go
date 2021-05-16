package emacs

import (
	"fmt"
	"strings"
)

func basic(debugInit bool, fos ...*fileOpts) ([]string, error) {
	r := make([]string, 0, 1+2*len(fos))
	r = append(r, "emacs", "--no-window-system")
	if debugInit {
		r = append(r, "--debug-init")
	}
	// Reverse order.
	for i := len(fos) - 1; i >= 0; i-- {
		f := fos[i]
		if f.lineNumber != 0 {
			r = append(r, fmt.Sprintf("+%d", f.lineNumber))
		}
		r = append(r, f.name)
	}

	return r, nil
}

func daemon(debugInit bool, fos ...*fileOpts) ([]string, error) {
	if debugInit {
		return nil, fmt.Errorf("--debug-init flag is not allowed in daemon mode")
	}
	var eCmds []string
	findCmd := "find-file"
	for _, fo := range fos {
		eCmds = append(eCmds, fmt.Sprintf(`(%s "%s")`, findCmd, fo.name))
		if fo.lineNumber != 0 {
			eCmds = append(eCmds, fmt.Sprintf(`(goto-line %d)`, fo.lineNumber))
		}
		findCmd = "find-file-other-window"
	}
	if len(fos) == 2 {
		eCmds = append(eCmds, `(other-window 1)`)
	}
	return []string{
		// TODO: add daemon initializer code.
		"emacsclient",
		"-t",
		"-e",
		fmt.Sprintf("'(progn %s)'", strings.Join(eCmds, "")),
	}, nil
}
