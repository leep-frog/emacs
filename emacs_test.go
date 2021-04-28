package emacs

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/leep-frog/command"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
)

func TestLoad(t *testing.T) {
	for _, test := range []struct {
		name    string
		json    string
		want    *Emacs
		WantErr string
	}{
		{
			name: "handles empty string",
			want: &Emacs{},
		},
		{
			name:    "errors on invalid json",
			json:    "}",
			want:    &Emacs{},
			WantErr: "failed to unmarshal emacs json",
		},
		{
			name: "properly unmarshals",
			json: fmt.Sprintf(`{"Aliases":{"%s":{"city":["catan", "oreAndWheat"]}},"PreviousExecutions":null}`, fileAliaserName),
			want: &Emacs{
				Aliases: map[string]map[string][]string{fileAliaserName: {
					"city": {"catan", "oreAndWheat"},
				},
				},
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			e := &Emacs{}

			err := e.Load(test.json)
			if err != nil && test.WantErr == "" {
				t.Fatalf("Load(%v) returned error (%v); want nil", test.json, err)
			} else if err == nil && test.WantErr != "" {
				t.Fatalf("Load(%v) returned nil; want error (%v)", test.json, test.WantErr)
			} else if err != nil && test.WantErr != "" && !strings.Contains(err.Error(), test.WantErr) {
				t.Fatalf("Load(%v) returned error (%v); want (%v)", test.json, err, test.WantErr)
			}

			if diff := cmp.Diff(test.want, e, cmpopts.IgnoreUnexported(Emacs{})); diff != "" {
				t.Errorf("Load(%v) produced emacs diff (-want, +got):\n%s", test.json, diff)
			}
		})
	}
}

func TestAutocomplete(t *testing.T) {
	e := &Emacs{
		Aliases: map[string]map[string][]string{fileAliaserName: {
			"salt": {path("compounds", "sodiumChloride")},
			"city": {path("catan", "oreAndWheat")},
		},
		},
	}

	for _, test := range []struct {
		name string
		ctc  *command.CompleteTestCase
	}{
		{
			name: "doesn't suggest subcommands",
			ctc: &command.CompleteTestCase{
				Want: []string{
					".git/",
					"emacs.go",
					"emacs_test.go",
					"go.mod",
					"go.sum",
					"testing/",
					" ",
				},
				WantData: &command.Data{
					Values: map[string]*command.Value{
						emacsArg: command.StringListValue(""),
					},
				},
			},
		},
		{
			name: "file suggestions ignore case",
			ctc: &command.CompleteTestCase{
				Args: []string{"EmA"},
				Want: []string{
					"emacs",
					"emacs_",
				},
				WantData: &command.Data{
					Values: map[string]*command.Value{
						emacsArg: command.StringListValue("EmA"),
					},
				},
			},
		},
		{
			name: "suggests only files after first command",
			ctc: &command.CompleteTestCase{
				Args: []string{"file1.txt", ""},
				Want: []string{
					".git/",
					"emacs.go",
					"emacs_test.go",
					"go.mod",
					"go.sum",
					"testing/",
					" ",
				},
				WantData: &command.Data{
					Values: map[string]*command.Value{
						emacsArg: command.StringListValue("file1.txt", ""),
					},
				},
			},
		},
		{
			name: "doesn't include files already included",
			ctc: &command.CompleteTestCase{
				Args: []string{"emacs.go", "e"},
				Want: []string{
					"emacs_test.go",
				},
				WantData: &command.Data{
					Values: map[string]*command.Value{
						emacsArg: command.StringListValue("emacs.go", "e"),
					},
				},
			},
		},
		{
			name: "doesn't include files a directory down that are already included",
			ctc: &command.CompleteTestCase{
				Args: []string{"testing/alpha.txt", "testing/a"},
				Want: []string{
					"testing/alpha.go",
				},
				WantData: &command.Data{
					Values: map[string]*command.Value{
						emacsArg: command.StringListValue("testing/alpha.txt", "testing/a"),
					},
				},
			},
		},
		// aliasFetcher tests
		{
			name: "suggests only aliases for delete",
			ctc: &command.CompleteTestCase{
				Args: []string{"d", ""},
				Want: []string{
					"city",
					"salt",
				},
				WantData: &command.Data{
					Values: map[string]*command.Value{
						aliasArg: command.StringListValue(""),
					},
				},
			},
		},
		// GetAlias
		{
			name: "suggests aliases for get",
			ctc: &command.CompleteTestCase{
				Args: []string{"g", ""},
				Want: []string{
					"city",
					"salt",
				},
				WantData: &command.Data{
					Values: map[string]*command.Value{
						aliasArg: command.StringListValue(""),
					},
				},
			},
		},
		{
			name: "completes partial alias for get",
			ctc: &command.CompleteTestCase{
				Args: []string{"g", "s"},
				Want: []string{
					"salt",
				},
				WantData: &command.Data{
					Values: map[string]*command.Value{
						aliasArg: command.StringListValue("s"),
					},
				},
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			test.ctc.Node = e.Node()
			command.CompleteTest(t, test.ctc, nil)
		})
	}
}

