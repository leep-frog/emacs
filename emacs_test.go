package emacs

import (
	"fmt"
	"os"
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
				" ",
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
				// TODO: shouldn't have this
				" ",
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
	}{
		// OpenEditor tests
		{
			name:       "handles nil args",
			wantStderr: []string{`no argument provided for "EMACS_ARG"`},
		},
		{
			name:       "handles empty args",
			args:       []string{},
			wantStderr: []string{`no argument provided for "EMACS_ARG"`},
		},
		{
			name:       "error when too many arguments",
			args:       []string{"file1", "file2", "file3", "file4", "file5"},
			wantStderr: []string{"extra unknown args ([file5])"},
		},
		{
			name: "handles files and alises",
			e: &Emacs{
				Aliases: map[string]string{
					"salt": "compounds/sodiumChloride",
					"city": "catan/oreAndWheat",
				},
			},
			args:   []string{"first.txt", "salt", "city", "fourth.go"},
			wantOK: true,
			want: &Emacs{
				Aliases: map[string]string{
					"salt": "compounds/sodiumChloride",
					"city": "catan/oreAndWheat",
				},
			},
			wantResp: &commands.ExecutorResponse{
				Executable: []string{
					"emacs",
					"--no-window-system",
					"first.txt",
					"compounds/sodiumChloride",
					"catan/oreAndWheat",
					"fourth.go",
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
			args:   []string{"first.txt", "salt", "32", "fourth.go"},
			wantOK: true,
			want: &Emacs{
				Aliases: map[string]string{
					"salt": "compounds/sodiumChloride",
					"city": "catan/oreAndWheat",
				},
			},
			wantResp: &commands.ExecutorResponse{
				Executable: []string{
					"emacs",
					"--no-window-system",
					"first.txt",
					"+32",
					"compounds/sodiumChloride",
					"fourth.go",
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
			args:   []string{"salt", "32", "14"},
			wantOK: true,
			want: &Emacs{
				Aliases: map[string]string{
					"salt": "compounds/sodiumChloride",
					"city": "catan/oreAndWheat",
				},
			},
			wantResp: &commands.ExecutorResponse{
				Executable: []string{
					"emacs",
					"--no-window-system",
					"+32",
					"compounds/sodiumChloride",
					"14",
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
			want: &Emacs{
				Aliases: map[string]string{
					"salt": "NaCl",
				},
			},
		},
		{
			name:       "fails if osStat error",
			e:          &Emacs{},
			args:       []string{"a", "salt", "sodiumChloride"},
			osStatErr:  fmt.Errorf("broken"),
			wantStderr: []string{"error with file: broken"},
			want:       &Emacs{},
		},
		{
			name:       "fails if directory",
			e:          &Emacs{},
			args:       []string{"a", "salt", "sodiumChloride"},
			osStatInfo: &fakeFileInfo{mode: os.ModeDir},
			wantStderr: []string{"sodiumChloride is a directory"},
			want:       &Emacs{},
		},
		{
			name:            "fails if can't get absolute path",
			e:               &Emacs{},
			args:            []string{"a", "salt", "sodiumChloride"},
			osStatInfo:      &fakeFileInfo{mode: 0},
			absolutePathErr: fmt.Errorf("absolute mistake"),
			want:            &Emacs{},
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
			wantResp:    &commands.ExecutorResponse{},
			wantChanged: true,
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
			wantResp:    &commands.ExecutorResponse{},
			wantChanged: true,
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
			wantResp:    &commands.ExecutorResponse{},
			wantChanged: true,
		},
		// DeleteAliases tests
		{
			name:       "error if no arguments",
			e:          &Emacs{},
			args:       []string{"d"},
			want:       &Emacs{},
			wantStderr: []string{`no argument provided for "ALIAS"`},
		},
		{
			name:       "ignores unknown alias",
			e:          &Emacs{},
			args:       []string{"d", "salt"},
			want:       &Emacs{},
			wantOK:     true,
			wantResp:   &commands.ExecutorResponse{},
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
			wantOK:      true,
			wantResp:    &commands.ExecutorResponse{},
			wantChanged: true,
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
			wantResp: &commands.ExecutorResponse{},
			wantStderr: []string{
				`alias "settlement" does not exist`,
				`alias "5" does not exist`,
			},
			wantChanged: true,
		},
		// ListAliases tests
		{
			name:       "error when too many arguments",
			args:       []string{"l", "extra"},
			wantStderr: []string{"extra unknown args ([extra])"},
		},
		{
			name:     "no output for nil aliases",
			args:     []string{"l"},
			e:        &Emacs{},
			want:     &Emacs{},
			wantOK:   true,
			wantResp: &commands.ExecutorResponse{},
		},
		{
			name: "no output for empty aliases",
			args: []string{"l"},
			e: &Emacs{
				Aliases: map[string]string{},
			},
			want: &Emacs{
				Aliases: map[string]string{},
			},
			wantOK:   true,
			wantResp: &commands.ExecutorResponse{},
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
			want: &Emacs{
				Aliases: map[string]string{
					"salt": "compounds/sodiumChloride",
					"city": "catan/oreAndWheat",
					"4":    "2+2",
				},
			},
			wantResp: &commands.ExecutorResponse{},
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

			if diff := cmp.Diff(test.want, test.e, cmpopts.IgnoreUnexported(Emacs{})); diff != "" {
				t.Fatalf("Execute(%v) produced emacs diff (-want, +got):\n%s", test.args, diff)
			}

			changed := test.e != nil && test.e.Changed()
			if changed != test.wantChanged {
				t.Fatalf("Execute(%v) marked Changed as %v; want %v", test.args, changed, test.wantChanged)
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
		"EMACS_ARG", "[", "EMACS_ARG", "EMACS_ARG", "EMACS_ARG", "]",
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
