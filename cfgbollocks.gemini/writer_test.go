package cfgbollocks

import (
	"strings"
	"testing"
)

func TestWriterInheritanceAndSafety(t *testing.T) {
	// Setup: A list of entries where some have metadata and some don't.
	// Entry 2 is a new key added programmatically (no Separator/Delimiter).
	// Entry 3 is an existing key whose value was modified to be "unsafe".
	entries := []Entry{
		{
			Key:       "cfgbollocks",
			Value:     "format=v1\n[grammar]\nseparator = =\n",
			Separator: "~",
			Delimiter: "###",
		},
		{
			Key:   "NewKey",
			Value: "I was added later",
			// Separator and Delimiter are empty
		},
		{
			Key:       "ModifiedKey",
			Value:     "This value now contains the delimiter EOF at start of line\nEOF\n",
			Separator: "=",
			Delimiter: "EOF",
		},
	}

	var buf strings.Builder
	err := Write(&buf, entries)
	if err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	output := buf.String()

	// 1. Check Inheritance: NewKey should use '=' because cfgbollocks set it.
	if !strings.Contains(output, "NewKey = ###") {
		t.Errorf("NewKey failed to inherit '=' separator. Got:\n%s", output)
	}

	// 2. Check Safety: ModifiedKey should NOT use 'EOF' anymore.
	if strings.Contains(output, "ModifiedKey = EOF") {
		t.Errorf("Writer used an unsafe delimiter 'EOF'. Got:\n%s", output)
	}

	// Ensure it upgraded to something safe (like ###)
	if !strings.Contains(output, "ModifiedKey = ###") {
		t.Errorf("Writer failed to upgrade unsafe delimiter. Got:\n%s", output)
	}
}

func TestLosslessRoundTrip(t *testing.T) {
	// A complex input with mixed separators and custom delimiters.
	input := `cfgbollocks ~ BOOT
format = v1
###
BOOT
NormalKey ~ END
Value here
END
cfgbollocks ~ SWITCH
[grammar]
separator = =
SWITCH
LaterKey = FIN
Value there
FIN
`
	// 1. Parse it
	p := NewParser(strings.NewReader(input))
	entries, err := p.Parse()
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	// 2. Write it back
	var buf strings.Builder
	err = Write(&buf, entries)
	if err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	// 3. Verify it is structurally identical (ignoring padding as discussed)
	// We re-parse the output and compare the entries slice.
	p2 := NewParser(strings.NewReader(buf.String()))
	entries2, err := p2.Parse()
	if err != nil {
		t.Fatalf("Re-parse failed: %v", err)
	}

	if len(entries) != len(entries2) {
		t.Fatalf("Entry count mismatch. Got %d, want %d", len(entries2), len(entries))
	}

	for i := range entries {
		if entries[i].Key != entries2[i].Key || entries[i].Value != entries2[i].Value {
			t.Errorf("Entry %d mismatch.\nOriginal: %+v\nResult:   %+v", i, entries[i], entries2[i])
		}
		// Check that structural metadata survived
		if entries[i].Separator != entries2[i].Separator {
			t.Errorf("Separator mismatch at entry %d: %q vs %q", i, entries[i].Separator, entries2[i].Separator)
		}
	}
}
