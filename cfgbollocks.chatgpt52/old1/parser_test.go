package cfgbollocks_test

import (
	"reflect"
	"testing"

	. "cfgbollocks"
)

func TestParse(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    []Entry
		wantErr bool
	}{
		{
			name: "single key with value, \n is included",
			input: `
key=###
value
###
`,
			want: []Entry{
				{Key: "key", Value: "value\n"},
			},
		},

		{
			name: "value preserves internal newlines",
			input: `
key=###
line1
line2
line3
###
`,
			want: []Entry{
				{Key: "key", Value: "line1\nline2\nline3\n"},
			},
		},

		{
			name: "empty value must use delimiters",
			input: `
key=###
###
`,
			want: []Entry{
				{Key: "key", Value: ""},
			},
		},
		{
			name: "new line only",
			input: `
key=###

###
`,
			want: []Entry{
				{Key: "key", Value: "\n"},
			},
		},
		{
			name: "new line with a space before it",
			input: `
key=###
 
###
`,
			want: []Entry{
				{Key: "key", Value: " \n"},
			},
		},
		{
			name: "2 new lines value",
			input: `
key=###


###
`,
			want: []Entry{
				{Key: "key", Value: "\n\n"},
			},
		},
		{
			name: "value plus 2 new lines",
			input: `
key=###
something

###
`,
			want: []Entry{
				{Key: "key", Value: "something\n\n"},
			},
		},
		{
			name: "multiple entries preserve order",
			input: `
a=###
one
###
b=###
two
###
c=###
three
###
`,
			want: []Entry{
				{Key: "a", Value: "one\n"},
				{Key: "b", Value: "two\n"},
				{Key: "c", Value: "three\n"},
			},
		},

		{
			name: "same key may appear multiple times",
			input: `
k=###
first
###
k=###
second
###
`,
			want: []Entry{
				{Key: "k", Value: "first\n"},
				{Key: "k", Value: "second\n"},
			},
		},

		{
			name: "delimiter may have surrounding whitespace on key line",
			input: `
key =   ###   
value
###
`,
			want: []Entry{
				{Key: "key", Value: "value\n"},
			},
		},

		{
			name: "closing delimiter allows trailing whitespace only",
			input: `
key=###
value
###     
`,
			want: []Entry{
				{Key: "key", Value: "value\n"},
			},
		},

		{
			name: "leading whitespace before closing delimiter is NOT allowed",
			input: `
key=###
value
  ###
`,
			wantErr: true,
		},

		{
			name: "delimiter string inside value is allowed if not exact line",
			input: `
key=###
### not a delimiter
###
`,
			want: []Entry{
				{Key: "key", Value: "### not a delimiter\n"},
			},
		},

		{
			name: "delimiter-like line with leading whitespace is value",
			input: `
key=###
 ### 
###
`,
			want: []Entry{
				{Key: "key", Value: " ### \n"},
			},
		},

		{
			name: "unterminated value is an error",
			input: `
key=###
value
`,
			wantErr: true,
		},

		{
			name: "key without delimiter is invalid",
			input: `
key=
`,
			wantErr: true,
		},

		{
			name: "key with no delimiter token is invalid",
			input: `
key=value
`,
			wantErr: true,
		},

		{
			name: "garbage outside entries is rejected",
			input: `
this is nonsense
`,
			wantErr: true,
		},

		{
			name:  "empty file is valid",
			input: ``,
			want:  nil,
		},

		{
			name: "whitespace-only file is valid",
			input: `
            
            
`,
			want: nil,
		},

		{
			name: "adjacent entries without blank lines",
			input: `
a=###
1
###
b=###
2
###
`,
			want: []Entry{
				{Key: "a", Value: "1\n"},
				{Key: "b", Value: "2\n"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Parse(tt.input)

			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("mismatch\nwant: %#v\ngot:  %#v", tt.want, got)
			}
		})
	}
}