func TestEmacsExecution(t *testing.T) {
	for _, test := range []struct {
		name string
		e    *Emacs
		etc  *command.ExecuteTestCase
		want *Emacs
	}{
		// OpenEditor tests
		{
			name: "error when too many arguments",
			etc: &command.ExecuteTestCase{
				Args:       []string{path("alpha.txt"), path("alpha.go"), path("file3")},
				WantStderr: []string{fmt.Sprintf("Unprocessed extra args: [%s]", path("file3"))},
				WantErr:    fmt.Errorf("Unprocessed extra args: [%s]", path("file3")),
				WantData: &command.Data{
					Values: map[string]*command.Value{
						emacsArg: command.StringListValue(absPath(t, "alpha.txt"), absPath(t, "alpha.go")),
					},
				},
				WantExecuteData: &command.ExecuteData{
					Executable: [][]string{
						{
							"emacs",
							"--no-window-system",
							absPath(t, "alpha.go"),
							absPath(t, "alpha.txt"),
						},
					},
				},
			},
		}, {
			name: "cds into directory",
			etc: &command.ExecuteTestCase{
				Args: []string{path("dirA")},
				WantExecuteData: &command.ExecuteData{
					Executable: [][]string{{
						"cd",
						absPath(t, "dirA"),
					}},
				},
				WantData: &command.Data{
					Values: map[string]*command.Value{
						emacsArg: command.StringListValue(absPath(t, "dirA")),
					},
				},
			},
			want: &Emacs{
				Caches: map[string][]string{
					cacheName: {absPath(t, "dirA")},
				},
			},
		}, {
			name: "fails if file does not exist and new flag not provided",
			etc: &command.ExecuteTestCase{
				Args: []string{path("newFile.txt")},
				WantStderr: []string{
					fmt.Sprintf(`file %q does not exist; include "new" flag to create it`, absPath(t, "newFile.txt")),
				},
				WantErr: fmt.Errorf(`file %q does not exist; include "new" flag to create it`, absPath(t, "newFile.txt")),
				WantData: &command.Data{
					Values: map[string]*command.Value{
						emacsArg: command.StringListValue(absPath(t, "newFile.txt")),
					},
				},
			},
			want: &Emacs{
				Caches: map[string][]string{
					cacheName: {absPath(t, "newFile.txt")},
				},
			},
		}, {
			name: "creates new file if short new flag is provided",
			etc: &command.ExecuteTestCase{
				Args: []string{path("newFile.txt"), "-n"},
				WantData: &command.Data{
					Values: map[string]*command.Value{
						emacsArg:   command.StringListValue(absPath(t, "newFile.txt")),
						newFileArg: command.BoolValue(true),
					},
				},
				WantExecuteData: &command.ExecuteData{
					Executable: [][]string{{
						"emacs",
						"--no-window-system",
						absPath(t, "newFile.txt"),
					}},
				},
			},
			want: &Emacs{
				Caches: map[string][]string{
					cacheName: {
						absPath(t, "newFile.txt"),
						"-n",
					},
				},
			},
		}, {
			name: "creates new file if new flag is provided",
			etc: &command.ExecuteTestCase{
				Args: []string{path("newFile.txt"), "--new"},
				WantExecuteData: &command.ExecuteData{
					Executable: [][]string{{
						"emacs",
						"--no-window-system",
						absPath(t, "newFile.txt"),
					}},
				},
				WantData: &command.Data{
					Values: map[string]*command.Value{
						emacsArg:   command.StringListValue(absPath(t, "newFile.txt")),
						newFileArg: command.BoolValue(true),
					},
				},
			},
			want: &Emacs{
				Caches: map[string][]string{
					cacheName: {
						absPath(t, "newFile.txt"),
						"--new",
					},
				},
			},
		}, {
			name: "handles all aliases",
			e: &Emacs{
				Aliases: map[string]map[string][]string{
					fileAliaserName: {
						"salt": {path("compounds", "sodiumChloride")},
						"city": {path("catan", "oreAndWheat")},
					},
				},
			},
			etc: &command.ExecuteTestCase{
				Args: []string{"salt", "city"},
				WantData: &command.Data{
					Values: map[string]*command.Value{
						emacsArg: command.StringListValue(absPath(t, "compounds", "sodiumChloride"), absPath(t, "catan", "oreAndWheat")),
					},
				},
				WantExecuteData: &command.ExecuteData{
					Executable: [][]string{{
						"emacs",
						"--no-window-system",
						absPath(t, "catan", "oreAndWheat"),
						absPath(t, "compounds", "sodiumChloride"),
					}},
				},
			},
			want: &Emacs{
				Aliases: map[string]map[string][]string{
					fileAliaserName: {
						"salt": {path("compounds", "sodiumChloride")},
						"city": {path("catan", "oreAndWheat")},
					},
				},
				Caches: map[string][]string{cacheName: {
					absPath(t, "compounds", "sodiumChloride"),
					absPath(t, "catan", "oreAndWheat"),
				}},
			},
		}, {
			name: "handles line numbers",
			e: &Emacs{
				Aliases: map[string]map[string][]string{
					fileAliaserName: {
						"salt": {path("compounds", "sodiumChloride")},
						"city": {path("catan", "oreAndWheat")},
					},
				},
			},
			etc: &command.ExecuteTestCase{
				Args: []string{path("alpha.txt"), "salt", "32"},
				WantData: &command.Data{
					Values: map[string]*command.Value{
						emacsArg: command.StringListValue(
							absPath(t, "alpha.txt"),
							absPath(t, "compounds", "sodiumChloride"),
						),
						lineArg: command.IntListValue(0, 32),
					},
				},
				WantExecuteData: &command.ExecuteData{
					Executable: [][]string{{
						"emacs",
						"--no-window-system",
						"+32",
						absPath(t, "compounds", "sodiumChloride"),
						absPath(t, "alpha.txt"),
					}},
				},
			},
			want: &Emacs{
				Aliases: map[string]map[string][]string{fileAliaserName: {
					"salt": {path("compounds", "sodiumChloride")},
					"city": {path("catan", "oreAndWheat")},
				},
				},
				Caches: map[string][]string{
					cacheName: {
						absPath(t, "alpha.txt"),
						absPath(t, "compounds", "sodiumChloride"),
						"32",
					},
				},
			},
		}, {
			name: "handles multiple numbers with number filename",
			e: &Emacs{
				Aliases: map[string]map[string][]string{
					fileAliaserName: {
						"salt": {path("compounds", "sodiumChloride")},
						"city": {path("catan", "oreAndWheat")},
					},
				},
			},
			etc: &command.ExecuteTestCase{
				Args: []string{"salt", "32", path("42")},
				WantData: &command.Data{
					Values: map[string]*command.Value{
						emacsArg: command.StringListValue(absPath(t, "compounds", "sodiumChloride"), absPath(t, "42")),
						lineArg:  command.IntListValue(32),
					},
				},
				WantExecuteData: &command.ExecuteData{
					Executable: [][]string{{
						"emacs",
						"--no-window-system",
						absPath(t, "42"),
						"+32",
						absPath(t, "compounds", "sodiumChloride"),
					}},
				},
			},
			want: &Emacs{
				Aliases: map[string]map[string][]string{fileAliaserName: {
					"salt": {path("compounds", "sodiumChloride")},
					"city": {path("catan", "oreAndWheat")},
				},
				},
				Caches: map[string][]string{
					cacheName: {
						absPath(t, "compounds", "sodiumChloride"),
						"32",
						absPath(t, "42"),
					},
				},
			},
		}, {
			name: "adds to previous executions",
			e: &Emacs{
				Aliases: map[string]map[string][]string{
					fileAliaserName: {
						"city": {path("catan", "oreAndWheat")},
					},
				},
				Caches: map[string][]string{
					cacheName: {
						"firstFile",
					},
				},
			},
			etc: &command.ExecuteTestCase{
				Args: []string{path("luckyNumberThree")},
				WantData: &command.Data{
					Values: map[string]*command.Value{
						emacsArg: command.StringListValue(absPath(t, "luckyNumberThree")),
					},
				},
				WantExecuteData: &command.ExecuteData{
					Executable: [][]string{{
						"emacs",
						"--no-window-system",
						absPath(t, "luckyNumberThree"),
					}},
				},
			},
			want: &Emacs{
				Aliases: map[string]map[string][]string{fileAliaserName: {
					"city": {path("catan", "oreAndWheat")},
				},
				},
				Caches: map[string][]string{
					cacheName: {
						absPath(t, "luckyNumberThree"),
					},
				},
			},
		}, {
			name: "reduces size of previous executions if at limit",
			e: &Emacs{
				Aliases: map[string]map[string][]string{fileAliaserName: {
					"city": {path("catan", "oreAndWheat")},
				},
				},
				Caches: map[string][]string{
					cacheName: {
						"firstFile",
					},
				},
			},
			etc: &command.ExecuteTestCase{
				Args: []string{path("luckyNumberThree")},
				WantData: &command.Data{
					Values: map[string]*command.Value{
						emacsArg: command.StringListValue(absPath(t, "luckyNumberThree")),
					},
				},
				WantExecuteData: &command.ExecuteData{
					Executable: [][]string{{
						"emacs",
						"--no-window-system",
						absPath(t, "luckyNumberThree"),
					}},
				},
			},
			want: &Emacs{
				Aliases: map[string]map[string][]string{fileAliaserName: {
					"city": {path("catan", "oreAndWheat")},
				}},
				Caches: map[string][]string{
					cacheName: {
						absPath(t, "luckyNumberThree"),
					},
				},
			},
		}, {
			name: "if empty cache and no arguments, error",
			etc: &command.ExecuteTestCase{
				WantErr:    fmt.Errorf("not enough arguments"),
				WantStderr: []string{"not enough arguments"},
			},
		}, {
			name: "if nil argument, run last command",
			e: &Emacs{
				Aliases: map[string]map[string][]string{
					fileAliaserName: {
						"city": {path("catan", "oreAndWheat")},
					}},
				Caches: map[string][]string{
					cacheName: {
						absPath(t, "alpha.go"),
					},
				},
			},
			etc: &command.ExecuteTestCase{
				WantExecuteData: &command.ExecuteData{
					Executable: [][]string{{
						"emacs",
						"--no-window-system",
						absPath(t, "alpha.go"),
					}},
				},
				WantData: &command.Data{
					Values: map[string]*command.Value{
						emacsArg: command.StringListValue(absPath(t, "alpha.go")),
					},
				},
			},
		}, {
			name: "if empty arguments, run last command",
			e: &Emacs{
				Aliases: map[string]map[string][]string{
					fileAliaserName: {
						"city": {path("catan", "oreAndWheat")},
					}},
				Caches: map[string][]string{
					cacheName: {
						absPath(t, "alpha.go"),
					},
				},
			},
			etc: &command.ExecuteTestCase{
				Args: []string{},
				WantExecuteData: &command.ExecuteData{
					Executable: [][]string{{
						"emacs",
						"--no-window-system",
						absPath(t, "alpha.go"),
					}},
				},
				WantData: &command.Data{
					Values: map[string]*command.Value{
						emacsArg: command.StringListValue(absPath(t, "alpha.go")),
					},
				},
			},
		}, // AddAlias tests
		{
			name: "fails if no alias",
			etc: &command.ExecuteTestCase{
				Args:       []string{"a"},
				WantStderr: []string{"not enough arguments"},
				WantErr:    fmt.Errorf("not enough arguments"),
			},
		},
		{
			name: "handles more than one arguments",
			etc: &command.ExecuteTestCase{
				Args: []string{"a", "duo", path("alpha.go"), path("alpha.txt")},
				WantData: &command.Data{
					Values: map[string]*command.Value{
						"ALIAS":  command.StringValue("duo"),
						emacsArg: command.StringListValue(absPath(t, "alpha.go"), absPath(t, "alpha.txt")),
					},
				},
			},
			want: &Emacs{
				Aliases: map[string]map[string][]string{
					fileAliaserName: {
						"duo": []string{absPath(t, "alpha.go"), absPath(t, "alpha.txt")},
					},
				},
				Caches: map[string][]string{
					cacheName: {absPath(t, "alpha.go"), absPath(t, "alpha.txt")},
				},
			},
		}, {
			name: "fails if alias already defined",
			e: &Emacs{
				Aliases: map[string]map[string][]string{
					fileAliaserName: {
						"salt": {"NaCl"},
					}},
			},
			etc: &command.ExecuteTestCase{
				Args: []string{"a", "salt", "sodiumChloride"},
				WantData: &command.Data{
					Values: map[string]*command.Value{
						"ALIAS": command.StringValue("salt"),
					},
				},
				WantStderr: []string{`Alias "salt" already exists`},
				WantErr:    fmt.Errorf(`Alias "salt" already exists`),
			},
		}, {
			name: "fails if file does not exist",
			etc: &command.ExecuteTestCase{
				Args: []string{"a", "nf", path("newFile.txt")},
				WantData: &command.Data{
					Values: map[string]*command.Value{
						"ALIAS":  command.StringValue("nf"),
						emacsArg: command.StringListValue(absPath(t, "newFile.txt")),
					},
				},
				WantErr:    fmt.Errorf(`file %q does not exist; include "new" flag to create it`, absPath(t, "newFile.txt")),
				WantStderr: []string{fmt.Sprintf(`file %q does not exist; include "new" flag to create it`, absPath(t, "newFile.txt"))},
			},
			want: &Emacs{
				Caches: map[string][]string{
					cacheName: {absPath(t, "newFile.txt")},
				},
			},
		}, {
			name: "adds to nil aliases",
			etc: &command.ExecuteTestCase{
				Args: []string{"a", "uno", path("alpha.go")},
				WantData: &command.Data{
					Values: map[string]*command.Value{
						"ALIAS":  command.StringValue("uno"),
						emacsArg: command.StringListValue(absPath(t, "alpha.go")),
					},
				},
			},
			want: &Emacs{
				Aliases: map[string]map[string][]string{fileAliaserName: {
					"uno": {absPath(t, "alpha.go")},
				}},
				Caches: map[string][]string{
					cacheName: {absPath(t, "alpha.go")},
				},
			},
		}, {
			name: "adds to empty aliases",
			e: &Emacs{
				Aliases: map[string]map[string][]string{
					fileAliaserName: {},
				},
			},
			etc: &command.ExecuteTestCase{
				Args: []string{"a", "uno", path("alpha.txt")},
				WantData: &command.Data{
					Values: map[string]*command.Value{
						"ALIAS":  command.StringValue("uno"),
						emacsArg: command.StringListValue(absPath(t, "alpha.txt")),
					},
				},
			},
			want: &Emacs{
				Aliases: map[string]map[string][]string{fileAliaserName: {
					"uno": {absPath(t, "alpha.txt")},
				}},
				Caches: map[string][]string{
					cacheName: {absPath(t, "alpha.txt")},
				},
			},
		}, {
			name: "adds to aliases",
			e: &Emacs{
				Aliases: map[string]map[string][]string{
					fileAliaserName: {
						"other": {path("things")},
						"ab":    {path("cd")},
					},
				},
			},
			etc: &command.ExecuteTestCase{
				Args: []string{"a", "un", path("alpha.go")},
				WantData: &command.Data{
					Values: map[string]*command.Value{
						"ALIAS":  command.StringValue("un"),
						emacsArg: command.StringListValue(absPath(t, "alpha.go")),
					},
				},
			},
			want: &Emacs{
				Aliases: map[string]map[string][]string{fileAliaserName: {
					"other": {path("things")},
					"ab":    {path("cd")},
					"un":    {absPath(t, "alpha.go")},
				}},
				Caches: map[string][]string{
					cacheName: {absPath(t, "alpha.go")},
				},
			},
		}, {
			name: "adds alias for directory",
			etc: &command.ExecuteTestCase{
				Args: []string{"a", "t", path("dirA")},
				WantData: &command.Data{
					Values: map[string]*command.Value{
						"ALIAS":  command.StringValue("t"),
						emacsArg: command.StringListValue(absPath(t, "dirA")),
					},
				},
			},
			want: &Emacs{
				Aliases: map[string]map[string][]string{fileAliaserName: {
					"t": {absPath(t, "dirA")},
				}},
				Caches: map[string][]string{
					cacheName: {absPath(t, "dirA")},
				},
			},
		}, // DeleteAliases tests
		{
			name: "error if no arguments",
			etc: &command.ExecuteTestCase{
				Args:       []string{"d"},
				WantStderr: []string{"not enough arguments"},
				WantErr:    fmt.Errorf("not enough arguments"),
			},
		}, {
			name: "ignores unknown alias",
			etc: &command.ExecuteTestCase{
				Args: []string{"d", "salt"},
				WantData: &command.Data{
					Values: map[string]*command.Value{
						"ALIAS": command.StringListValue("salt"),
					},
				},
				WantStderr: []string{`Alias group has no aliases yet.`},
				WantErr:    fmt.Errorf(`Alias group has no aliases yet.`),
			},
		}, {
			name: "deletes existing alias",
			e: &Emacs{
				Aliases: map[string]map[string][]string{
					fileAliaserName: {
						"salt": {path("compounds", "sodiumChloride")},
					},
				},
			},
			etc: &command.ExecuteTestCase{
				Args: []string{"d", "salt"},
				WantData: &command.Data{
					Values: map[string]*command.Value{
						"ALIAS": command.StringListValue("salt"),
					},
				},
			},
			want: &Emacs{
				Aliases: map[string]map[string][]string{fileAliaserName: {}},
			},
		}, {
			name: "handles multiple missing and present",
			e: &Emacs{
				Aliases: map[string]map[string][]string{
					fileAliaserName: {
						"salt": {path("compounds", "sodiumChloride")},
						"city": {path("catan", "oreAndWheat")},
						"4":    {"2+2"},
					},
				},
			},
			etc: &command.ExecuteTestCase{
				Args: []string{"d", "salt", "settlement", "5", "4"},
				WantData: &command.Data{
					Values: map[string]*command.Value{
						"ALIAS": command.StringListValue("salt", "settlement", "5", "4"),
					},
				},
				WantStderr: []string{
					`Alias "settlement" does not exist`,
					`Alias "5" does not exist`,
				},
			},
			want: &Emacs{
				Aliases: map[string]map[string][]string{fileAliaserName: {
					"city": {path("catan", "oreAndWheat")},
				}},
			},
		}, // ListAliases tests
		{
			name: "error when too many arguments for list",
			etc: &command.ExecuteTestCase{
				Args:       []string{"l", "extra"},
				WantStderr: []string{"Unprocessed extra args: [extra]"},
				WantErr:    fmt.Errorf("Unprocessed extra args: [extra]"),
			},
		}, {
			name: "no output for nil aliases",
			etc: &command.ExecuteTestCase{
				Args: []string{"l"},
			},
		}, {
			name: "no output for empty aliases",
			e: &Emacs{
				Aliases: map[string]map[string][]string{
					fileAliaserName: {},
				},
			},
			etc: &command.ExecuteTestCase{
				Args: []string{"l"},
			},
		}, {
			name: "proper output for aliases",
			e: &Emacs{
				Aliases: map[string]map[string][]string{
					fileAliaserName: {
						"salt": {"compounds/sodiumChloride"},
						"city": {"catan", "oreAndWheat"},
						"4":    {"2+2"},
					},
				},
			},
			etc: &command.ExecuteTestCase{
				Args: []string{"l"},
				WantStdout: []string{
					"4: 2+2",
					"city: catan oreAndWheat",
					"salt: compounds/sodiumChloride",
				},
			},
		}, // GetAlias
		{
			name: "GetAlias requires alias",
			etc: &command.ExecuteTestCase{
				Args: []string{"g"},
				WantStderr: []string{
					fmt.Sprintf("not enough arguments"),
				},
				WantErr: fmt.Errorf("not enough arguments"),
			},
		}, {
			name: "GetAlias fails if alias group does not exist",
			etc: &command.ExecuteTestCase{
				Args: []string{"g", "salt"},
				WantData: &command.Data{
					Values: map[string]*command.Value{
						"ALIAS": command.StringListValue("salt"),
					},
				},
				WantStderr: []string{
					`No aliases exist for alias type "fileAliases"`,
				},
				WantErr: fmt.Errorf(`No aliases exist for alias type "fileAliases"`),
			},
		}, {
			name: "GetAlias fails if alias does not exist",
			e: &Emacs{
				Aliases: map[string]map[string][]string{
					fileAliaserName: {
						"ot": []string{"h", "e", "r"},
					},
				},
			},
			etc: &command.ExecuteTestCase{
				Args: []string{"g", "salt"},
				WantData: &command.Data{
					Values: map[string]*command.Value{
						"ALIAS": command.StringListValue("salt"),
					},
				},
				WantStderr: []string{
					`Alias "salt" does not exist`,
				},
			},
		}, {
			name: "GetAlias works",
			e: &Emacs{
				Aliases: map[string]map[string][]string{
					fileAliaserName: {
						"salt": {"compounds/sodiumChloride"},
						"city": {"catan/oreAndWheat"},
						"4":    {"2+2"},
					}},
			},
			etc: &command.ExecuteTestCase{
				Args: []string{"g", "salt"},
				WantStdout: []string{
					"salt: compounds/sodiumChloride",
				},
				WantData: &command.Data{
					Values: map[string]*command.Value{
						"ALIAS": command.StringListValue("salt"),
					},
				},
			},
		}, // SearchAliases
		{
			name: "SearchAlias requires regexp",
			etc: &command.ExecuteTestCase{
				Args: []string{"s"},
				WantStderr: []string{
					fmt.Sprintf("not enough arguments"),
				},
				WantErr: fmt.Errorf("not enough arguments"),
			},
		}, {
			name: "SearchAlias requires valid regexp",
			etc: &command.ExecuteTestCase{
				Args: []string{"s", "[a-9]"},
				WantStderr: []string{
					"Invalid regexp: error parsing regexp: invalid character class range: `a-9`",
				},
				WantData: &command.Data{
					Values: map[string]*command.Value{
						"regexp": command.StringListValue("[a-9]"),
					},
				},
				WantErr: fmt.Errorf("Invalid regexp: error parsing regexp: invalid character class range: `a-9`"),
			},
		}, {
			name: "SearchAlias works",
			e: &Emacs{
				Aliases: map[string]map[string][]string{
					fileAliaserName: {
						"water": {"liquids/compounds/hydrogenDioxide"},
						"salt":  {"compounds/sodiumChloride"},
						"city":  {"catan/oreAndWheat"},
						"4":     {"2+2"},
					},
				},
			},
			etc: &command.ExecuteTestCase{
				Args: []string{"s", "compounds"},
				WantData: &command.Data{
					Values: map[string]*command.Value{
						"regexp": command.StringListValue("compounds"),
					},
				},
				WantStdout: []string{
					"salt: compounds/sodiumChloride",
					"water: liquids/compounds/hydrogenDioxide",
				},
			},
		},
		/* Useful for commenting out tests. */
	} {
		t.Run(test.name, func(t *testing.T) {
			if test.e == nil {
				test.e = &Emacs{}
			}
			test.etc.Node = test.e.Node()
			command.ExecuteTest(t, test.etc, nil)
			command.ChangeTest(t, test.want, test.e, cmpopts.IgnoreUnexported(Emacs{}), cmpopts.EquateEmpty())
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
	want := "e"
	if e.Name() != want {
		t.Errorf("Incorrect emacs name: got %s; want %s", e.Name(), want)
	}
}

func absPath(t *testing.T, sl ...string) string {
	t.Helper()
	r, err := filepath.Abs(path(sl...))
	if err != nil {
		t.Fatalf("filepath.Abs(%s) returned error: %v", sl, err)
	}
	return r
}

func path(sl ...string) string {
	r := []string{"testing"}
	r = append(r, sl...)
	return filepath.Join(r...)
}
