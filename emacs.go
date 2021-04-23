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

	"github.com/leep-frog/command"
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
	//fileAliaser = commands.NewFileAliaser
	// This is in the var section so it can be stubbed out for tests.
	historyLimit = 25
)

type Emacs struct {
	// Aliases is a map from alias to full file path.
	Aliases            map[string]map[string][]string
	PreviousExecutions [][]string
	changed            bool
}

func (e *Emacs) AliasMap() map[string]map[string][]string {
	if e.Aliases == nil {
		e.Aliases = map[string]map[string][]string{}
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
func (e *Emacs) RunHistorical(input *command.Input, output command.Output, data *command.Data, eData *command.ExecuteData) error {
	if !data.Values[historicalArg].Provided() {
		// print and return
		for idx, pe := range e.PreviousExecutions {
			revIdx := len(e.PreviousExecutions) - 1 - idx
			output.Stdout("%2d: %s", revIdx, strings.Join(pe, " "))
		}
		return nil
	}

	idx := data.Values[historicalArg].Int()
	// TODO: can this check be dynamic option (like IntNonNegative)?
	if idx >= len(e.PreviousExecutions) {
		return output.Stderr("%s is larger than list of stored commands", historicalArg)
	}

	eData.Executable = append(eData.Executable, e.PreviousExecutions[len(e.PreviousExecutions)-1-idx])
	return nil
}

// OpenEditor constructs an emacs command to open the specified files.
func (e *Emacs) OpenEditor(input *command.Input, output command.Output, data *command.Data, eData *command.ExecuteData) error {
	allowNewFiles := data.Values[newFileArg].Bool()
	ergs := data.Values[emacsArg].StringList()

	if len(ergs) == 0 {
		if len(e.PreviousExecutions) == 0 {
			return output.Stderr("no previous executions")
		}
		eData.Executable = append(eData.Executable, e.PreviousExecutions[len(e.PreviousExecutions)-1])
		return nil
	}

	// If only a directory was provided, then just cd into the directory.
	if len(ergs) == 1 {
		fi, _ := os.Stat(ergs[0])
		if fi != nil && fi.IsDir() {
			eData.Executable = append(eData.Executable, []string{"cd", ergs[0]})
			return nil
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
		var err error
		f.name, err = filepath.Abs(f.name)
		if err != nil {
			return output.Stderr("failed to get absolute path for file %q: %v", f.name, err)
		}
		sortedFiles = append(sortedFiles, f)
	}

	// Check all files exist, unless --new flag provided.
	if !allowNewFiles {
		for _, fo := range files {
			if _, err := os.Stat(fo.name); os.IsNotExist(err) {
				return output.Stderr("file %q does not exist; include %q flag to create it", fo.name, newFileArg)
			}
		}
	}

	cmd := make([]string, 0, 1+2*len(sortedFiles))
	cmd = append(cmd, "emacs")
	cmd = append(cmd, "--no-window-system")
	for _, f := range sortedFiles {
		if f.lineNumber != 0 {
			cmd = append(cmd, fmt.Sprintf("+%d", f.lineNumber))
		}
		cmd = append(cmd, f.name)
	}

	// We only want to run changes afterwards.
	eData.Executor = func(output command.Output, data *command.Data) error {
		e.changed = true
		e.PreviousExecutions = append(e.PreviousExecutions, cmd)
		if len(e.PreviousExecutions) > historyLimit {
			e.PreviousExecutions = e.PreviousExecutions[len(e.PreviousExecutions)-historyLimit:]
		}
		return nil
	}

	eData.Executable = append(eData.Executable, cmd)
	return nil
}

func (e *Emacs) Changed() bool {
	return e.changed
}

//func (e *Emacs) Option() *command.Option { return nil }

func (e *Emacs) Node() *command.Node {
	completor := &command.Completor{
		SuggestionFetcher: &command.FileFetcher{
			Distinct: true,
		},
	}

	return command.BranchNode(
		map[string]*command.Node{
			"h": command.SerialNodes(
				command.OptionalIntNode(historicalArg, &command.ArgOpt{Validators: []command.ArgValidator{command.IntNonNegative()}}),
				command.SimpleProcessor(e.RunHistorical, nil),
			),
		},
		command.AliasNode(fileAliaserName, e,
			command.SerialNodes(
				command.NewFlagNode(command.BoolFlag(newFileArg, 'n')),
				command.StringListNode(emacsArg, 0, 4, &command.ArgOpt{
					Completor:   completor,
					Transformer: command.FileListTransformer(),
				}),
				command.SimpleProcessor(e.OpenEditor, nil),
			),
		),
	)
}
