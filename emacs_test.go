package emacs

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/leep-frog/commands/commands"
	"github.com/leep-frog/commands/commandtest"
	"google.golang.org/protobuf/testing/protocmp"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
)

func TestLoad(t *testing.T) {
	for _, test := range []struct {
		name    string
		json    string
		want    *Emacs
		wantErr string
	}{
		{
			name: "handles empty string",
			want: &Emacs{},
		},
		{
			name:    "errors on invalid json",
			json:    "}",
			want:    &Emacs{},
			wantErr: "failed to unmarshal emacs json",
		},
		{
			name: "properly unmarshals",
			json: fmt.Sprintf(`{"Aliases":{"%s":{"city":{"Type":"String","String":"catan/oreAndWheat"}}},"PreviousExecutions":null}`, fileAliaserName),
			want: &Emacs{
				Aliases: map[string]map[string]*commands.Value{fileAliaserName: map[string]*commands.Value{
					"city": commands.StringValue("catan/oreAndWheat"),
				},
				},
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			e := &Emacs{}

			err := e.Load(test.json)
			if err != nil && test.wantErr == "" {
				t.Fatalf("Load(%v) returned error (%v); want nil", test.json, err)
			} else if err == nil && test.wantErr != "" {
				t.Fatalf("Load(%v) returned nil; want error (%v)", test.json, test.wantErr)
			} else if err != nil && test.wantErr != "" && !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("Load(%v) returned error (%v); want (%v)", test.json, err, test.wantErr)
			}

			if diff := cmp.Diff(test.want, e, cmpopts.IgnoreUnexported(Emacs{})); diff != "" {
				t.Errorf("Load(%v) produced emacs diff (-want, +got):\n%s", test.json, diff)
			}
		})
	}
}

/*func TestAutocomplete(t *testing.T) {
	e := &Emacs{
		Aliases: map[string]map[string]*commands.Value{fileAliaserName: map[string]*commands.Value{
			"salt": commands.StringValue("compounds/sodiumChloride"),
			"city": commands.StringValue("catan/oreAndWheat"),
		},
		},
	}

	for _, test := range []struct {
		name string
		args []string
		want []string
	}{
		{
			name: "doesn't suggest subcommands",
			want: []string{
				".git/",
				"emacs.go",
				"emacs_test.go",
				"go.mod",
				"go.sum",
				"testing/",
				" ",
			},
		},
		{
			name: "file suggestions ignore case",
			args: []string{"EmA"},
			want: []string{
				"emacs",
				"emacs_",
			},
		},
		{
			name: "suggests only files after first command",
			args: []string{"file1.txt", ""},
			want: []string{
				".git/",
				"emacs.go",
				"emacs_test.go",
				"go.mod",
				"go.sum",
				"testing/",
				" ",
			},
		},
		{
			name: "doesn't include files already included",
			args: []string{"emacs.go", "e"},
			want: []string{
				"emacs_test.go",
			},
		},
		{
			name: "doesn't include files a directory down that are already included",
			args: []string{"testing/alpha.txt", "testing/a"},
			want: []string{
				"testing/alpha.go",
			},
		},
		// aliasFetcher tests
		{
			name: "suggests only aliases for delete",
			args: []string{"d", ""},
			want: []string{
				"city",
				"salt",
			},
		},
		// GetAlias
		{
			name: "suggests aliases for get",
			args: []string{"g", ""},
			want: []string{
				"city",
				"salt",
			},
		},
		{
			name: "completes partial alias for get",
			args: []string{"g", "s"},
			want: []string{
				"salt",
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			commands.GenericAutocomplete()
			suggestions := commands.Autocomplete(e.Node(), test.args, -1)
			if diff := cmp.Diff(test.want, suggestions); diff != "" {
				t.Errorf("Complete(%v) produced diff (-want, +got):\n%s", test.args, diff)
			}
		})
	}
}*/

