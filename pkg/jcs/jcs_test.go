package jcs

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

// esc turns %uXXXX into the JSON escape \uXXXX. Spelling the escapes this way
// keeps editors and tooling from replacing them with literal characters.
func esc(s string) string {
	return strings.ReplaceAll(s, "%u", `\`+"u")
}

func TestCanonicalize(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			// String and literal part of the RFC 8785 section 3.2.3 example.
			name: "rfc8785 strings and literals",
			in:   esc(`{"string": "%u20ac$%u000F%u000aA'%u0042%u0022%u005c\\\"\/", "literals": [null, true, false]}`),
			want: fmt.Sprintf(esc(`{"literals":[null,true,false],"string":"%c$%u000f\nA'B\"\\\\\"/"}`), 0x20ac),
		},
		{
			// RFC 8785 section 3.2.3 sorting example: UTF-16 order puts the emoji
			// (D83D) before U+FB33, unlike UTF-8 byte order.
			name: "rfc8785 key order",
			in: esc(`{"%u20ac":"Euro Sign","\r":"Carriage Return","%ufb33":"Hebrew Letter Dalet With Dagesh",` +
				`"1":"One","%ud83d%ude00":"Emoji: Grinning Face","%u0080":"Control","%u00f6":"Latin Small Letter O With Diaeresis"}`),
			want: fmt.Sprintf(`{"\r":"Carriage Return","1":"One","%c":"Control","%c":"Latin Small Letter O With Diaeresis",`+
				`"%c":"Euro Sign","%c":"Emoji: Grinning Face","%c":"Hebrew Letter Dalet With Dagesh"}`,
				0x80, 0xf6, 0x20ac, 0x1f600, 0xfb33),
		},
		{
			name: "nested objects and whitespace",
			in:   " { \"b\" : [ 1 , { \"d\":2, \"c\":-3 } ], \"a\" : {} } ",
			want: `{"a":{},"b":[1,{"c":-3,"d":2}]}`,
		},
		{
			name: "integer edge cases",
			in:   `[0, -0, 9007199254740991, -9007199254740991]`,
			want: `[0,0,9007199254740991,-9007199254740991]`,
		},
		{
			name: "html characters and line separator are not escaped",
			in:   esc(`{"x":"<a href='/'>&</a>%u2028"}`),
			want: fmt.Sprintf(`{"x":"<a href='/'>&</a>%c"}`, 0x2028),
		},
		{
			name: "empty containers and scalars",
			in:   `{"o":{},"a":[],"s":""}`,
			want: `{"a":[],"o":{},"s":""}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Canonicalize([]byte(tt.in))
			if err != nil {
				t.Fatalf("Canonicalize: %v", err)
			}
			if string(got) != tt.want {
				t.Errorf("got  %s\nwant %s", got, tt.want)
			}
		})
	}
}

func TestCanonicalizeRejects(t *testing.T) {
	tests := []struct {
		name string
		in   []byte
		want error
	}{
		{"fraction", []byte(`{"a":1.5}`), ErrUnsupportedNumber},
		{"exponent", []byte(`{"a":1e3}`), ErrUnsupportedNumber},
		{"integer with fraction syntax", []byte(`{"a":2.0}`), ErrUnsupportedNumber},
		{"beyond 2^53", []byte(`[9007199254740992]`), ErrUnsupportedNumber},
		{"beyond -2^53", []byte(`[-9007199254740992]`), ErrUnsupportedNumber},
		{"duplicate key", []byte(`{"a":1,"a":2}`), ErrDuplicateKey},
		{"nested duplicate key", []byte(`{"x":{"a":1,"a":1}}`), ErrDuplicateKey},
		{"invalid utf8", []byte{'"', 0xff, '"'}, ErrInvalidUTF8},
		{"trailing data", []byte(`{} {}`), ErrTrailingData},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Canonicalize(tt.in)
			if !errors.Is(err, tt.want) {
				t.Fatalf("got error %v, want %v", err, tt.want)
			}
		})
	}
}

func TestCanonicalizeRejectsMalformed(t *testing.T) {
	for _, in := range []string{``, `{`, `{"a"}`, `[1,]`, `{"a":01}`, `nul`} {
		if _, err := Canonicalize([]byte(in)); err == nil {
			t.Errorf("Canonicalize(%q) succeeded, want error", in)
		}
	}
}

func TestCanonicalizeWithout(t *testing.T) {
	got, err := CanonicalizeWithout([]byte(`{"sig":"abc","b":2,"a":{"sig":"inner stays"}}`), "sig")
	if err != nil {
		t.Fatal(err)
	}
	if want := `{"a":{"sig":"inner stays"},"b":2}`; string(got) != want {
		t.Errorf("got %s, want %s", got, want)
	}

	if _, err := CanonicalizeWithout([]byte(`[1]`), "sig"); !errors.Is(err, ErrNotObject) {
		t.Errorf("array input: got %v, want ErrNotObject", err)
	}
}

func TestCanonicalizeIsIdempotent(t *testing.T) {
	first, err := Canonicalize([]byte(esc(`{"z":[{"y":"%u00e9\t"},null],"a":true}`)))
	if err != nil {
		t.Fatal(err)
	}
	second, err := Canonicalize(first)
	if err != nil {
		t.Fatal(err)
	}
	if string(first) != string(second) {
		t.Errorf("not idempotent:\n%s\n%s", first, second)
	}
}
