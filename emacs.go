// Package emacs implements an emacs cache
package emacs

// TODO: this package should eventually deal with maintaining an emacs server.

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
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
	regexpArg     = "REGEXP"
	newFileArg    = "new"
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

// GetAlias
func (e *Emacs) GetAlias(cos commands.CommandOS, args, flags map[string]*commands.Value, _ *commands.OptionInfo) (*commands.ExecutorResponse, bool) {
	alias := *args[aliasArg].String()
	f, ok := e.Aliases[alias]
	if ok {
		cos.Stdout("%s: %s", alias, f)
	} else {
		cos.Stderr("Alias %q does not exist", alias)
	}
	return nil, ok
}

// SearchAliases
func (e *Emacs) SearchAliases(cos commands.CommandOS, args, flags map[string]*commands.Value, _ *commands.OptionInfo) (*commands.ExecutorResponse, bool) {
	searchRegex, err := regexp.Compile(*args[regexpArg].String())
	if err != nil {
		cos.Stderr("Invalid regexp: %v", err)
		return nil, false
	}

	var as []string
	for a := range e.Aliases {
		as = append(as, a)
	}
	sort.Strings(as)
	for _, a := range as {
		f := e.Aliases[a]
		if searchRegex.MatchString(f) {
			cos.Stdout("%s: %s", a, f)
		}
	}
	return nil, true
}

// AddAlias creates a new emacs alias.
func (e *Emacs) AddAlias(cos commands.CommandOS, args, flags map[string]*commands.Value, _ *commands.OptionInfo) (*commands.ExecutorResponse, bool) {
	alias := *args[aliasArg].String()
	filename := *args[fileArg].String()

	if f, ok := e.Aliases[alias]; ok {
		cos.Stderr("alias already defined: (%s: %s)", alias, f)
		return nil, false
	}

	if _, err := osStat(filename); err != nil {
		cos.Stderr("file does not exist: %v", err)
		return nil, false
	}

	absPath, err := filepathAbs(filename)
	if err != nil {
		cos.Stderr("failed to get absolute file path for file %q: %v", filename, err)
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
	allowNewFiles := flags[newFileArg].Bool() != nil && *flags[newFileArg].Bool()
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
			f.name = name
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
		SuggestionFetcher: &commands.FileFetcher{
			Distinct: true,
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
			// GetAlias
			"g": &commands.TerminusCommand{
				Executor: e.GetAlias,
				Args: []commands.Arg{
					commands.StringArg(aliasArg, true, &commands.Completor{
						SuggestionFetcher: &aliasFetcher{
							emacs: e,
						},
					}),
				},
			},
			// SearchAliases
			"s": &commands.TerminusCommand{
				Executor: e.SearchAliases,
				Args: []commands.Arg{
					commands.StringArg(regexpArg, true, nil),
				},
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