func TestEmacsExecution(t *testing.T) {
	for _, test := range []struct {
		name            string
		e               *Emacs
		args            []string
		limitOverride   int
		want            *Emacs
		wantOK          bool
		wantResp        *commands.WorldState
		wantStdout      []string
		wantStderr      []string
		osStatInfo      os.FileInfo
		osStatErr       error
		absolutePath    map[string]string
		absolutePathErr map[string]error
	}{
		// OpenEditor tests
		{
			name: "error when too many arguments",
			e:    &Emacs{},
			args: []string{"file1", "file2", "file3", "file4", "file5"},
			wantResp: &commands.WorldState{
				RawArgs: []string{"file5"},
				Values: map[string]*commands.Value{
					emacsArg: commands.StringListValue("file1", "file2", "file3", "file4"),
				},
			},
			wantStderr: []string{"extra unknown args ([file5])"},
		},
		{
			name: "fails if can't get absolute path",
			e:    &Emacs{},
			args: []string{"file1"},
			absolutePathErr: map[string]error{
				"file1": fmt.Errorf("what is this nonsense?"),
			},
			wantStderr: []string{
				`failed to get absolute path for file "file1": what is this nonsense?`,
			},
		},
		{
			name:       "cds into directory",
			e:          &Emacs{},
			args:       []string{"dirName"},
			osStatInfo: &fakeFileInfo{mode: os.ModeDir},
			wantResp: &commands.WorldState{
				Executable: [][]string{{
					"cd",
					"dirName",
				}},
			},
			wantOK: true,
		},
		{
			name:      "fails if file does not exist and new flag not provided",
			e:         &Emacs{},
			args:      []string{"newFile.txt"},
			osStatErr: os.ErrNotExist,
			wantStderr: []string{
				`file "newFile.txt" does not exist; include "new" flag to create it`,
			},
		},
		{
			name:      "creates new file if short new flag is provided",
			e:         &Emacs{},
			args:      []string{"newFile.txt", "-n"},
			osStatErr: os.ErrNotExist,
			wantOK:    true,
			wantResp: &commands.WorldState{
				Executable: [][]string{{
					"emacs",
					"--no-window-system",
					"newFile.txt",
				}},
			},
			want: &Emacs{
				PreviousExecutions: [][]string{{
					"emacs",
					"--no-window-system",
					"newFile.txt",
				}},
			},
		},
		{
			name:      "creates new file if new flag is provided",
			e:         &Emacs{},
			args:      []string{"newFile.txt", "--new"},
			osStatErr: os.ErrNotExist,
			wantOK:    true,
			wantResp: &commands.WorldState{
				Executable: [][]string{{
					"emacs",
					"--no-window-system",
					"newFile.txt",
				}},
			},
			want: &Emacs{
				PreviousExecutions: [][]string{{
					"emacs",
					"--no-window-system",
					"newFile.txt",
				}},
			},
		},
		{
			name: "handles files and alises",
			e: &Emacs{
				Aliases: map[string]map[string]*commands.Value{fileAliaserName: map[string]*commands.Value{
					"salt": commands.StringValue("compounds/sodiumChloride"),
					"city": commands.StringValue("catan/oreAndWheat"),
				},
				},
			},
			args: []string{"first.txt", "salt", "city", "fourth.go"},
			absolutePath: map[string]string{
				"first.txt": filepath.Join("current/dir", "first.txt"),
				"fourth.go": filepath.Join("current/dir", "fourth.go"),
			},
			wantOK: true,
			want: &Emacs{
				Aliases: map[string]map[string]*commands.Value{fileAliaserName: map[string]*commands.Value{
					"salt": commands.StringValue("compounds/sodiumChloride"),
					"city": commands.StringValue("catan/oreAndWheat"),
				},
				},
				PreviousExecutions: [][]string{{
					"emacs",
					"--no-window-system",
					filepath.Join("current/dir", "fourth.go"),
					"catan/oreAndWheat",
					"compounds/sodiumChloride",
					filepath.Join("current/dir", "first.txt"),
				}},
			},
			wantResp: &commands.WorldState{
				Executable: [][]string{{
					"emacs",
					"--no-window-system",
					filepath.Join("current/dir", "fourth.go"),
					"catan/oreAndWheat",
					"compounds/sodiumChloride",
					filepath.Join("current/dir", "first.txt"),
				}},
			},
		},
		{
			name: "handles line numbers",
			e: &Emacs{
				Aliases: map[string]map[string]*commands.Value{fileAliaserName: map[string]*commands.Value{
					"salt": commands.StringValue("compounds/sodiumChloride"),
					"city": commands.StringValue("catan/oreAndWheat"),
				},
				},
			},
			args: []string{"first.txt", "salt", "32", "fourth.go"},
			absolutePath: map[string]string{
				"first.txt": filepath.Join("home", "first.txt"),
				"fourth.go": filepath.Join("home", "fourth.go"),
			},
			wantOK: true,
			want: &Emacs{
				Aliases: map[string]map[string]*commands.Value{fileAliaserName: map[string]*commands.Value{
					"salt": commands.StringValue("compounds/sodiumChloride"),
					"city": commands.StringValue("catan/oreAndWheat"),
				},
				},
				PreviousExecutions: [][]string{{
					"emacs",
					"--no-window-system",
					filepath.Join("home", "fourth.go"),
					"+32",
					"compounds/sodiumChloride",
					filepath.Join("home", "first.txt"),
				}},
			},
			wantResp: &commands.WorldState{
				Executable: [][]string{{
					"emacs",
					"--no-window-system",
					filepath.Join("home", "fourth.go"),
					"+32",
					"compounds/sodiumChloride",
					filepath.Join("home", "first.txt"),
				}},
			},
		},
		{
			name: "handles multiple numbers",
			e: &Emacs{
				Aliases: map[string]map[string]*commands.Value{fileAliaserName: map[string]*commands.Value{
					"salt": commands.StringValue("compounds/sodiumChloride"),
					"city": commands.StringValue("catan/oreAndWheat"),
				},
				},
			},
			args: []string{"salt", "32", "14"},
			absolutePath: map[string]string{
				"14": filepath.Join("here", "14"),
			},
			wantOK: true,
			want: &Emacs{
				Aliases: map[string]map[string]*commands.Value{fileAliaserName: map[string]*commands.Value{
					"salt": commands.StringValue("compounds/sodiumChloride"),
					"city": commands.StringValue("catan/oreAndWheat"),
				},
				},
				PreviousExecutions: [][]string{{
					"emacs",
					"--no-window-system",
					filepath.Join("here", "14"),
					"+32",
					"compounds/sodiumChloride",
				}},
			},
			wantResp: &commands.WorldState{
				Executable: [][]string{{
					"emacs",
					"--no-window-system",
					filepath.Join("here", "14"),
					"+32",
					"compounds/sodiumChloride",
				}},
			},
		},
		{
			name: "adds to previous executions",
			e: &Emacs{
				Aliases: map[string]map[string]*commands.Value{fileAliaserName: map[string]*commands.Value{
					"city": commands.StringValue("catan/oreAndWheat"),
				},
				},
				PreviousExecutions: [][]string{
					{
						"emacs",
						"--no-window-system",
						"firstFile",
					},
					{
						"emacs",
						"--no-window-system",
						"2ndFile",
					},
				},
			},
			args:   []string{"luckyNumberThree"},
			wantOK: true,
			want: &Emacs{
				Aliases: map[string]map[string]*commands.Value{fileAliaserName: map[string]*commands.Value{
					"city": commands.StringValue("catan/oreAndWheat"),
				},
				},
				PreviousExecutions: [][]string{
					{
						"emacs",
						"--no-window-system",
						"firstFile",
					},
					{
						"emacs",
						"--no-window-system",
						"2ndFile",
					},
					{
						"emacs",
						"--no-window-system",
						"luckyNumberThree",
					},
				},
			},
			wantResp: &commands.WorldState{
				Executable: [][]string{{
					"emacs",
					"--no-window-system",
					"luckyNumberThree",
				}},
			},
		},
		{
			name:          "reduces size of previous executions if at limit",
			limitOverride: 2,
			e: &Emacs{
				Aliases: map[string]map[string]*commands.Value{fileAliaserName: map[string]*commands.Value{
					"city": commands.StringValue("catan/oreAndWheat"),
				},
				},
				PreviousExecutions: [][]string{
					{
						"emacs",
						"--no-window-system",
						"firstFile",
					},
					{
						"emacs",
						"--no-window-system",
						"2ndFile",
					},
				},
			},
			args:   []string{"luckyNumberThree"},
			wantOK: true,
			want: &Emacs{
				Aliases: map[string]map[string]*commands.Value{fileAliaserName: map[string]*commands.Value{
					"city": commands.StringValue("catan/oreAndWheat"),
				}},
				PreviousExecutions: [][]string{
					{
						"emacs",
						"--no-window-system",
						"2ndFile",
					},
					{
						"emacs",
						"--no-window-system",
						"luckyNumberThree",
					},
				},
			},
			wantResp: &commands.WorldState{
				Executable: [][]string{{
					"emacs",
					"--no-window-system",
					"luckyNumberThree",
				}},
			},
		},
		{
			name:          "reduces size of previous executions if over limit",
			limitOverride: 2,
			e: &Emacs{
				Aliases: map[string]map[string]*commands.Value{fileAliaserName: map[string]*commands.Value{
					"city": commands.StringValue("catan/oreAndWheat"),
				}},
				PreviousExecutions: [][]string{
					{
						"emacs",
						"--no-window-system",
						"firstFile",
					},
					{
						"emacs",
						"--no-window-system",
						"2ndFile",
					},
					{
						"emacs",
						"--no-window-system",
						"3rdFile",
					},
					{
						"emacs",
						"--no-window-system",
						"FourthFile",
					},
				},
			},
			args:   []string{"luckyNumberFive"},
			wantOK: true,
			want: &Emacs{
				Aliases: map[string]map[string]*commands.Value{fileAliaserName: map[string]*commands.Value{
					"city": commands.StringValue("catan/oreAndWheat"),
				}},
				PreviousExecutions: [][]string{
					{
						"emacs",
						"--no-window-system",
						"FourthFile",
					},
					{
						"emacs",
						"--no-window-system",
						"luckyNumberFive",
					},
				},
			},
			wantResp: &commands.WorldState{
				Executable: [][]string{{
					"emacs",
					"--no-window-system",
					"luckyNumberFive",
				}},
			},
		},
		{
			name: "if empty PreviousExecutions and no arguments, error",
			e:    &Emacs{},
			wantStderr: []string{
				"no previous executions",
			},
		},
		{
			name: "if nil argument, run last command",
			e: &Emacs{
				Aliases: map[string]map[string]*commands.Value{fileAliaserName: map[string]*commands.Value{
					"city": commands.StringValue("catan/oreAndWheat"),
				}},
				PreviousExecutions: [][]string{
					{
						"emacs",
						"--no-window-system",
						"firstFile",
					},
					{
						"emacs",
						"--no-window-system",
						"2ndFile",
					},
					{
						"emacs",
						"--no-window-system",
						"3rdFile",
					},
				},
			},
			wantOK: true,
			wantResp: &commands.WorldState{
				Executable: [][]string{{
					"emacs",
					"--no-window-system",
					"3rdFile",
				}},
			},
		},
		{
			name: "if empty arguments, run last command",
			args: []string{},
			e: &Emacs{
				Aliases: map[string]map[string]*commands.Value{fileAliaserName: map[string]*commands.Value{
					"city": commands.StringValue("catan/oreAndWheat"),
				}},
				PreviousExecutions: [][]string{
					{
						"emacs",
						"--no-window-system",
						"firstFile",
					},
					{
						"emacs",
						"--no-window-system",
						"2ndFile",
					},
					{
						"emacs",
						"--no-window-system",
						"3rdFile",
					},
				},
			},
			wantOK: true,
			wantResp: &commands.WorldState{
				Executable: [][]string{{
					"emacs",
					"--no-window-system",
					"3rdFile",
				}},
			},
		},
		// AddAlias tests
		{
			name:       "fails if no alias",
			args:       []string{"a"},
			wantStderr: []string{`no argument provided for "ALIAS"`},
		},
		{
			name:       "fails if no filename",
			args:       []string{"a", "bond"},
			wantStderr: []string{`no argument provided for "FILE"`},
		},
		{
			name:       "fails if too many arguments",
			args:       []string{"a", "salt", "Na", "Cl"},
			wantStderr: []string{"extra unknown args ([Cl])"},
		},
		{
			name: "fails if alias already defined",
			e: &Emacs{
				Aliases: map[string]map[string]*commands.Value{fileAliaserName: map[string]*commands.Value{
					"salt": commands.StringValue("NaCl"),
				}},
			},
			args:       []string{"a", "salt", "sodiumChloride"},
			wantStderr: []string{"alias already defined: (salt: NaCl)"},
		},
		{
			name:       "fails if osStat is not exist error",
			e:          &Emacs{},
			args:       []string{"a", "salt", "sodiumChloride"},
			osStatErr:  fmt.Errorf("nope"),
			wantStderr: []string{"file does not exist: nope"},
		},
		{
			name:       "fails if can't get absolute path",
			e:          &Emacs{},
			args:       []string{"a", "salt", "sodiumChloride"},
			osStatInfo: &fakeFileInfo{mode: 0},
			absolutePathErr: map[string]error{
				"sodiumChloride": fmt.Errorf("absolute mistake"),
			},
			wantStderr: []string{`failed to get absolute file path for file "sodiumChloride": absolute mistake`},
		},
		{
			name:       "adds to nil aliases",
			e:          &Emacs{},
			args:       []string{"a", "salt", "sodiumChloride"},
			osStatInfo: &fakeFileInfo{mode: 0},
			absolutePath: map[string]string{
				"sodiumChloride": "compounds/sodiumChloride",
			},
			wantOK: true,
			want: &Emacs{
				Aliases: map[string]map[string]*commands.Value{fileAliaserName: map[string]*commands.Value{
					"salt": commands.StringValue("compounds/sodiumChloride"),
				}},
			},
		},
		{
			name: "adds to empty aliases",
			e: &Emacs{
				Aliases: map[string]map[string]*commands.Value{fileAliaserName: map[string]*commands.Value{}},
			},
			args:       []string{"a", "salt", "sodiumChloride"},
			osStatInfo: &fakeFileInfo{mode: 0},
			absolutePath: map[string]string{
				"sodiumChloride": "compounds/sodiumChloride",
			},
			wantOK: true,
			want: &Emacs{
				Aliases: map[string]map[string]*commands.Value{fileAliaserName: map[string]*commands.Value{
					"salt": commands.StringValue("compounds/sodiumChloride"),
				}},
			},
		},
		{
			name: "adds to aliases",
			e: &Emacs{
				Aliases: map[string]map[string]*commands.Value{fileAliaserName: map[string]*commands.Value{
					"other": commands.StringValue("things"),
					"ab":    commands.StringValue("cd"),
				}},
			},
			args:       []string{"a", "salt", "sodiumChloride"},
			osStatInfo: &fakeFileInfo{mode: 0},
			absolutePath: map[string]string{
				"sodiumChloride": "compounds/sodiumChloride",
			},
			wantOK: true,
			want: &Emacs{
				Aliases: map[string]map[string]*commands.Value{fileAliaserName: map[string]*commands.Value{
					"other": commands.StringValue("things"),
					"ab":    commands.StringValue("cd"),
					"salt":  commands.StringValue("compounds/sodiumChloride"),
				}},
			},
		},
		{
			name:       "adds alias for directory",
			e:          &Emacs{},
			args:       []string{"a", "salt", "sodiumChloride"},
			osStatInfo: &fakeFileInfo{mode: os.ModeDir},
			absolutePath: map[string]string{
				"sodiumChloride": "compounds/sodiumChloride",
			},
			wantOK: true,
			want: &Emacs{
				Aliases: map[string]map[string]*commands.Value{fileAliaserName: map[string]*commands.Value{
					"salt": commands.StringValue("compounds/sodiumChloride"),
				}},
			},
		},
		// DeleteAliases tests
		{
			name:       "error if no arguments",
			e:          &Emacs{},
			args:       []string{"d"},
			wantStderr: []string{`no argument provided for "ALIAS"`},
		},
		{
			name:       "ignores unknown alias",
			e:          &Emacs{},
			args:       []string{"d", "salt"},
			wantOK:     true,
			wantStderr: []string{`alias "salt" does not exist`},
		},
		{
			name: "deletes existing alias",
			e: &Emacs{
				Aliases: map[string]map[string]*commands.Value{fileAliaserName: map[string]*commands.Value{
					"salt": commands.StringValue("compounds/sodiumChloride"),
				}},
			},
			args: []string{"d", "salt"},
			want: &Emacs{
				Aliases: map[string]map[string]*commands.Value{fileAliaserName: map[string]*commands.Value{}},
			},
			wantOK: true,
		},
		{
			name: "handles multiple missing and present",
			e: &Emacs{
				Aliases: map[string]map[string]*commands.Value{fileAliaserName: map[string]*commands.Value{
					"salt": commands.StringValue("compounds/sodiumChloride"),
					"city": commands.StringValue("catan/oreAndWheat"),
					"4":    commands.StringValue("2+2"),
				}},
			},
			args:   []string{"d", "salt", "settlement", "5", "4"},
			wantOK: true,
			want: &Emacs{
				Aliases: map[string]map[string]*commands.Value{fileAliaserName: map[string]*commands.Value{
					"city": commands.StringValue("catan/oreAndWheat"),
				}},
			},
			wantStderr: []string{
				`alias "settlement" does not exist`,
				`alias "5" does not exist`,
			},
		},
		// ListAliases tests
		{
			name:       "error when too many arguments",
			args:       []string{"l", "extra"},
			wantStderr: []string{"extra unknown args ([extra])"},
		},
		{
			name:   "no output for nil aliases",
			args:   []string{"l"},
			e:      &Emacs{},
			wantOK: true,
		},
		{
			name: "no output for empty aliases",
			args: []string{"l"},
			e: &Emacs{
				Aliases: map[string]map[string]*commands.Value{fileAliaserName: map[string]*commands.Value{}},
			},
			wantOK: true,
		},
		{
			name: "proper output for aliases",
			args: []string{"l"},
			e: &Emacs{
				Aliases: map[string]map[string]*commands.Value{fileAliaserName: map[string]*commands.Value{
					"salt": commands.StringValue("compounds/sodiumChloride"),
					"city": commands.StringValue("catan/oreAndWheat"),
					"4":    commands.StringValue("2+2"),
				}},
			},
			wantOK: true,
			wantStdout: []string{
				"4: 2+2",
				"city: catan/oreAndWheat",
				"salt: compounds/sodiumChloride",
			},
		},
		// GetAlias
		{
			name: "GetAlias requires alias",
			args: []string{"g"},
			e:    &Emacs{},
			wantStderr: []string{
				fmt.Sprintf("no argument provided for %q", aliasArg),
			},
		},
		{
			name: "GetAlias fails if alias does not exist",
			args: []string{"g", "salt"},
			e:    &Emacs{},
			wantStderr: []string{
				`Alias "salt" does not exist`,
			},
		},
		{
			name: "GetAlias works",
			args: []string{"g", "salt"},
			e: &Emacs{
				Aliases: map[string]map[string]*commands.Value{fileAliaserName: map[string]*commands.Value{
					"salt": commands.StringValue("compounds/sodiumChloride"),
					"city": commands.StringValue("catan/oreAndWheat"),
					"4":    commands.StringValue("2+2"),
				}},
			},
			wantOK: true,
			wantStdout: []string{
				"salt: compounds/sodiumChloride",
			},
		},
		// SearchAliases
		{
			name: "SearchAlias requires regexp",
			args: []string{"s"},
			e:    &Emacs{},
			wantStderr: []string{
				fmt.Sprintf("no argument provided for %q", regexpArg),
			},
		},
		{
			name: "SearchAlias requires valid regexp",
			args: []string{"s", "[a-9]"},
			e:    &Emacs{},
			wantStderr: []string{
				"Invalid regexp: error parsing regexp: invalid character class range: `a-9`",
			},
		},
		{
			name: "SearchAlias works",
			args: []string{"s", "compounds"},
			e: &Emacs{
				Aliases: map[string]map[string]*commands.Value{fileAliaserName: map[string]*commands.Value{
					"water": commands.StringValue("liquids/compounds/hydrogenDioxide"),
					"salt":  commands.StringValue("compounds/sodiumChloride"),
					"city":  commands.StringValue("catan/oreAndWheat"),
					"4":     commands.StringValue("2+2"),
				},
				},
			},
			wantOK: true,
			wantStdout: []string{
				"salt: compounds/sodiumChloride",
				"water: liquids/compounds/hydrogenDioxide",
			},
		},
		// RunHistorical
		{
			name: "prints historical commands if no index given",
			args: []string{"h"},
			e: &Emacs{
				Aliases: map[string]map[string]*commands.Value{fileAliaserName: map[string]*commands.Value{
					"city": commands.StringValue("catan/oreAndWheat"),
				}},
				PreviousExecutions: [][]string{
					{
						"emacs",
						"--no-window-system",
						"firstFile",
					},
					{
						"emacs",
						"--no-window-system",
						"2nd", "File",
					},
					{
						"emacs",
						"--no-window-system",
						"File", "three",
					},
				},
			},
			wantOK: true,
			wantStdout: []string{
				" 2: emacs --no-window-system firstFile",
				" 1: emacs --no-window-system 2nd File",
				" 0: emacs --no-window-system File three",
			},
		},
		{
			name: "historical fails if negative idx",
			args: []string{"h", "-1"},
			e: &Emacs{
				Aliases: map[string]map[string]*commands.Value{fileAliaserName: map[string]*commands.Value{
					"city": commands.StringValue("catan/oreAndWheat"),
				}},
				PreviousExecutions: [][]string{
					{
						"emacs",
						"--no-window-system",
						"firstFile",
					},
					{
						"emacs",
						"--no-window-system",
						"2nd", "File",
					},
					{
						"emacs",
						"--no-window-system",
						"File", "three",
					},
				},
			},
			wantStderr: []string{
				// TODO: modify commands package to produce a better message here.
				"failed to process args: failed to convert value: validation failed: [IntNonNegative] value isn't non-negative",
			},
		},
		{
			name: "historical fails if index is too large",
			args: []string{"h", "3"},
			e: &Emacs{
				Aliases: map[string]map[string]*commands.Value{fileAliaserName: map[string]*commands.Value{
					"city": commands.StringValue("catan/oreAndWheat"),
				}},
				PreviousExecutions: [][]string{
					{
						"emacs",
						"--no-window-system",
						"firstFile",
					},
					{
						"emacs",
						"--no-window-system",
						"2nd", "File",
					},
					{
						"emacs",
						"--no-window-system",
						"File", "three",
					},
				},
			},
			wantStderr: []string{
				fmt.Sprintf("%s is larger than list of stored commands", historicalArg),
			},
		},
		{
			name: "historical returns 0 index",
			args: []string{"h", "0"},
			e: &Emacs{
				Aliases: map[string]map[string]*commands.Value{fileAliaserName: map[string]*commands.Value{
					"city": commands.StringValue("catan/oreAndWheat"),
				}},
				PreviousExecutions: [][]string{
					{
						"emacs",
						"--no-window-system",
						"firstFile",
					},
					{
						"emacs",
						"--no-window-system",
						"2nd", "File",
					},
					{
						"emacs",
						"--no-window-system",
						"File", "three",
					},
					{
						"emacs",
						"--no-window-system",
						"fourth",
					},
				},
			},
			wantOK: true,
			wantResp: &commands.WorldState{
				Executable: [][]string{{
					"emacs",
					"--no-window-system",
					"fourth",
				}},
			},
		},
		{
			name: "historical returns 0 index",
			args: []string{"h", "2"},
			e: &Emacs{
				Aliases: map[string]map[string]*commands.Value{fileAliaserName: map[string]*commands.Value{
					"city": commands.StringValue("catan/oreAndWheat"),
				}},
				PreviousExecutions: [][]string{
					{
						"emacs",
						"--no-window-system",
						"firstFile",
					},
					{
						"emacs",
						"--no-window-system",
						"2nd", "File",
					},
					{
						"emacs",
						"--no-window-system",
						"File", "three",
					},
					{
						"emacs",
						"--no-window-system",
						"fourth",
					},
				},
			},
			wantOK: true,
			wantResp: &commands.WorldState{
				Executable: [][]string{{
					"emacs",
					"--no-window-system",
					"2nd", "File",
				}},
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			oldStat := osStat
			osStat = func(string) (os.FileInfo, error) { return test.osStatInfo, test.osStatErr }
			defer func() { osStat = oldStat }()

			oldAbs := filepathAbs
			filepathAbs = func(s string) (string, error) {
				p, ok := test.absolutePath[s]
				if !ok {
					p = s
				}
				err, ok := test.absolutePathErr[s]
				if !ok {
					err = nil
				}
				return p, err
			}
			defer func() { filepathAbs = oldAbs }()

			/*oldFileAliaser := fileAliaser
			fileAliaser = func() commands.Aliaser { return commands.TestFileAliaser(osStat, filepathAbs) }
			defer func() { fileAliaser = oldFileAliaser }()*/

			if test.limitOverride != 0 {
				oldLimit := historyLimit
				historyLimit = test.limitOverride
				defer func() { historyLimit = oldLimit }()
			}

			ws := &commands.WorldState{
				RawArgs: test.args,
			}
			commandtest.Execute(t, test.e.Node(), ws, test.wantResp, test.wantStdout, test.wantStderr)
			/*got, ok := commands.Execute(tcos, test.e.Command(), test.args, nil)
			if ok != test.wantOK {
				t.Fatalf("commands.Execute(%v) returned %v for ok; want %v", test.args, ok, test.wantOK)
			}
			if diff := cmp.Diff(test.wantResp, got); diff != "" {
				t.Fatalf("Execute(%v) produced response diff (-want, +got):\n%s", test.args, diff)
			}

			if diff := cmp.Diff(test.wantStdout, tcos.GetStdout()); diff != "" {
				t.Errorf("command.Execute(%v) produced stdout diff (-want, +got):\n%s", test.args, diff)
			}
			if diff := cmp.Diff(test.wantStderr, tcos.GetStderr()); diff != "" {
				t.Errorf("command.Execute(%v) produced stderr diff (-want, +got):\n%s", test.args, diff)
			}*/

			// Assume wantChanged if test.want is set
			wantChanged := test.want != nil
			changed := test.e != nil && test.e.Changed()
			if changed != wantChanged {
				t.Fatalf("Execute(%v) marked Changed as %v; want %v", test.args, changed, wantChanged)
			}

			// Only check diff if we are expecting a change.
			if wantChanged {
				//b, _ := json.Marshal(test.want)
				//fmt.Println(string(b))
				opts := []cmp.Option{
					cmpopts.IgnoreUnexported(Emacs{}), // commands.Value{}),
					// TODO: remove this once set is moved into a separate proto message.
					//cmpopts.IgnoreFields(commands.Value{}, "Set"),
					protocmp.Transform(),
				}
				if diff := cmp.Diff(test.want, test.e, opts...); diff != "" {
					t.Fatalf("Execute(%v) produced emacs diff (-want, +got):\n%s", test.args, diff)
				}
			}
		})
	}
}

