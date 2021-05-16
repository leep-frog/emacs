// Package emacs implements an emacs cache
package emacs

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"

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
	// This is in the var section so it can be stubbed out for tests.
	historyLimit = 25

	debugInitFlag = command.BoolFlag("debugInit", 'd')
)

func CLI() *Emacs {
	return &Emacs{}
}

type Emacs struct {
	// Aliases is a map from alias to full file path.
	Aliases map[string]map[string][]string
	changed bool
	Caches  map[string][]string

	DaemonMode bool
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
			eData.Executable = append(eData.Executable, fmt.Sprintf("cd %s", ergs[0]))
			return nil
		}
	}

	files := make([]*fileOpts, 0, len(ergs))
	il := data.Values[lineArg].IntList()
	for i, erg := range ergs {
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

	getCmd := basic
	if e.DaemonMode {
		getCmd = daemon
	}

	gotCmd, err := getCmd(data.Values[debugInitFlag.Name()].Bool(), files...)
	if err != nil {
		return output.Err(err)
	}

	eData.Executable = append(eData.Executable, gotCmd)
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

func (e *Emacs) AliasDotEl(output command.Output, data *command.Data) error {
	var aliases []string
	for k := range e.Aliases[fileAliaserName] {
		aliases = append(aliases, k)
	}
	sort.Strings(aliases)

	r := []string{
		"(setq aliasMap",
		"#s(hash-table",
		fmt.Sprintf("size %d", len(aliases)),
		"test equal",
		"data (",
	}
	for _, k := range aliases {
		r = append(r, fmt.Sprintf(`"%s" "%s"`, k, e.Aliases[fileAliaserName][k]))
	}
	r = append(r,
		")))",
		"",
		`(global-set-key (kbd "C-x C-j") (lambda () (interactive)`,
		`(setq a (read-string "Alias: "))`,
		`(setq v (gethash a aliasMap))`,
		`(if v (find-file v) (message "Unknown alias: %s" a))`,
		"))",
	)
	output.Stdout(strings.Join(r, "\n"))
	return nil
}

func (e *Emacs) Node() *command.Node {
	// We don't want to cache alias commands. Hence why it comes after.
	return command.BranchNode(
		// TODO: Make a settings node. But wait until we have more use
		// cases so we can get an idea of how to actual make that node useful.
		map[string]*command.Node{
			"el": command.SerialNodes(command.ExecutorNode(e.AliasDotEl)),
			"dae": command.SerialNodes(command.ExecutorNode(func(output command.Output, _ *command.Data) error {
				e.DaemonMode = !e.DaemonMode
				e.MarkChanged()
				if e.DaemonMode {
					output.Stdout("Daemon mode activated.")
				} else {
					output.Stdout("Daemon mode deactivated.")
				}
				return nil
			})),
			"dk": command.SerialNodes(command.SimpleProcessor(func(input *command.Input, output command.Output, _ *command.Data, eData *command.ExecuteData) error {
				eData.Executable = append(eData.Executable,
					"echo Killing emacs daemon",
					"emacsclient -e '(kill-emacs)'",
					"echo Success!",
				)
				return nil
			}, nil)),
			"ds": command.SerialNodes(command.SimpleProcessor(func(input *command.Input, output command.Output, _ *command.Data, eData *command.ExecuteData) error {
				eData.Executable = append(eData.Executable,
					"echo Starting emacs daemon",
					"emacs --daemon",
					"echo Success!",
				)
				return nil
			}, nil)),
		},
		command.AliasNode(fileAliaserName, e, command.CacheNode(cacheName, e, e.emacsArgNode())),
		false,
	)
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

	return command.SerialNodesTo(n,
		command.NewFlagNode(
			command.BoolFlag(newFileArg, 'n'),
			debugInitFlag,
		),
	)
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
