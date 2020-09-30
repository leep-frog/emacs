// Package emacs implements an emacs cache
package emacs

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"

	"github.com/leep-frog/cli/commands"
)

const (
	aliasArg = "ALIAS"
	fileArg  = "FILE"
	emacsArg = "EMACS_ARG"
)

var (
	osStat      = os.Stat
	filepathAbs = filepath.Abs
)

type Emacs struct {
	// Aliases is a map from alias to full file path.
	Aliases map[string]string
	changed bool
}

// AddAlias creates a new emacs alias.
func (e *Emacs) AddAlias(args, flags map[string]*commands.Value) (*commands.ExecutorResponse, error) {
	alias := *args[aliasArg].String()
	filename := *args[fileArg].String()

	if f, ok := e.Aliases[alias]; ok {
		// TODO: just return stderr?
		return nil, fmt.Errorf("alias already defined: (%s: %s)", alias, f)
	}

	fileInfo, err := osStat(filename)
	if err != nil {
		return nil, fmt.Errorf("error with file: %v", err)
	}
	if fileInfo.Mode().IsDir() {
		return nil, fmt.Errorf("%s is a directory", filename)
	}

	absPath, err := filepathAbs(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to get absolute file path: %v", err)
	}

	if e.Aliases == nil {
		e.Aliases = map[string]string{}
	}

	e.Aliases[alias] = absPath
	e.changed = true
	return &commands.ExecutorResponse{}, nil
}

// DeleteAliases removes an existing emacs alias.
func (e *Emacs) DeleteAliases(args, flags map[string]*commands.Value) (*commands.ExecutorResponse, error) {
	aliases := *args[aliasArg].StringList()
	errors := make([]string, 0, len(aliases))
	for _, alias := range aliases {
		// TODO: check if Aliases is nil?
		if _, ok := e.Aliases[alias]; !ok {
			errors = append(errors, fmt.Sprintf("alias %q does not exist", alias))
		} else {
			delete(e.Aliases, alias)
			e.changed = true
		}
	}

	resp := &commands.ExecutorResponse{}
	if len(errors) > 0 {
		resp.Stderr = errors
	}
	return resp, nil
}

// ListAliases removes an existing emacs alias.
func (e *Emacs) ListAliases(_, _ map[string]*commands.Value) (*commands.ExecutorResponse, error) {
	keys := make([]string, 0, len(e.Aliases))
	for k := range e.Aliases {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	output := make([]string, 0, len(keys))
	for _, k := range keys {
		output = append(output, fmt.Sprintf("%s: %s", k, e.Aliases[k]))
	}

	resp := &commands.ExecutorResponse{}
	if len(output) > 0 {
		resp.Stdout = output
	}
	return resp, nil
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

// OpenEditor constructs an emacs command to open the specified files.
func (e *Emacs) OpenEditor(args, flags map[string]*commands.Value) (*commands.ExecutorResponse, error) {
	ergs := *args[emacsArg].StringList()

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
	for _, f := range files {
		if f.lineNumber != 0 {
			command = append(command, fmt.Sprintf("+%d", f.lineNumber))
		}
		if name, ok := e.Aliases[f.name]; ok {
			command = append(command, name)
		} else {
			command = append(command, f.name)
		}
	}
	return &commands.ExecutorResponse{Executable: command}, nil
}

func (e *Emacs) Changed() bool {
	return e.changed
}

type aliasFetcher struct {
	emacs *Emacs
}

func (af *aliasFetcher) Fetch(value *commands.Value, args, flags map[string]*commands.Value) []string {
	suggestions := make([]string, 0, len(af.emacs.Aliases))
	for k := range af.emacs.Aliases {
		suggestions = append(suggestions, k)
	}
	return suggestions
}

// Command defines the emacs command and subcommands.
func (e *Emacs) Command() commands.Command {
	completor := &commands.Completor{
		Distinct: true,
		SuggestionFetcher: &commands.FileFetcher{},
	}
	// TODO: add option to ignore subcommand suggestions.
	return &commands.CommandBranch{
		TerminusCommand: &commands.TerminusCommand{
			Executor: e.OpenEditor,
			Args: []commands.Arg{
				// TODO filename for first command
				// any for second
				commands.StringListArg(emacsArg, 1, 3, completor),
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
		},
	}
}
