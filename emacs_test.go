package emacs

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/leep-frog/command"
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
		Aliases: map[string]map[string][]string{fileAliaserName: {
			"salt": {path("compounds", "sodiumChloride")},
			"city": {path("catan", "oreAndWheat")},
		},
		},
	}

	for _, test := range []struct {
		name string
		args []string
		want []string
	}{
		// TODO: fix this in command package.
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
			suggestions := command.Autocomplete(e.Node(), test.args)
			if diff := cmp.Diff(test.want, suggestions); diff != "" {
				t.Errorf("Complete(%v) produced diff (-want, +got):\n%s", test.args, diff)
			}
		})
	}
}

func TestEmacsExecution(t *testing.T) {
	for _, test := range []struct {
		name          string
		e             *Emacs
		args          []string
		limitOverride int
		want          *Emacs
		wantEData     *command.ExecuteData
		wantErr       error
		wantData      *command.Data
		wantStdout    []string
		wantStderr    []string
	}{
		// OpenEditor tests
		{
			name:       "error when too many arguments",
			e:          &Emacs{},
			args:       []string{path("alpha.txt"), path("alpha.go"), path("file3")},
			wantStderr: []string{fmt.Sprintf("Unprocessed extra args: [%s]", path("file3"))},
			wantErr:    fmt.Errorf("Unprocessed extra args: [%s]", path("file3")),
			wantData: &command.Data{
				Values: map[string]*command.Value{
					emacsArg: command.StringListValue(absPath(t, "alpha.txt"), absPath(t, "alpha.go")),
				},
			},
			wantEData: &command.ExecuteData{
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
		{
			name: "cds into directory",
			e:    &Emacs{},
			args: []string{path("dirA")},
			wantEData: &command.ExecuteData{
				Executable: [][]string{{
					"cd",
					absPath(t, "dirA"),
				}},
			},
			wantData: &command.Data{
				Values: map[string]*command.Value{
					emacsArg: command.StringListValue(absPath(t, "dirA")),
				},
			},
			want: &Emacs{
				Caches: map[string][]string{
					cacheName: []string{absPath(t, "dirA")},
				},
			},
		},
		{
			name: "fails if file does not exist and new flag not provided",
			e:    &Emacs{},
			args: []string{path("newFile.txt")},
			wantStderr: []string{
				fmt.Sprintf(`file %q does not exist; include "new" flag to create it`, absPath(t, "newFile.txt")),
			},
			wantErr: fmt.Errorf(`file %q does not exist; include "new" flag to create it`, absPath(t, "newFile.txt")),
			wantData: &command.Data{
				Values: map[string]*command.Value{
					emacsArg: command.StringListValue(absPath(t, "newFile.txt")),
				},
			},
			want: &Emacs{
				Caches: map[string][]string{
					cacheName: []string{absPath(t, "newFile.txt")},
				},
			},
		},
		{
			name: "creates new file if short new flag is provided",
			e:    &Emacs{},
			args: []string{path("newFile.txt"), "-n"},
			wantData: &command.Data{
				Values: map[string]*command.Value{
					emacsArg:   command.StringListValue(absPath(t, "newFile.txt")),
					newFileArg: command.BoolValue(true),
				},
			},
			wantEData: &command.ExecuteData{
				Executable: [][]string{{
					"emacs",
					"--no-window-system",
					absPath(t, "newFile.txt"),
				}},
			},
			want: &Emacs{
				Caches: map[string][]string{
					cacheName: {
						absPath(t, "newFile.txt"),
						"-n",
					},
				},
			},
		},
		{
			name: "creates new file if new flag is provided",
			e:    &Emacs{},
			args: []string{path("newFile.txt"), "--new"},
			wantEData: &command.ExecuteData{
				Executable: [][]string{{
					"emacs",
					"--no-window-system",
					absPath(t, "newFile.txt"),
				}},
			},
			wantData: &command.Data{
				Values: map[string]*command.Value{
					emacsArg:   command.StringListValue(absPath(t, "newFile.txt")),
					newFileArg: command.BoolValue(true),
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
		},
		{
			name: "handles all aliases",
			e: &Emacs{
				Aliases: map[string]map[string][]string{fileAliaserName: {
					"salt": {path("compounds", "sodiumChloride")},
					"city": {path("catan", "oreAndWheat")},
				},
				},
			},
			args: []string{"salt", "city"},
			wantData: &command.Data{
				Values: map[string]*command.Value{
					emacsArg: command.StringListValue(absPath(t, "compounds", "sodiumChloride"), absPath(t, "catan", "oreAndWheat")),
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
			wantEData: &command.ExecuteData{
				Executable: [][]string{{
					"emacs",
					"--no-window-system",
					absPath(t, "catan", "oreAndWheat"),
					absPath(t, "compounds", "sodiumChloride"),
				}},
			},
		},
		{
			name: "handles line numbers",
			e: &Emacs{
				Aliases: map[string]map[string][]string{fileAliaserName: {
					"salt": {path("compounds", "sodiumChloride")},
					"city": {path("catan", "oreAndWheat")},
				},
				},
			},
			args: []string{path("alpha.txt"), "salt", "32"},
			wantData: &command.Data{
				Values: map[string]*command.Value{
					emacsArg: command.StringListValue(
						absPath(t, "alpha.txt"),
						absPath(t, "compounds", "sodiumChloride"),
					),
					lineArg: command.IntListValue(0, 32),
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
			wantEData: &command.ExecuteData{
				Executable: [][]string{{
					"emacs",
					"--no-window-system",
					"+32",
					absPath(t, "compounds", "sodiumChloride"),
					absPath(t, "alpha.txt"),
				}},
			},
		},
		{
			name: "handles multiple numbers with number filename",
			e: &Emacs{
				Aliases: map[string]map[string][]string{fileAliaserName: {
					"salt": {path("compounds", "sodiumChloride")},
					"city": {path("catan", "oreAndWheat")},
				},
				},
			},
			args: []string{"salt", "32", path("42")},
			wantData: &command.Data{
				Values: map[string]*command.Value{
					emacsArg: command.StringListValue(absPath(t, "compounds", "sodiumChloride"), absPath(t, "42")),
					lineArg:  command.IntListValue(32),
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
			wantEData: &command.ExecuteData{
				Executable: [][]string{{
					"emacs",
					"--no-window-system",
					absPath(t, "42"),
					"+32",
					absPath(t, "compounds", "sodiumChloride"),
				}},
			},
		},
		{
			name: "adds to previous executions",
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
			args: []string{path("luckyNumberThree")},
			wantData: &command.Data{
				Values: map[string]*command.Value{
					emacsArg: command.StringListValue(absPath(t, "luckyNumberThree")),
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
			wantEData: &command.ExecuteData{
				Executable: [][]string{{
					"emacs",
					"--no-window-system",
					absPath(t, "luckyNumberThree"),
				}},
			},
		},
		{
			name:          "reduces size of previous executions if at limit",
			limitOverride: 2,
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
			args: []string{path("luckyNumberThree")},
			wantData: &command.Data{
				Values: map[string]*command.Value{
					emacsArg: command.StringListValue(absPath(t, "luckyNumberThree")),
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
			wantEData: &command.ExecuteData{
				Executable: [][]string{{
					"emacs",
					"--no-window-system",
					absPath(t, "luckyNumberThree"),
				}},
			},
		},
		{
			name:       "if empty cache and no arguments, error",
			e:          &Emacs{},
			wantErr:    fmt.Errorf("not enough arguments"),
			wantStderr: []string{"not enough arguments"},
		},
		{
			name: "if nil argument, run last command",
			e: &Emacs{
				Aliases: map[string]map[string][]string{fileAliaserName: {
					"city": {path("catan", "oreAndWheat")},
				}},
				Caches: map[string][]string{
					cacheName: {
						absPath(t, "alpha.go"),
					},
				},
			},
			wantEData: &command.ExecuteData{
				Executable: [][]string{{
					"emacs",
					"--no-window-system",
					absPath(t, "alpha.go"),
				}},
			},
			wantData: &command.Data{
				Values: map[string]*command.Value{
					emacsArg: command.StringListValue(absPath(t, "alpha.go")),
				},
			},
		},
		{
			name: "if empty arguments, run last command",
			args: []string{},
			e: &Emacs{
				Aliases: map[string]map[string][]string{fileAliaserName: {
					"city": {path("catan", "oreAndWheat")},
				}},
				Caches: map[string][]string{
					cacheName: {
						absPath(t, "alpha.go"),
					},
				},
			},
			wantEData: &command.ExecuteData{
				Executable: [][]string{{
					"emacs",
					"--no-window-system",
					absPath(t, "alpha.go"),
				}},
			},
			wantData: &command.Data{
				Values: map[string]*command.Value{
					emacsArg: command.StringListValue(absPath(t, "alpha.go")),
				},
			},
		},
		// AddAlias tests
		{
			name:       "fails if no alias",
			args:       []string{"a"},
			wantStderr: []string{"not enough arguments"},
			wantErr:    fmt.Errorf("not enough arguments"),
		},
		// TODO: disallow alias with no values.
		{
			name: "handles more than one arguments",
			args: []string{"a", "duo", path("alpha.go"), path("alpha.txt")},
			wantData: &command.Data{
				Values: map[string]*command.Value{
					"ALIAS":  command.StringValue("duo"),
					emacsArg: command.StringListValue(absPath(t, "alpha.go"), absPath(t, "alpha.txt")),
				},
			},
		},
		{
			name: "fails if alias already defined",
			e: &Emacs{
				Aliases: map[string]map[string][]string{fileAliaserName: {
					"salt": {"NaCl"},
				}},
			},
			args: []string{"a", "salt", "sodiumChloride"},
			wantData: &command.Data{
				Values: map[string]*command.Value{
					"ALIAS": command.StringValue("salt"),
				},
			},
			wantStderr: []string{`Alias "salt" already exists`},
			wantErr:    fmt.Errorf(`Alias "salt" already exists`),
		},
		{
			name: "fails if file does not exist",
			e:    &Emacs{},
			args: []string{"a", "nf", path("newFile.txt")},
			wantData: &command.Data{
				Values: map[string]*command.Value{
					"ALIAS":  command.StringValue("nf"),
					emacsArg: command.StringListValue(absPath(t, "newFile.txt")),
				},
			},
			want: &Emacs{
				Caches: map[string][]string{
					cacheName: []string{absPath(t, "newFile.txt")},
				},
			},
			wantErr:    fmt.Errorf(`file %q does not exist; include "new" flag to create it`, absPath(t, "newFile.txt")),
			wantStderr: []string{fmt.Sprintf(`file %q does not exist; include "new" flag to create it`, absPath(t, "newFile.txt"))},
		},
		{
			name: "adds to nil aliases",
			e:    &Emacs{},
			args: []string{"a", "uno", path("alpha.go")},
			wantData: &command.Data{
				Values: map[string]*command.Value{
					"ALIAS":  command.StringValue("uno"),
					emacsArg: command.StringListValue(absPath(t, "alpha.go")),
				},
			},
			want: &Emacs{
				Aliases: map[string]map[string][]string{fileAliaserName: {
					"uno": {absPath(t, "alpha.go")},
				}},
				Caches: map[string][]string{
					cacheName: []string{absPath(t, "alpha.go")},
				},
			},
		},
		{
			name: "adds to empty aliases",
			e: &Emacs{
				Aliases: map[string]map[string][]string{fileAliaserName: {}},
			},
			args: []string{"a", "uno", path("alpha.txt")},
			wantData: &command.Data{
				Values: map[string]*command.Value{
					"ALIAS":  command.StringValue("uno"),
					emacsArg: command.StringListValue(absPath(t, "alpha.txt")),
				},
			},
			want: &Emacs{
				Aliases: map[string]map[string][]string{fileAliaserName: {
					"uno": {absPath(t, "alpha.txt")},
				}},
				Caches: map[string][]string{
					cacheName: []string{absPath(t, "alpha.txt")},
				},
			},
		},
		{
			name: "adds to aliases",
			e: &Emacs{
				Aliases: map[string]map[string][]string{fileAliaserName: {
					"other": {path("things")},
					"ab":    {path("cd")},
				}},
			},
			args: []string{"a", "un", path("alpha.go")},
			wantData: &command.Data{
				Values: map[string]*command.Value{
					"ALIAS":  command.StringValue("un"),
					emacsArg: command.StringListValue(absPath(t, "alpha.go")),
				},
			},
			want: &Emacs{
				Aliases: map[string]map[string][]string{fileAliaserName: {
					"other": {path("things")},
					"ab":    {path("cd")},
					"un":    {absPath(t, "alpha.go")},
				}},
				Caches: map[string][]string{
					cacheName: []string{absPath(t, "alpha.go")},
				},
			},
		},
		{
			name: "adds alias for directory",
			e:    &Emacs{},
			args: []string{"a", "t", path("dirA")},
			wantData: &command.Data{
				Values: map[string]*command.Value{
					"ALIAS":  command.StringValue("t"),
					emacsArg: command.StringListValue(absPath(t, "dirA")),
				},
			},
			want: &Emacs{
				Aliases: map[string]map[string][]string{fileAliaserName: {
					"t": {absPath(t, "dirA")},
				}},
				Caches: map[string][]string{
					cacheName: []string{absPath(t, "dirA")},
				},
			},
		},
		// DeleteAliases tests
		{
			name:       "error if no arguments",
			e:          &Emacs{},
			args:       []string{"d"},
			wantStderr: []string{"not enough arguments"},
			wantErr:    fmt.Errorf("not enough arguments"),
		},
		{
			name: "ignores unknown alias",
			e:    &Emacs{},
			args: []string{"d", "salt"},
			wantData: &command.Data{
				Values: map[string]*command.Value{
					"ALIAS": command.StringListValue("salt"),
				},
			},
			wantStderr: []string{`Alias group has no aliases yet.`},
			wantErr:    fmt.Errorf(`Alias group has no aliases yet.`),
		},
		{
			name: "deletes existing alias",
			e: &Emacs{
				Aliases: map[string]map[string][]string{fileAliaserName: {
					"salt": {path("compounds", "sodiumChloride")},
				}},
			},
			args: []string{"d", "salt"},
			wantData: &command.Data{
				Values: map[string]*command.Value{
					"ALIAS": command.StringListValue("salt"),
				},
			},
			want: &Emacs{
				Aliases: map[string]map[string][]string{fileAliaserName: {}},
			},
		},
		{
			name: "handles multiple missing and present",
			e: &Emacs{
				Aliases: map[string]map[string][]string{fileAliaserName: {
					"salt": {path("compounds", "sodiumChloride")},
					"city": {path("catan", "oreAndWheat")},
					"4":    {"2+2"},
				}},
			},
			args: []string{"d", "salt", "settlement", "5", "4"},
			wantData: &command.Data{
				Values: map[string]*command.Value{
					"ALIAS": command.StringListValue("salt", "settlement", "5", "4"),
				},
			},
			want: &Emacs{
				Aliases: map[string]map[string][]string{fileAliaserName: {
					"city": {path("catan", "oreAndWheat")},
				}},
			},
			wantStderr: []string{
				`Alias "settlement" does not exist`,
				`Alias "5" does not exist`,
			},
		},
		// ListAliases tests
		{
			name:       "error when too many arguments for list",
			args:       []string{"l", "extra"},
			wantStderr: []string{"Unprocessed extra args: [extra]"},
			wantErr:    fmt.Errorf("Unprocessed extra args: [extra]"),
		},
		{
			name: "no output for nil aliases",
			args: []string{"l"},
			e:    &Emacs{},
		},
		{
			name: "no output for empty aliases",
			args: []string{"l"},
			e: &Emacs{
				Aliases: map[string]map[string][]string{fileAliaserName: {}},
			},
		},
		{
			name: "proper output for aliases",
			args: []string{"l"},
			e: &Emacs{
				Aliases: map[string]map[string][]string{fileAliaserName: {
					"salt": {"compounds/sodiumChloride"},
					"city": {"catan", "oreAndWheat"},
					"4":    {"2+2"},
				}},
			},
			wantStdout: []string{
				"4: 2+2",
				"city: catan oreAndWheat",
				"salt: compounds/sodiumChloride",
			},
		},
		// GetAlias
		{
			name: "GetAlias requires alias",
			args: []string{"g"},
			e:    &Emacs{},
			wantStderr: []string{
				fmt.Sprintf("not enough arguments"),
			},
			wantErr: fmt.Errorf("not enough arguments"),
		},
		{
			name: "GetAlias fails if alias group does not exist",
			args: []string{"g", "salt"},
			e:    &Emacs{},
			wantData: &command.Data{
				Values: map[string]*command.Value{
					"ALIAS": command.StringListValue("salt"),
				},
			},
			wantStderr: []string{
				`No aliases exist for alias type "fileAliases"`,
			},
			wantErr: fmt.Errorf(`No aliases exist for alias type "fileAliases"`),
		},
		{
			name: "GetAlias fails if alias does not exist",
			args: []string{"g", "salt"},
			e: &Emacs{
				Aliases: map[string]map[string][]string{
					fileAliaserName: {
						"ot": []string{"h", "e", "r"},
					},
				},
			},
			wantData: &command.Data{
				Values: map[string]*command.Value{
					"ALIAS": command.StringListValue("salt"),
				},
			},
			wantStderr: []string{
				`Alias "salt" does not exist`,
			},
		},
		{
			name: "GetAlias works",
			args: []string{"g", "salt"},
			e: &Emacs{
				Aliases: map[string]map[string][]string{fileAliaserName: {
					"salt": {"compounds/sodiumChloride"},
					"city": {"catan/oreAndWheat"},
					"4":    {"2+2"},
				}},
			},
			wantStdout: []string{
				"salt: compounds/sodiumChloride",
			},
			wantData: &command.Data{
				Values: map[string]*command.Value{
					"ALIAS": command.StringListValue("salt"),
				},
			},
		},
		// SearchAliases
		{
			name: "SearchAlias requires regexp",
			args: []string{"s"},
			e:    &Emacs{},
			wantStderr: []string{
				fmt.Sprintf("not enough arguments"),
			},
			wantErr: fmt.Errorf("not enough arguments"),
		},
		{
			name: "SearchAlias requires valid regexp",
			args: []string{"s", "[a-9]"},
			e:    &Emacs{},
			wantStderr: []string{
				"Invalid regexp: error parsing regexp: invalid character class range: `a-9`",
			},
			wantData: &command.Data{
				Values: map[string]*command.Value{
					"regexp": command.StringListValue("[a-9]"),
				},
			},
			wantErr: fmt.Errorf("Invalid regexp: error parsing regexp: invalid character class range: `a-9`"),
		},
		{
			name: "SearchAlias works",
			args: []string{"s", "compounds"},
			e: &Emacs{
				Aliases: map[string]map[string][]string{fileAliaserName: {
					"water": {"liquids/compounds/hydrogenDioxide"},
					"salt":  {"compounds/sodiumChloride"},
					"city":  {"catan/oreAndWheat"},
					"4":     {"2+2"},
				},
				},
			},
			wantData: &command.Data{
				Values: map[string]*command.Value{
					"regexp": command.StringListValue("compounds"),
				},
			},
			wantStdout: []string{
				"salt: compounds/sodiumChloride",
				"water: liquids/compounds/hydrogenDioxide",
			},
		},
		/* Useful for commenting out tests. */
	} {
		t.Run(test.name, func(t *testing.T) {
			if test.limitOverride != 0 {
				oldLimit := historyLimit
				historyLimit = test.limitOverride
				defer func() { historyLimit = oldLimit }()
			}

			e := test.e
			if e == nil {
				e = &Emacs{}
			}
			command.ExecuteTest(t, e.Node(), test.args, test.wantErr, test.wantEData, test.wantData, test.wantStdout, test.wantStderr)

			// Assume wantChanged if test.want is set
			wantChanged := test.want != nil
			changed := test.e != nil && test.e.Changed()
			if changed != wantChanged {
				t.Fatalf("Execute(%v) marked Changed as %v; want %v", test.args, changed, wantChanged)
			}

			// Only check diff if we are expecting a change.
			if wantChanged {
				opts := []cmp.Option{
					cmpopts.IgnoreUnexported(Emacs{}), // commands.Value{}),
					protocmp.Transform(),
					cmpopts.EquateEmpty(),
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
