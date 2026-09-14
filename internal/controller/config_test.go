package controller

import (
	"strings"
	"testing"
)

func TestSupersetPythonConfigMarshaler(t *testing.T) {
	marshaler := supersetPythonConfigMarshaler{}

	t.Run("renders header defaults sorted overrides and footer in order", func(t *testing.T) {
		got, err := marshaler.Marshal(map[string]string{
			configOverrideFileFooterKey:       "FOOTER_VALUE = True",
			"Z_LAST":                          "2",
			supersetConfigGeneratedContentKey: "OPERATOR_DEFAULT = True\n",
			configOverrideFileHeaderKey:       "HEADER_VALUE = True\n",
			"A_FIRST":                         "'first'",
		})
		if err != nil {
			t.Fatalf("Marshal() error = %v", err)
		}

		want := "" +
			"HEADER_VALUE = True\n" +
			"OPERATOR_DEFAULT = True\n" +
			"A_FIRST = 'first'\n" +
			"Z_LAST = 2\n" +
			"FOOTER_VALUE = True\n"
		if got != want {
			t.Fatalf("Marshal() =\n%s\nwant exactly:\n%s", got, want)
		}
	})

	t.Run("preserves multiline fragments and inserts only missing boundaries", func(t *testing.T) {
		got, err := marshaler.Marshal(map[string]string{
			configOverrideFileHeaderKey:       "if True:\n    HEADER = 1",
			supersetConfigGeneratedContentKey: "BODY = {\n    'enabled': True,\n}",
			configOverrideFileFooterKey:       "\nFOOTER = 1\n",
		})
		if err != nil {
			t.Fatalf("Marshal() error = %v", err)
		}
		want := "if True:\n    HEADER = 1\nBODY = {\n    'enabled': True,\n}\n\nFOOTER = 1\n"
		if got != want {
			t.Fatalf("Marshal() = %q, want %q", got, want)
		}
	})

	t.Run("rejects an empty ordinary value instead of emitting invalid Python", func(t *testing.T) {
		_, err := marshaler.Marshal(map[string]string{
			supersetConfigGeneratedContentKey: "OPERATOR_DEFAULT = True\n",
			"EMPTY":                           "",
		})
		if err == nil || !strings.Contains(err.Error(), "EMPTY") {
			t.Fatalf("Marshal() error = %v, want an error naming the empty override", err)
		}
	})

	t.Run("requires the resolver supplied operator defaults", func(t *testing.T) {
		_, err := marshaler.Marshal(map[string]string{
			configOverrideFileHeaderKey: "HEADER_ONLY = True",
		})
		if err == nil || !strings.Contains(err.Error(), supersetConfigGeneratedContentKey) {
			t.Fatalf("Marshal() error = %v, want missing generated-content key", err)
		}
	})
}

func TestMainContainerCommands_CopiesConfigMapSymlinkTargets(t *testing.T) {
	commands := mainContainerCommands()
	want := "cp -RL /kubedoop/mount/config/* /kubedoop/app/pythonpath"
	if !strings.Contains(commands, want) {
		t.Fatalf("mainContainerCommands() does not contain %q:\n%s", want, commands)
	}
}
