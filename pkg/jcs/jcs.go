// Package jcs implements the JSON Canonicalization Scheme (RFC 8785) for the
// subset of JSON that SmartCluster signs.
//
// Restriction: numbers must be integers with an absolute value of at most
// 2^53-1. For those, RFC 8785 number serialization is plain decimal digits, so
// no ECMAScript float formatting is needed. Documents containing any other
// number are rejected instead of being canonicalized in a way other
// implementations might disagree with.
package jcs

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"slices"
	"strconv"
	"unicode/utf16"
	"unicode/utf8"
)

const maxSafeInteger = 1<<53 - 1

var (
	ErrInvalidUTF8       = errors.New("jcs: document is not valid UTF-8")
	ErrDuplicateKey      = errors.New("jcs: duplicate object key")
	ErrUnsupportedNumber = errors.New("jcs: only integers with |n| <= 2^53-1 are supported")
	ErrNotObject         = errors.New("jcs: document is not a JSON object")
	ErrTrailingData      = errors.New("jcs: trailing data after document")
)

type object map[string]any

// Canonicalize returns the canonical form of the JSON document doc.
func Canonicalize(doc []byte) ([]byte, error) {
	v, err := parse(doc)
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	write(&buf, v)
	return buf.Bytes(), nil
}

// CanonicalizeWithout returns the canonical form of the top-level object doc
// with the member name removed. It yields the signed bytes of a message that
// carries its own signature in that member.
func CanonicalizeWithout(doc []byte, name string) ([]byte, error) {
	v, err := parse(doc)
	if err != nil {
		return nil, err
	}
	obj, ok := v.(object)
	if !ok {
		return nil, ErrNotObject
	}
	delete(obj, name)
	var buf bytes.Buffer
	write(&buf, obj)
	return buf.Bytes(), nil
}

func parse(doc []byte) (any, error) {
	if !utf8.Valid(doc) {
		return nil, ErrInvalidUTF8
	}
	dec := json.NewDecoder(bytes.NewReader(doc))
	dec.UseNumber()
	v, err := parseValue(dec)
	if err != nil {
		return nil, err
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return nil, ErrTrailingData
	}
	return v, nil
}

func parseValue(dec *json.Decoder) (any, error) {
	tok, err := dec.Token()
	if err != nil {
		return nil, fmt.Errorf("jcs: %w", err)
	}
	switch t := tok.(type) {
	case json.Delim:
		switch t {
		case '{':
			return parseObject(dec)
		case '[':
			return parseArray(dec)
		default:
			return nil, fmt.Errorf("jcs: unexpected delimiter %q", rune(t))
		}
	case json.Number:
		return parseInteger(string(t))
	case string, bool, nil:
		return t, nil
	default:
		return nil, fmt.Errorf("jcs: unexpected token %v", tok)
	}
}

func parseObject(dec *json.Decoder) (any, error) {
	obj := object{}
	for dec.More() {
		keyTok, err := dec.Token()
		if err != nil {
			return nil, fmt.Errorf("jcs: %w", err)
		}
		key, ok := keyTok.(string)
		if !ok {
			return nil, fmt.Errorf("jcs: object key is %T, not a string", keyTok)
		}
		if _, dup := obj[key]; dup {
			return nil, fmt.Errorf("%w: %q", ErrDuplicateKey, key)
		}
		val, err := parseValue(dec)
		if err != nil {
			return nil, err
		}
		obj[key] = val
	}
	if _, err := dec.Token(); err != nil { // closing '}'
		return nil, fmt.Errorf("jcs: %w", err)
	}
	return obj, nil
}

func parseArray(dec *json.Decoder) (any, error) {
	arr := []any{}
	for dec.More() {
		val, err := parseValue(dec)
		if err != nil {
			return nil, err
		}
		arr = append(arr, val)
	}
	if _, err := dec.Token(); err != nil { // closing ']'
		return nil, fmt.Errorf("jcs: %w", err)
	}
	return arr, nil
}

func parseInteger(lit string) (int64, error) {
	digits := lit
	if len(digits) > 0 && digits[0] == '-' {
		digits = digits[1:]
	}
	if digits == "" {
		return 0, fmt.Errorf("%w: %s", ErrUnsupportedNumber, lit)
	}
	for _, c := range digits {
		if c < '0' || c > '9' {
			return 0, fmt.Errorf("%w: %s", ErrUnsupportedNumber, lit)
		}
	}
	n, err := strconv.ParseInt(lit, 10, 64)
	if err != nil || n > maxSafeInteger || n < -maxSafeInteger {
		return 0, fmt.Errorf("%w: %s", ErrUnsupportedNumber, lit)
	}
	return n, nil
}

func write(buf *bytes.Buffer, v any) {
	switch t := v.(type) {
	case nil:
		buf.WriteString("null")
	case bool:
		buf.WriteString(strconv.FormatBool(t))
	case int64:
		buf.WriteString(strconv.FormatInt(t, 10))
	case string:
		writeString(buf, t)
	case []any:
		buf.WriteByte('[')
		for i, e := range t {
			if i > 0 {
				buf.WriteByte(',')
			}
			write(buf, e)
		}
		buf.WriteByte(']')
	case object:
		keys := make([]string, 0, len(t))
		for k := range t {
			keys = append(keys, k)
		}
		slices.SortFunc(keys, compareUTF16)
		buf.WriteByte('{')
		for i, k := range keys {
			if i > 0 {
				buf.WriteByte(',')
			}
			writeString(buf, k)
			buf.WriteByte(':')
			write(buf, t[k])
		}
		buf.WriteByte('}')
	}
}

// compareUTF16 orders strings by their UTF-16 code units, as RFC 8785 requires.
// This differs from Go's byte order for characters outside the BMP.
func compareUTF16(a, b string) int {
	ua, ub := utf16.Encode([]rune(a)), utf16.Encode([]rune(b))
	return slices.Compare(ua, ub)
}

func writeString(buf *bytes.Buffer, s string) {
	buf.WriteByte('"')
	for _, r := range s {
		switch r {
		case '"':
			buf.WriteString(`\"`)
		case '\\':
			buf.WriteString(`\\`)
		case '\b':
			buf.WriteString(`\b`)
		case '\f':
			buf.WriteString(`\f`)
		case '\n':
			buf.WriteString(`\n`)
		case '\r':
			buf.WriteString(`\r`)
		case '\t':
			buf.WriteString(`\t`)
		default:
			if r < 0x20 {
				fmt.Fprintf(buf, `\u%04x`, r)
			} else {
				buf.WriteRune(r)
			}
		}
	}
	buf.WriteByte('"')
}
