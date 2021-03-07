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
)

var (
	osStat      = os.Stat
	filepathAbs = filepath.Abs
	fileAliaser = commands.NewFileAliaser
	// This is in the var section so it can be stubbed out for tests.
	historyLimit = 25
)

type Emacs struct {
	// Aliases is a map from alias to full file path.
	Aliases            map[string]*commands.Value
	PreviousExecutions []*commands.ExecutorResponse
	changed            bool
}

func (e *Emacs) GetAlias(s string) (*commands.Value, bool) {
	v, ok := e.Aliases[s]
	return v, ok
}

func (e *Emacs) SetAlias(s string, v *commands.Value) {
	if e.Aliases == nil {
		e.Aliases = map[string]*commands.Value{}
	}
	e.Aliases[s] = v
	e.changed = true
	return
}

func (e *Emacs) DeleteAlias(s string) {
	delete(e.Aliases, s)
	e.changed = true
	return
}

func (e *Emacs) AllAliases() []string {
	ss := make([]string, 0, len(e.Aliases))
	for k := range e.Aliases {
		ss = append(ss, k)
	}
	return ss
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

// RunHistorical runs a previous command
func (e *Emacs) RunHistorical(cos commands.CommandOS, args, flags map[string]*commands.Value, _ *commands.OptionInfo) (*commands.ExecutorResponse, bool) {
	if !args[historicalArg].Provided() {
		// print and return
		for idx, pe := range e.PreviousExecutions {
			revIdx := len(e.PreviousExecutions) - 1 - idx
			cos.Stdout(fmt.Sprintf("%2d: %s", revIdx, strings.Join(pe.Executable, " ")))
		}
		return nil, true
	}

	idx := int(args[historicalArg].Int())
	// TODO: can this check be dynamic option (like IntNonNegative)?
	if idx >= len(e.PreviousExecutions) {
		cos.Stderr("%s is larger than list of stored commands", historicalArg)
		return nil, false
	}

	return e.PreviousExecutions[len(e.PreviousExecutions)-1-idx], true
}

// OpenEditor constructs an emacs command to open the specified files.
func (e *Emacs) OpenEditor(cos commands.CommandOS, args, flags map[string]*commands.Value, _ *commands.OptionInfo) (*commands.ExecutorResponse, bool) {
	allowNewFiles := flags[newFileArg].Provided() && flags[newFileArg].Bool()
	ergs := args[emacsArg].StringList()

	if len(ergs) == 0 {
		if len(e.PreviousExecutions) == 0 {
			cos.Stderr("no previous executions")
			return nil, false
		}
		return e.PreviousExecutions[len(e.PreviousExecutions)-1], true
	}

	// If only a directory was provided, then just cd into the directory.
	if len(ergs) == 1 {
		fi, _ := osStat(ergs[0])
		if fi != nil && fi.IsDir() {
			return &commands.ExecutorResponse{
				Executable: []string{"cd", ergs[0]},
			}, true
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
		if name, ok := e.Aliases[f.name]; ok {
			f.name = name.String()
		} else {
			var err error
			f.name, err = filepathAbs(f.name)
			if err != nil {
				cos.Stderr("failed to get absolute path for file %q: %v", f.name, err)
				return nil, false
			}
		}
		sortedFiles = append(sortedFiles, f)
	}

	// Check all files exist, unless --new flag provided.
	if !allowNewFiles {
		for _, fo := range files {
			if _, err := osStat(fo.name); os.IsNotExist(err) {
				cos.Stderr("file %q does not exist; include %q flag to create it", fo.name, newFileArg)
				return nil, false
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
	e.PreviousExecutions = append(e.PreviousExecutions, &commands.ExecutorResponse{Executable: command})
	if len(e.PreviousExecutions) > historyLimit {
		e.PreviousExecutions = e.PreviousExecutions[len(e.PreviousExecutions)-historyLimit:]
	}

	return &commands.ExecutorResponse{Executable: command}, true
}

func (e *Emacs) Changed() bool {
	return e.changed
}

func (e *Emacs) Option() *commands.Option { return nil }

// Command defines the emacs command and subcommands.
func (e *Emacs) Command() commands.Command {
	completor := &commands.Completor{
		SuggestionFetcher: &commands.FileFetcher{
			Distinct: true,
		},
	}

	scs := commands.AliasSubcommands(e, fileAliaser())
	// Run earlier command
	scs["h"] = &commands.TerminusCommand{
		Executor: e.RunHistorical,
		Args: []commands.Arg{
			commands.IntArg(historicalArg, false, nil, commands.IntNonNegative()),
		},
	}

	return &commands.CommandBranch{
		IgnoreSubcommandAutocomplete: true,
		TerminusCommand: &commands.TerminusCommand{
			Executor: e.OpenEditor,
			Args: []commands.Arg{
				// TODO filename for first command
				// any for second
				commands.StringListArg(emacsArg, 0, 4, completor),
			},
			Flags: []commands.Flag{
				commands.BoolFlag(newFileArg, 'n'),
			},
		},
		Subcommands: scs,
	}
}
