// Package emacs implements an emacs cache
package emacs

// TODO: this package should eventually deal with maintaining an emacs server.

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/leep-frog/commands/commands"
)

const (
	aliasArg      = "ALIAS"
	fileArg       = "FILE"
	emacsArg      = "EMACS_ARG"
	historicalArg = "COMMAND_IDX"
)

var (
	osGetwd     = os.Getwd
	osStat      = os.Stat
	filepathAbs = filepath.Abs
	// This is in the var section so it can be stubbed out for tests.
	historyLimit = 25
)

type Emacs struct {
	// Aliases is a map from alias to full file path.
	Aliases            map[string]string
	PreviousExecutions []*commands.ExecutorResponse
	changed            bool
}

// AddAlias creates a new emacs alias.
func (e *Emacs) AddAlias(cos commands.CommandOS, args, flags map[string]*commands.Value, _ *commands.OptionInfo) (*commands.ExecutorResponse, bool) {
	alias := *args[aliasArg].String()
	filename := *args[fileArg].String()

	if f, ok := e.Aliases[alias]; ok {
		cos.Stderr("alias already defined: (%s: %s)", alias, f)
		return nil, false
	}

	fileInfo, err := osStat(filename)
	if err != nil {
		cos.Stderr("error with file: %v", err)
		return nil, false
	}
	if fileInfo.Mode().IsDir() {
		cos.Stderr("%s is a directory", filename)
		return nil, false
	}

	absPath, err := filepathAbs(filename)
	if err != nil {
		cos.Stderr("failed to get absolute file path: %v", err)
		return nil, false
	}

	if e.Aliases == nil {
		e.Aliases = map[string]string{}
	}

	e.Aliases[alias] = absPath
	e.changed = true
	return nil, true
}

// DeleteAliases removes an existing emacs alias.
func (e *Emacs) DeleteAliases(cos commands.CommandOS, args, flags map[string]*commands.Value, _ *commands.OptionInfo) (*commands.ExecutorResponse, bool) {
	aliases := *args[aliasArg].StringList()
	for _, alias := range aliases {
		if _, ok := e.Aliases[alias]; !ok {
			cos.Stderr("alias %q does not exist", alias)
		} else {
			delete(e.Aliases, alias)
			e.changed = true
		}
	}

	return nil, true
}

// ListAliases removes an existing emacs alias.
func (e *Emacs) ListAliases(cos commands.CommandOS, _, _ map[string]*commands.Value, _ *commands.OptionInfo) (*commands.ExecutorResponse, bool) {
	keys := make([]string, 0, len(e.Aliases))
	for k := range e.Aliases {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, k := range keys {
		cos.Stdout("%s: %s", k, e.Aliases[k])
	}

	return nil, true
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
	if args[historicalArg].Int() == nil {
		// print and return
		for idx, pe := range e.PreviousExecutions {
			revIdx := len(e.PreviousExecutions) - 1 - idx
			cos.Stdout(fmt.Sprintf("%2d: %s", revIdx, strings.Join(pe.Executable, " ")))
		}
		return nil, true
	}

	idx := *args[historicalArg].Int()
	// TODO: can this check be dynamic option (like IntNonNegative)?
	if idx >= len(e.PreviousExecutions) {
		cos.Stderr("%s is larger than list of stored commands", historicalArg)
		return nil, false
	}

	return e.PreviousExecutions[len(e.PreviousExecutions)-1-idx], true
}

// OpenEditor constructs an emacs command to open the specified files.
func (e *Emacs) OpenEditor(cos commands.CommandOS, args, flags map[string]*commands.Value, _ *commands.OptionInfo) (*commands.ExecutorResponse, bool) {
	var ergs []string
	if ptr := args[emacsArg].StringList(); ptr != nil {
		ergs = *ptr
	}

	if len(ergs) == 0 {
		if len(e.PreviousExecutions) == 0 {
			cos.Stderr("no previous executions")
			return nil, false
		}
		return e.PreviousExecutions[len(e.PreviousExecutions)-1], true
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

	command := make([]string, 0, 1+2*len(files))
	command = append(command, "emacs")
	command = append(command, "--no-window-system")
	fileIdxs := make([]int, 0, len(files))
	for i := len(files) - 1; i >= 0; i-- {
		f := files[i]
		if f.lineNumber != 0 {
			command = append(command, fmt.Sprintf("+%d", f.lineNumber))
		}
		if name, ok := e.Aliases[f.name]; ok {
			command = append(command, name)
		} else {
			command = append(command, f.name)
			// Don't set this for aliases because those are already absolute paths.
			fileIdxs = append(fileIdxs, len(command)-1)
		}
	}

	if cwd, err := osGetwd(); err != nil {
		cos.Stderr("failed to get current directory: %v", err)
	} else {
		absCommand := make([]string, len(command))
		copy(absCommand, command)
		for _, idx := range fileIdxs {
			absCommand[idx] = filepath.Join(cwd, command[idx])
		}
		e.changed = true
		e.PreviousExecutions = append(e.PreviousExecutions, &commands.ExecutorResponse{Executable: absCommand})
		if len(e.PreviousExecutions) > historyLimit {
			e.PreviousExecutions = e.PreviousExecutions[len(e.PreviousExecutions)-historyLimit:]
		}
	}

	return &commands.ExecutorResponse{Executable: command}, true
}

func (e *Emacs) Changed() bool {
	return e.changed
}

func (e *Emacs) Option() *commands.Option { return nil }

type aliasFetcher struct {
	emacs *Emacs
}

func (af *aliasFetcher) Fetch(value *commands.Value, args, flags map[string]*commands.Value) *commands.Completion {
	suggestions := make([]string, 0, len(af.emacs.Aliases))
	for k := range af.emacs.Aliases {
		suggestions = append(suggestions, k)
	}
	return &commands.Completion{
		Suggestions: suggestions,
	}
}

// Command defines the emacs command and subcommands.
func (e *Emacs) Command() commands.Command {
	completor := &commands.Completor{
		Distinct:          true,
		SuggestionFetcher: &commands.FileFetcher{},
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
		},
		Subcommands: map[string]commands.Command{
			// AddAlias
			"a": &commands.TerminusCommand{
				Args: []commands.Arg{
					// TODO: list of pairs.
					commands.StringArg(aliasArg, true, nil),
					commands.StringArg(fileArg, true, &commands.Completor{
						SuggestionFetcher: &commands.FileFetcher{},
					}),
				},
				Executor: e.AddAlias,
			},
			// DeleteAliases
			"d": &commands.TerminusCommand{
				Executor: e.DeleteAliases,
				Args: []commands.Arg{
					commands.StringListArg(aliasArg, 1, -1, &commands.Completor{
						SuggestionFetcher: &aliasFetcher{
							emacs: e,
						},
					}),
				},
			},
			// ListAliases
			"l": &commands.TerminusCommand{
				Executor: e.ListAliases,
			},
			// Run earlier command
			"h": &commands.TerminusCommand{
				Executor: e.RunHistorical,
				Args: []commands.Arg{
					commands.IntArg(historicalArg, false, nil, commands.IntNonNegative()),
				},
			},
		},
	}
}
