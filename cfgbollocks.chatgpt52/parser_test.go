package cfgbollocks

import (
    "strings"
    "testing"
)
func TestHeaderRules(t *testing.T) {
    tests := []struct {
        name    string
        input   string
        wantErr bool
    }{
        {
            name: "valid minimal header",
            input: "cfgbollocks = |###| ###v1###",
        },
        {
            name: "valid header with BOM",
            input: "\uFEFFcfgbollocks = |###| ###v1###",
        },
        {
            name:    "missing header",
            input:   "key = ###value###",
            wantErr: true,
        },
        {
            name:    "whitespace before header",
            input:   "\ncfgbollocks = |###| ###v1###",
            wantErr: true,
        },
        {
            name:    "wrong header key casing",
            input:   "CfgBollocks = |###| ###v1###",
            wantErr: true,
        },
        {
            name:    "unsupported version",
            input:   "cfgbollocks = |###| ###v2###",
            wantErr: true,
        },
        {
            name: "cfgbollocks later has no meaning",
            input: strings.Join([]string{
                "cfgbollocks = |###| ###v1###",
                "a = ###1###",
                "cfgbollocks = ###ignored###",
            }, "\n"),
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            _, err := Parse(tt.input)
            if tt.wantErr && err == nil {
                t.Fatalf("expected error, got nil")
            }
            if !tt.wantErr && err != nil {
                t.Fatalf("unexpected error: %v", err)
            }
        })
    }
}

func TestHeaderTokenValidation(t *testing.T) {
    tests := []struct {
        name    string
        input   string
        wantErr bool
    }{
        {
            name: "empty declarator",
            input: "cfgbollocks = || ###v1###",
            wantErr: true,
        },
        {
            name: "whitespace in declarator",
            input: "cfgbollocks = | | ###v1###",
            wantErr: true,
        },
        {
            name: "token collision",
            input: "cfgbollocks = |#| #v1#",
            wantErr: true,
        },
        {
            name: "valid tokens",
            input: "cfgbollocks = |@@| ###v1###",
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            _, err := Parse(tt.input)
            if tt.wantErr && err == nil {
                t.Fatalf("expected error, got nil")
            }
            if !tt.wantErr && err != nil {
                t.Fatalf("unexpected error: %v", err)
            }
        })
    }
}

func TestRecords(t *testing.T) {
    tests := []struct {
        name  string
        input string
        want  []Entry
    }{
        {
            name: "single record default delimiter",
            input: strings.Join([]string{
                "cfgbollocks = |###| ###v1###",
                "path = ###/tmp/file###",
            }, "\n"),
            want: []Entry{
                {Key: "path", Value: "/tmp/file", Delimiter: "###"},
            },
        },
        {
            name: "inline delimiter",
            input: strings.Join([]string{
                "cfgbollocks = |###| ###v1###",
                "script = |@@@| @@@echo hi@@@",
            }, "\n"),
            want: []Entry{
                {Key: "script", Value: "echo hi", Delimiter: "@@@"},
            },
        },
        {
            name: "value with newlines preserved",
            input: strings.Join([]string{
                "cfgbollocks = |###| ###v1###",
                "data = ###line1",
                "line2",
                "###",
            }, "\n"),
            want: []Entry{
                {Key: "data", Value: "line1\nline2\n", Delimiter: "###"},
            },
        },
        {
            name: "empty value",
            input: strings.Join([]string{
                "cfgbollocks = |###| ###v1###",
                "empty = ######",
            }, "\n"),
            want: []Entry{
                {Key: "empty", Value: "", Delimiter: "###"},
            },
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            got, err := Parse(tt.input)
            if err != nil {
                t.Fatalf("unexpected error: %v", err)
            }
            if len(got) != len(tt.want) {
                t.Fatalf("got %d entries, want %d", len(got), len(tt.want))
            }
            for i := range got {
                if got[i] != tt.want[i] {
                    t.Fatalf("entry %d mismatch:\n got %#v\nwant %#v", i, got[i], tt.want[i])
                }
            }
        })
    }
}

func TestDelimiterErrors(t *testing.T) {
    tests := []struct {
        name    string
        input   string
        wantErr bool
    }{
        {
            name: "delimiter inside value",
            input: strings.Join([]string{
                "cfgbollocks = |###| ###v1###",
                "bad = ###hello ### world###",
            }, "\n"),
            wantErr: true,
        },
        {
            name: "trailing whitespace after delimiter",
            input: strings.Join([]string{
                "cfgbollocks = |###| ###v1###",
                "x = ###ok### ",
            }, "\n"),
            wantErr: true,
        },
        {
            name: "unterminated value",
            input: strings.Join([]string{
                "cfgbollocks = |###| ###v1###",
                "x = ###oops",
            }, "\n"),
            wantErr: true,
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            _, err := Parse(tt.input)
            if tt.wantErr && err == nil {
                t.Fatalf("expected error, got nil")
            }
            if !tt.wantErr && err != nil {
                t.Fatalf("unexpected error: %v", err)
            }
        })
    }
}

func TestInvariants(t *testing.T) {
    input := strings.Join([]string{
        "cfgbollocks = |###| ###v1###",
        "a = ###1###",
        "a = ###2###",
    }, "\n")

    got, err := Parse(input)
    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }

    if got[0].Key != "a" || got[1].Key != "a" {
        t.Fatalf("keys must preserve order and duplication")
    }
}

