package emacs

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/leep-frog/commands/commands"

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
			json: `{"Aliases": {"salt": "compounds/sodiumChloride"}}`,
			want: &Emacs{
				Aliases: map[string]string{
					"salt": "compounds/sodiumChloride",
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

func TestAutocomplete(t *testing.T) {
	e := &Emacs{
		Aliases: map[string]string{
			"salt": "compounds/sodiumChloride",
			"city": "catan/oreAndWheat",
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
	} {
		t.Run(test.name, func(t *testing.T) {
			suggestions := commands.Autocomplete(e.Command(), test.args, -1)
			if diff := cmp.Diff(test.want, suggestions); diff != "" {
				t.Errorf("Complete(%v) produced diff (-want, +got):\n%s", test.args, diff)
			}
		})
	}
}

func TestEmacsExecution(t *testing.T) {
	for _, test := range []struct {
		name            string
		e               *Emacs
		args            []string
		limitOverride   int
		want            *Emacs
		wantOK          bool
		wantResp        *commands.ExecutorResponse
		wantChanged     bool
		wantStdout      []string
		wantStderr      []string
		osStatInfo      os.FileInfo
		osStatErr       error
		absolutePath    string
		absolutePathErr error
		osGetwd         string
		osGetwdErr      error
	}{
		// OpenEditor tests
		{
			name:       "error when too many arguments",
			args:       []string{"file1", "file2", "file3", "file4", "file5"},
			wantStderr: []string{"extra unknown args ([file5])"},
		},
		{
			name:       "doesn't set previous execution on getwd error",
			e:          &Emacs{},
			args:       []string{"file1", "file2"},
			osGetwdErr: fmt.Errorf("uh oh"),
			wantStderr: []string{"failed to get current directory: uh oh"},
			wantOK:     true,
			wantResp: &commands.ExecutorResponse{
				Executable: []string{"emacs", "--no-window-system", "file2", "file1"},
			},
		},
		{
			name: "handles files and alises",
			e: &Emacs{
				Aliases: map[string]string{
					"salt": "compounds/sodiumChloride",
					"city": "catan/oreAndWheat",
				},
			},
			args:    []string{"first.txt", "salt", "city", "fourth.go"},
			osGetwd: "current/dir",
			wantOK:  true,
			want: &Emacs{
				Aliases: map[string]string{
					"salt": "compounds/sodiumChloride",
					"city": "catan/oreAndWheat",
				},
				PreviousExecutions: []*commands.ExecutorResponse{
					{
						Executable: []string{
							"emacs",
							"--no-window-system",
							filepath.Join("current/dir", "fourth.go"),
							"catan/oreAndWheat",
							"compounds/sodiumChloride",
							filepath.Join("current/dir", "first.txt"),
						},
					},
				},
			},
			wantResp: &commands.ExecutorResponse{
				Executable: []string{
					"emacs",
					"--no-window-system",
					"fourth.go",
					"catan/oreAndWheat",
					"compounds/sodiumChloride",
					"first.txt",
				},
			},
		},
		{
			name: "handles line numbers",
			e: &Emacs{
				Aliases: map[string]string{
					"salt": "compounds/sodiumChloride",
					"city": "catan/oreAndWheat",
				},
			},
			args:    []string{"first.txt", "salt", "32", "fourth.go"},
			osGetwd: "home",
			wantOK:  true,
			want: &Emacs{
				Aliases: map[string]string{
					"salt": "compounds/sodiumChloride",
					"city": "catan/oreAndWheat",
				},
				PreviousExecutions: []*commands.ExecutorResponse{
					{
						Executable: []string{
							"emacs",
							"--no-window-system",
							filepath.Join("home", "fourth.go"),
							"+32",
							"compounds/sodiumChloride",
							filepath.Join("home", "first.txt"),
						},
					},
				},
			},
			wantResp: &commands.ExecutorResponse{
				Executable: []string{
					"emacs",
					"--no-window-system",
					"fourth.go",
					"+32",
					"compounds/sodiumChloride",
					"first.txt",
				},
			},
		},
		{
			name: "handles multiple numbers",
			e: &Emacs{
				Aliases: map[string]string{
					"salt": "compounds/sodiumChloride",
					"city": "catan/oreAndWheat",
				},
			},
			args:    []string{"salt", "32", "14"},
			osGetwd: "here",
			wantOK:  true,
			want: &Emacs{
				Aliases: map[string]string{
					"salt": "compounds/sodiumChloride",
					"city": "catan/oreAndWheat",
				},
				PreviousExecutions: []*commands.ExecutorResponse{
					{
						Executable: []string{
							"emacs",
							"--no-window-system",
							filepath.Join("here", "14"),
							"+32",
							"compounds/sodiumChloride",
						},
					},
				},
			},
			wantResp: &commands.ExecutorResponse{
				Executable: []string{
					"emacs",
					"--no-window-system",
					"14",
					"+32",
					"compounds/sodiumChloride",
				},
			},
		},
		{
			name: "adds to previous executions",
			e: &Emacs{
				Aliases: map[string]string{
					"city": "catan/oreAndWheat",
				},
				PreviousExecutions: []*commands.ExecutorResponse{
					{
						Executable: []string{
							"emacs",
							"--no-window-system",
							"firstFile",
						},
					},
					{
						Executable: []string{
							"emacs",
							"--no-window-system",
							"2ndFile",
						},
					},
				},
			},
			args:   []string{"luckyNumberThree"},
			wantOK: true,
			want: &Emacs{
				Aliases: map[string]string{
					"city": "catan/oreAndWheat",
				},
				PreviousExecutions: []*commands.ExecutorResponse{
					{
						Executable: []string{
							"emacs",
							"--no-window-system",
							"firstFile",
						},
					},
					{
						Executable: []string{
							"emacs",
							"--no-window-system",
							"2ndFile",
						},
					},
					{
						Executable: []string{
							"emacs",
							"--no-window-system",
							"luckyNumberThree",
						},
					},
				},
			},
			wantResp: &commands.ExecutorResponse{
				Executable: []string{
					"emacs",
					"--no-window-system",
					"luckyNumberThree",
				},
			},
		},
		{
			name:          "reduces size of previous executions if at limit",
			limitOverride: 2,
			e: &Emacs{
				Aliases: map[string]string{
					"city": "catan/oreAndWheat",
				},
				PreviousExecutions: []*commands.ExecutorResponse{
					{
						Executable: []string{
							"emacs",
							"--no-window-system",
							"firstFile",
						},
					},
					{
						Executable: []string{
							"emacs",
							"--no-window-system",
							"2ndFile",
						},
					},
				},
			},
			args:   []string{"luckyNumberThree"},
			wantOK: true,
			want: &Emacs{
				Aliases: map[string]string{
					"city": "catan/oreAndWheat",
				},
				PreviousExecutions: []*commands.ExecutorResponse{
					{
						Executable: []string{
							"emacs",
							"--no-window-system",
							"2ndFile",
						},
					},
					{
						Executable: []string{
							"emacs",
							"--no-window-system",
							"luckyNumberThree",
						},
					},
				},
			},
			wantResp: &commands.ExecutorResponse{
				Executable: []string{
					"emacs",
					"--no-window-system",
					"luckyNumberThree",
				},
			},
		},
		{
			name:          "reduces size of previous executions if over limit",
			limitOverride: 2,
			e: &Emacs{
				Aliases: map[string]string{
					"city": "catan/oreAndWheat",
				},
				PreviousExecutions: []*commands.ExecutorResponse{
					{
						Executable: []string{
							"emacs",
							"--no-window-system",
							"firstFile",
						},
					},
					{
						Executable: []string{
							"emacs",
							"--no-window-system",
							"2ndFile",
						},
					},
					{
						Executable: []string{
							"emacs",
							"--no-window-system",
							"3rdFile",
						},
					},
					{
						Executable: []string{
							"emacs",
							"--no-window-system",
							"FourthFile",
						},
					},
				},
			},
			args:   []string{"luckyNumberFive"},
			wantOK: true,
			want: &Emacs{
				Aliases: map[string]string{
					"city": "catan/oreAndWheat",
				},
				PreviousExecutions: []*commands.ExecutorResponse{
					{
						Executable: []string{
							"emacs",
							"--no-window-system",
							"FourthFile",
						},
					},
					{
						Executable: []string{
							"emacs",
							"--no-window-system",
							"luckyNumberFive",
						},
					},
				},
			},
			wantResp: &commands.ExecutorResponse{
				Executable: []string{
					"emacs",
					"--no-window-system",
					"luckyNumberFive",
				},
			},
		},
		{
			name: "if nil emacs and no arguments, error",
			wantStderr: []string{
				"no previous executions",
			},
		},
		{
			name: "if empty PreviousExecutions and no arguments, error",
			e: &Emacs{
				PreviousExecutions: []*commands.ExecutorResponse{},
			},
			wantStderr: []string{
				"no previous executions",
			},
		},
		{
			name: "if nil argument, run last command",
			e: &Emacs{
				Aliases: map[string]string{
					"city": "catan/oreAndWheat",
				},
				PreviousExecutions: []*commands.ExecutorResponse{
					{
						Executable: []string{
							"emacs",
							"--no-window-system",
							"firstFile",
						},
					},
					{
						Executable: []string{
							"emacs",
							"--no-window-system",
							"2ndFile",
						},
					},
					{
						Executable: []string{
							"emacs",
							"--no-window-system",
							"3rdFile",
						},
					},
				},
			},
			wantOK: true,
			wantResp: &commands.ExecutorResponse{
				Executable: []string{
					"emacs",
					"--no-window-system",
					"3rdFile",
				},
			},
		},
		{
			name: "if empty arguments, run last command",
			args: []string{},
			e: &Emacs{
				Aliases: map[string]string{
					"city": "catan/oreAndWheat",
				},
				PreviousExecutions: []*commands.ExecutorResponse{
					{
						Executable: []string{
							"emacs",
							"--no-window-system",
							"firstFile",
						},
					},
					{
						Executable: []string{
							"emacs",
							"--no-window-system",
							"2ndFile",
						},
					},
					{
						Executable: []string{
							"emacs",
							"--no-window-system",
							"3rdFile",
						},
					},
				},
			},
			wantOK: true,
			wantResp: &commands.ExecutorResponse{
				Executable: []string{
					"emacs",
					"--no-window-system",
					"3rdFile",
				},
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
				Aliases: map[string]string{
					"salt": "NaCl",
				},
			},
			args:       []string{"a", "salt", "sodiumChloride"},
			wantStderr: []string{"alias already defined: (salt: NaCl)"},
		},
		{
			name:       "fails if osStat error",
			e:          &Emacs{},
			args:       []string{"a", "salt", "sodiumChloride"},
			osStatErr:  fmt.Errorf("broken"),
			wantStderr: []string{"error with file: broken"},
		},
		{
			name:       "fails if directory",
			e:          &Emacs{},
			args:       []string{"a", "salt", "sodiumChloride"},
			osStatInfo: &fakeFileInfo{mode: os.ModeDir},
			wantStderr: []string{"sodiumChloride is a directory"},
		},
		{
			name:            "fails if can't get absolute path",
			e:               &Emacs{},
			args:            []string{"a", "salt", "sodiumChloride"},
			osStatInfo:      &fakeFileInfo{mode: 0},
			absolutePathErr: fmt.Errorf("absolute mistake"),
			wantStderr:      []string{"failed to get absolute file path: absolute mistake"},
		},
		{
			name:         "adds to nil aliases",
			e:            &Emacs{},
			args:         []string{"a", "salt", "sodiumChloride"},
			osStatInfo:   &fakeFileInfo{mode: 0},
			absolutePath: "compounds/sodiumChloride",
			wantOK:       true,
			want: &Emacs{
				Aliases: map[string]string{
					"salt": "compounds/sodiumChloride",
				},
			},
		},
		{
			name: "adds to empty aliases",
			e: &Emacs{
				Aliases: map[string]string{},
			},
			args:         []string{"a", "salt", "sodiumChloride"},
			osStatInfo:   &fakeFileInfo{mode: 0},
			absolutePath: "compounds/sodiumChloride",
			wantOK:       true,
			want: &Emacs{
				Aliases: map[string]string{
					"salt": "compounds/sodiumChloride",
				},
			},
		},
		{
			name: "adds to aliases",
			e: &Emacs{
				Aliases: map[string]string{
					"other": "things",
					"ab":    "cd",
				},
			},
			args:         []string{"a", "salt", "sodiumChloride"},
			osStatInfo:   &fakeFileInfo{mode: 0},
			absolutePath: "compounds/sodiumChloride",
			wantOK:       true,
			want: &Emacs{
				Aliases: map[string]string{
					"other": "things",
					"ab":    "cd",
					"salt":  "compounds/sodiumChloride",
				},
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
				Aliases: map[string]string{
					"salt": "compounds/sodiumChloride",
				},
			},
			args: []string{"d", "salt"},
			want: &Emacs{
				Aliases: map[string]string{},
			},
			wantOK: true,
		},
		{
			name: "handles multiple missing and present",
			e: &Emacs{
				Aliases: map[string]string{
					"salt": "compounds/sodiumChloride",
					"city": "catan/oreAndWheat",
					"4":    "2+2",
				},
			},
			args:   []string{"d", "salt", "settlement", "5", "4"},
			wantOK: true,
			want: &Emacs{
				Aliases: map[string]string{
					"city": "catan/oreAndWheat",
				},
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
				Aliases: map[string]string{},
			},
			wantOK: true,
		},
		{
			name: "proper output for aliases",
			args: []string{"l"},
			e: &Emacs{
				Aliases: map[string]string{
					"salt": "compounds/sodiumChloride",
					"city": "catan/oreAndWheat",
					"4":    "2+2",
				},
			},
			wantOK: true,
			wantStdout: []string{
				"4: 2+2",
				"city: catan/oreAndWheat",
				"salt: compounds/sodiumChloride",
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			oldStat := osStat
			osStat = func(string) (os.FileInfo, error) { return test.osStatInfo, test.osStatErr }
			defer func() { osStat = oldStat }()

			oldAbs := filepathAbs
			filepathAbs = func(string) (string, error) { return test.absolutePath, test.absolutePathErr }
			defer func() { filepathAbs = oldAbs }()

			oldGetwd := osGetwd
			osGetwd = func() (string, error) { return test.osGetwd, test.osGetwdErr }
			defer func() { osGetwd = oldGetwd }()

			if test.limitOverride != 0 {
				oldLimit := historyLimit
				historyLimit = test.limitOverride
				defer func() { historyLimit = oldLimit }()
			}

			tcos := &commands.TestCommandOS{}
			got, ok := commands.Execute(tcos, test.e.Command(), test.args, nil)
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
			}

			// Assume wantChanged if test.want is set
			wantChanged := test.want != nil
			changed := test.e != nil && test.e.Changed()
			if changed != wantChanged {
				t.Fatalf("Execute(%v) marked Changed as %v; want %v", test.args, changed, test.wantChanged)
			}

			// Only check diff if we are expecting a change.
			if wantChanged {
				if diff := cmp.Diff(test.want, test.e, cmpopts.IgnoreUnexported(Emacs{})); diff != "" {
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

func TestUsage(t *testing.T) {
	e := &Emacs{}
	wantUsage := []string{
		"a", "ALIAS", "FILE", "\n",
		"d", "ALIAS", "[ALIAS ...]", "\n",
		"l", "\n",
		"[", "EMACS_ARG", "EMACS_ARG", "EMACS_ARG", "EMACS_ARG", "]",
	}
	usage := e.Command().Usage()
	if diff := cmp.Diff(wantUsage, usage); diff != "" {
		t.Errorf("Usage() produced diff (-want, +got):\n%s", diff)
	}
}

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
}
