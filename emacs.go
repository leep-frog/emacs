// Package emacs implements an emacs cache
package emacs

// TODO: this package should eventually deal with maintaining an emacs server.

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"

	"github.com/leep-frog/command"
)

const (
	aliasArg      = "ALIAS"
	fileArg       = "FILE"
	emacsArg      = "EMACS_ARG"
	lineArg       = "LINE_NUMBER"
	historicalArg = "COMMAND_IDX"
	regexpArg     = "REGEXP"
	newFileArg    = "new"

	fileAliaserName = "fileAliases"
	cacheName       = "emacsCache"
)

var (
	//fileAliaser = commands.NewFileAliaser
	// This is in the var section so it can be stubbed out for tests.
	historyLimit = 25
)

func CLI() *Emacs {
	return &Emacs{}
}

type Emacs struct {
	// Aliases is a map from alias to full file path.
	Aliases map[string]map[string][]string
	changed bool
	Caches  map[string][]string
}

func (e *Emacs) AliasMap() map[string]map[string][]string {
	if e.Aliases == nil {
		e.Aliases = map[string]map[string][]string{}
	}
	return e.Aliases
}

func (e *Emacs) Setup() []string { return nil }

func (e *Emacs) MarkChanged() {
	e.changed = true
}

// Name returns the name of the CLI.
func (e *Emacs) Name() string {
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
func (e *Emacs) OpenEditor(input *command.Input, output command.Output, data *command.Data, eData *command.ExecuteData) error {
	allowNewFiles := data.Values[newFileArg].Bool()
	ergs := data.Values[emacsArg].StringList()

	// If only a directory was provided, then just cd into the directory.
	if len(ergs) == 1 {
		fi, _ := os.Stat(ergs[0])
		if fi != nil && fi.IsDir() {
			eData.Executable = append(eData.Executable, []string{"cd", ergs[0]})
			return nil
		}
	}

	files := make([]*fileOpts, 0, len(ergs))
	il := data.Values[lineArg].IntList()
	for i := len(ergs) - 1; i >= 0; i-- {
		erg := ergs[i]
		// Check file exists, unless --new flag provided.
		if !allowNewFiles {
			if _, err := os.Stat(erg); os.IsNotExist(err) {
				return output.Stderr("file %q does not exist; include %q flag to create it", erg, newFileArg)
			}
		}

		var iv int
		if i < len(il) {
			iv = il[i]
		}
		files = append(files, &fileOpts{erg, iv})
	}

	cmd := make([]string, 0, 1+2*len(files))
	cmd = append(cmd, "emacs")
	cmd = append(cmd, "--no-window-system")
	for _, f := range files {
		if f.lineNumber != 0 {
			cmd = append(cmd, fmt.Sprintf("+%d", f.lineNumber))
		}
		cmd = append(cmd, f.name)
	}

	eData.Executable = append(eData.Executable, cmd)
	return nil
}

func (e *Emacs) Changed() bool {
	return e.changed
}

func (e *Emacs) Cache() map[string][]string {
	if e.Caches == nil {
		e.Caches = map[string][]string{}
	}
	return e.Caches
}

func (e *Emacs) Node() *command.Node {
	// We don't want to cache alias commands. Hence why it comes after.
	return command.AliasNode(fileAliaserName, e, command.CacheNode(cacheName, e, e.emacsArgNode()))
}

func (e *Emacs) emacsArgNode() *command.Node {
	completor := &command.Completor{
		Distinct: true,
		SuggestionFetcher: &command.FileFetcher{
			Distinct: true,
			IgnoreFunc: func(v *command.Value, d *command.Data) []string {
				return d.Values[emacsArg].StringList()
			},
		},
	}

	opt := &command.ArgOpt{
		Alias: &command.AliasOpt{
			AliasName: fileAliaserName,
			AliasCLI:  e,
		},
		Completor:   completor,
		Transformer: command.FileTransformer(),
		CustomSet: func(v *command.Value, d *command.Data) {
			// TODO: CustomSet shouldn't be run if v wasn't provided.
			// fix this in command package.
			if !v.Provided() {
				return
			}
			slv, ok := d.Values[emacsArg]
			if !ok {
				d.Set(emacsArg, command.StringListValue(v.String()))
				return
			} else {
				d.Set(emacsArg, command.StringListValue(append(slv.StringList(), v.String())...))
			}
		},
	}

	intOpt := &command.ArgOpt{
		CustomSet: func(v *command.Value, d *command.Data) {
			sl := d.Values[emacsArg].StringList()
			il := d.Values[lineArg].IntList()
			for i := len(il); i < len(sl)-1; i++ {
				il = append(il, 0)
			}
			il = append(il, v.Int())
			d.Set(lineArg, command.IntListValue(il...))
		},
	}

	n := &command.Node{
		Processor: command.StringNode(emacsArg, opt),
	}
	in := &command.Node{
		Processor: command.IntNode(lineArg, intOpt),
		//Edge:      command.SimpleEdge(n),
	}
	next := command.SerialNodes(command.SimpleProcessor(e.OpenEditor, nil))
	n.Edge = &emacsEdge{
		next:    next,
		eNode:   n,
		intNode: in,
	}
	in.Edge = &intEdge{
		next:  next,
		eNode: n,
	}

	return command.SerialNodesTo(n, command.NewFlagNode(command.BoolFlag(newFileArg, 'n')))
}

type intEdge struct {
	next  *command.Node
	eNode *command.Node
}

func (ie *intEdge) Next(input *command.Input, data *command.Data) (*command.Node, error) {
	if _, ok := input.Peek(); !ok {
		return ie.next, nil
	}
	return ie.eNode, nil
}

// TODO: make helper function command.EdgeFromFunc(func(...) (node, error)) {...}
type emacsEdge struct {
	next    *command.Node
	eNode   *command.Node
	intNode *command.Node
}

func (ee *emacsEdge) Next(input *command.Input, data *command.Data) (*command.Node, error) {
	s, ok := input.Peek()
	if !ok {
		return ee.next, nil
	}

	if _, err := strconv.Atoi(s); err == nil {
		return ee.intNode, nil
	}

	if len(data.Values[emacsArg].StringList()) >= 2 {
		return ee.next, nil
	}

	return ee.eNode, nil
}
