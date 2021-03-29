// Package emacs implements an emacs cache
package emacs

// TODO: this package should eventually deal with maintaining an emacs server.

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/leep-frog/commands/commands"
)

const (
	aliasArg      = "ALIAS"
	fileArg       = "FILE"
	emacsArg      = "EMACS_ARG"
	historicalArg = "COMMAND_IDX"
	regexpArg     = "REGEXP"
	newFileArg    = "new"

	fileAliaserName = "fileAliases"
)

var (
	osStat      = os.Stat
	filepathAbs = filepath.Abs
	//fileAliaser = commands.NewFileAliaser
	// This is in the var section so it can be stubbed out for tests.
	historyLimit = 25
)

type Emacs struct {
	// Aliases is a map from alias to full file path.
	Aliases            map[string]map[string]*commands.AliasedValues
	PreviousExecutions [][]string
	changed            bool
}

func (e *Emacs) AliasMap() map[string]map[string]*commands.AliasedValues {
	if e.Aliases == nil {
		e.Aliases = map[string]map[string]*commands.AliasedValues{}
	}
	return e.Aliases
}

func (e *Emacs) MarkChanged() {
	e.changed = true
}

// Name returns the name of the CLI.
func (e *Emacs) Name() string {
	return "emacs-shortcuts"
}

// Alias returns the CLI alias.
func (e *Emacs) Alias() string {
	return "e"
}

// Load creates an Emacs object from a JSON string.
func (e *Emacs) Load(jsn string) error {
	if jsn == "" {
		e = &Emacs{}
		return nil
	}

	if err := json.Unmarshal([]byte(jsn), e); err != nil {
		return fmt.Errorf("failed to unmarshal emacs json: %v", err)
	}
	return nil
}

type fileOpts struct {
	name       string
	lineNumber int
}

// TODO: add this as an option in aliasers. Specifically, if no args
// are provided, then run the last command.
// RunHistorical runs a previous command
func (e *Emacs) RunHistorical(ws *commands.WorldState) bool {
	if !ws.Values[historicalArg].Provided() {
		// print and return
		for idx, pe := range e.PreviousExecutions {
			revIdx := len(e.PreviousExecutions) - 1 - idx
			ws.Cos.Stdout(fmt.Sprintf("%2d: %s", revIdx, strings.Join(pe, " ")))
		}
		return true
	}

	idx := int(ws.Values[historicalArg].Int())
	// TODO: can this check be dynamic option (like IntNonNegative)?
	if idx >= len(e.PreviousExecutions) {
		ws.Cos.Stderr("%s is larger than list of stored commands", historicalArg)
		return false
	}

	ws.Executable = append(ws.Executable, e.PreviousExecutions[len(e.PreviousExecutions)-1-idx])
	return true
}

// OpenEditor constructs an emacs command to open the specified files.
func (e *Emacs) OpenEditor(ws *commands.WorldState) bool {
	allowNewFiles := ws.Values[newFileArg].Bool()
	ergs := ws.Values[emacsArg].StringList()

	if len(ergs) == 0 {
		if len(e.PreviousExecutions) == 0 {
			ws.Cos.Stderr("no previous executions")
			return false
		}
		ws.Executable = append(ws.Executable, e.PreviousExecutions[len(e.PreviousExecutions)-1])
		return true
	}

	// If only a directory was provided, then just cd into the directory.
	if len(ergs) == 1 {
		fi, _ := osStat(ergs[0])
		if fi != nil && fi.IsDir() {
			ws.Executable = append(ws.Executable, []string{"cd", ergs[0]})
			return true
		}
	}

	files := make([]*fileOpts, 0, len(ergs))
	var fo *fileOpts
	for _, erg := range ergs {
		if fo == nil {
			fo = &fileOpts{name: erg}
			continue
		}

		lineNumber, err := strconv.Atoi(erg)
		if err != nil {
			files = append(files, fo)
			fo = &fileOpts{name: erg}
			continue
		}

		fo.lineNumber = lineNumber
		files = append(files, fo)
		fo = nil
	}
	if fo != nil {
		files = append(files, fo)
	}

	sortedFiles := make([]*fileOpts, 0, len(ergs))
	for i := len(files) - 1; i >= 0; i-- {
		f := files[i]
		if name, ok := e.AliasMap()[fileAliaserName][f.name]; ok {
			// TODO: need to make Alias Command (already implemented) and Alias Arg (just for simple, but potentially multiple substitutions)
			// The only difference really is the number of args to iterate across.
			// So an alias for an entire command would be AliasNode(name, nodes, cli, 1)
			// whereas an alias for a partial command would be AliasNode(name, nodes, cli, 4)
			f.name = name.TODO
		} else {
			var err error
			f.name, err = filepathAbs(f.name)
			if err != nil {
				ws.Cos.Stderr("failed to get absolute path for file %q: %v", f.name, err)
				return false
			}
		}
		sortedFiles = append(sortedFiles, f)
	}

	// Check all files exist, unless --new flag provided.
	if !allowNewFiles {
		for _, fo := range files {
			if _, err := osStat(fo.name); os.IsNotExist(err) {
				ws.Cos.Stderr("file %q does not exist; include %q flag to create it", fo.name, newFileArg)
				return false
			}
		}
	}

	command := make([]string, 0, 1+2*len(sortedFiles))
	command = append(command, "emacs")
	command = append(command, "--no-window-system")
	for _, f := range sortedFiles {
		if f.lineNumber != 0 {
			command = append(command, fmt.Sprintf("+%d", f.lineNumber))
		}
		command = append(command, f.name)
	}

	e.changed = true
	e.PreviousExecutions = append(e.PreviousExecutions, command)
	if len(e.PreviousExecutions) > historyLimit {
		e.PreviousExecutions = e.PreviousExecutions[len(e.PreviousExecutions)-historyLimit:]
	}

	ws.Executable = append(ws.Executable, command)
	return true
}

func (e *Emacs) Changed() bool {
	return e.changed
}

func (e *Emacs) Option() *commands.Option { return nil }

func (e *Emacs) Node() *commands.Node {
	completor := &commands.Completor{
		SuggestionFetcher: &commands.FileFetcher{
			Distinct: true,
		},
	}

	return commands.BranchNode(
		map[string]*commands.Node{
			"h": commands.SerialNodes(
				commands.IntArg(historicalArg, false, &commands.ArgOpt{Validators: []commands.ArgValidator{commands.IntNonNegative()}}),
				commands.ExecutorNode(e.RunHistorical),
			),
		},
		commands.AliasNode("open-editor",
			commands.SerialNodes(
				commands.NewFlagNode(commands.BoolFlag(newFileArg, 'n')),
				commands.StringListNode(emacsArg, 0, 4, &commands.ArgOpt{Completor: completor}),
				commands.ExecutorNode(e.OpenEditor),
			),
			e,
		),
	)
}