type fakeFileInfo struct{ mode os.FileMode }

func (fi fakeFileInfo) Name() string       { return "" }
func (fi fakeFileInfo) Size() int64        { return 0 }
func (fi fakeFileInfo) Mode() os.FileMode  { return fi.mode }
func (fi fakeFileInfo) ModTime() time.Time { return time.Now() }
func (fi fakeFileInfo) IsDir() bool        { return fi.Mode().IsDir() }
func (fi fakeFileInfo) Sys() interface{}   { return nil }

/*func TestUsage(t *testing.T) {
	e := &Emacs{}
	wantUsage := []string{
		"a", aliasArg, fileArg, "\n",
		"d", aliasArg, fmt.Sprintf("[%s ...]", aliasArg), "\n",
		"g", aliasArg, "\n",
		"h", "[", historicalArg, "]", "\n",
		"l", "\n",
		"s", regexpArg, "\n",
		"[", emacsArg, emacsArg, emacsArg, emacsArg, "]",
		"--new|-n",
	}
	usage := e.Command().Usage()
	if diff := cmp.Diff(wantUsage, usage); diff != "" {
		t.Errorf("Usage() produced diff (-want, +got):\n%s", diff)
	}
}*/

func TestMetadata(t *testing.T) {
	e := &Emacs{}
	want := "emacs-shortcuts"
	if e.Name() != want {
		t.Errorf("Incorrect emacs name: got %s; want %s", e.Name(), want)
	}

	want = "e"
	if e.Alias() != want {
		t.Errorf("Incorrect emacs alias: got %s; want %s", e.Alias(), want)
	}

	if e.Option() != nil {
		t.Errorf("Emacs{}.Option() returned %v; want nil", e.Option())
	}
}
