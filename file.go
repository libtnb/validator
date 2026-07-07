package validator

import (
	"bytes"
	"errors"
	"io"
	"mime/multipart"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
)

func init() {
	registerRules(
		&extRule{},
		&mimetypesRule{},
		&fileminRule{},
		&filemaxRule{},
	)
}

var (
	_ Rule = (*extRule)(nil)
	_ Rule = (*mimetypesRule)(nil)
	_ Rule = (*fileminRule)(nil)
	_ Rule = (*filemaxRule)(nil)

	_ argChecker = (*fileminRule)(nil)
	_ argChecker = (*filemaxRule)(nil)
)

// extRule: the filename (a *multipart.FileHeader's or a string) has one of the
// listed extensions (ext:jpg,png; leading dots and case are ignored).
type extRule struct{}

func (r *extRule) Signature() string { return "ext" }

func (r *extRule) Passes(f Field) bool {
	rv := f.Val()
	if isEmptyV(rv) {
		return true
	}
	var name string
	if fh, ok := fileHeaderValue(rv); ok {
		name = fh.Filename
	} else if rv.Kind() == reflect.String {
		name = rv.String()
	} else {
		return false
	}
	e := strings.TrimPrefix(filepath.Ext(name), ".")
	if e == "" {
		return false
	}
	return slices.ContainsFunc(f.Attrs(), func(a string) bool {
		return strings.EqualFold(strings.TrimPrefix(a, "."), e)
	})
}

func (r *extRule) Message() string { return "The {field} must be a file of type: {0+}." }

// mimetypesRule: an uploaded file's SNIFFED content type (first 512 bytes, not
// the client-declared header) matches one of the args; "image/*" wildcards work.
type mimetypesRule struct{}

func (r *mimetypesRule) Signature() string { return "mimetypes" }

func (r *mimetypesRule) Passes(f Field) bool {
	rv := f.Val()
	if isEmptyV(rv) {
		return true
	}
	fh, ok := fileHeaderValue(rv)
	if !ok {
		return false
	}
	mt, ok := sniffFile(fh)
	if !ok {
		return false
	}
	return slices.ContainsFunc(f.Attrs(), func(a string) bool { return mimeMatch(a, mt) })
}

func (r *mimetypesRule) Message() string { return "The {field} must be a file of type: {0+}." }

// fileminRule: an uploaded file's size is at least the arg (filemin:1kb).
type fileminRule struct{}

func (r *fileminRule) Signature() string { return "filemin" }

func (r *fileminRule) Passes(f Field) bool {
	return fileSizeCompare(f, func(size, n int64) bool { return size >= n })
}

func (r *fileminRule) CheckArgs(args []string) error { return checkByteSizeArg(args) }

func (r *fileminRule) Message() string { return "The {field} file must be at least {0}." }

// filemaxRule: an uploaded file's size is at most the arg (filemax:10mb).
type filemaxRule struct{}

func (r *filemaxRule) Signature() string { return "filemax" }

func (r *filemaxRule) Passes(f Field) bool {
	return fileSizeCompare(f, func(size, n int64) bool { return size <= n })
}

func (r *filemaxRule) CheckArgs(args []string) error { return checkByteSizeArg(args) }

func (r *filemaxRule) Message() string { return "The {field} file must not be greater than {0}." }

// fileHeaderValue extracts a *multipart.FileHeader (nil pointer fails).
func fileHeaderValue(rv reflect.Value) (*multipart.FileHeader, bool) {
	if !rv.IsValid() || !rv.CanInterface() {
		return nil, false
	}
	switch fh := rv.Interface().(type) {
	case multipart.FileHeader:
		return &fh, true
	case *multipart.FileHeader:
		if fh != nil {
			return fh, true
		}
	}
	return nil, false
}

// fileSizeCompare: empty passes (omitempty); a non-file value or bad arg fails closed.
func fileSizeCompare(f Field, ok func(size, n int64) bool) bool {
	rv := f.Val()
	if isEmptyV(rv) {
		return true
	}
	fh, valid := fileHeaderValue(rv)
	attrs := f.Attrs()
	if !valid || len(attrs) == 0 {
		return false
	}
	n, valid := parseByteSize(attrs[0])
	return valid && ok(fh.Size, n)
}

// parseByteSize parses "10485760", "512kb", "10MB", "1.5gb" (case-insensitive,
// 1024-based b/kb/mb/gb/tb suffixes).
func parseByteSize(s string) (int64, bool) {
	s = strings.ToLower(strings.TrimSpace(s))
	mult := int64(1)
	switch {
	case strings.HasSuffix(s, "tb"):
		mult, s = 1<<40, s[:len(s)-2]
	case strings.HasSuffix(s, "gb"):
		mult, s = 1<<30, s[:len(s)-2]
	case strings.HasSuffix(s, "mb"):
		mult, s = 1<<20, s[:len(s)-2]
	case strings.HasSuffix(s, "kb"):
		mult, s = 1<<10, s[:len(s)-2]
	case strings.HasSuffix(s, "b"):
		s = s[:len(s)-1]
	}
	v, ok := parseNumericString(strings.TrimSpace(s))
	if !ok || v < 0 || v > float64(1<<62)/float64(mult) {
		return 0, false
	}
	return int64(v * float64(mult)), true
}

func checkByteSizeArg(args []string) error {
	if len(args) == 0 {
		return errors.New("a size argument is required (e.g. 512kb, 10mb)")
	}
	if _, ok := parseByteSize(args[0]); !ok {
		return errors.New("invalid size argument " + strings.TrimSpace(args[0]))
	}
	return nil
}

// mimeMatch reports pattern matching a sniffed media type; "image/*" matches the family.
func mimeMatch(pattern, mt string) bool {
	if fam, ok := strings.CutSuffix(pattern, "/*"); ok {
		return len(mt) > len(fam)+1 && strings.EqualFold(mt[:len(fam)], fam) && mt[len(fam)] == '/'
	}
	return strings.EqualFold(pattern, mt)
}

// sniffFile reads the file's first 512 bytes and sniffs the content type.
func sniffFile(fh *multipart.FileHeader) (string, bool) {
	fr, err := fh.Open()
	if err != nil {
		return "", false
	}
	defer fr.Close()
	var head [512]byte
	n, err := io.ReadFull(fr, head[:])
	if err != nil && err != io.ErrUnexpectedEOF && err != io.EOF {
		return "", false
	}
	return sniffMIME(head[:n]), true
}

// sniffMIME maps magic bytes to a media type (a focused subset of the WHATWG
// sniffing table net/http uses, kept local so the package never links net/http).
// Unknown binary data is application/octet-stream; control-character-free data
// is text/plain.
func sniffMIME(head []byte) string {
	switch {
	case bytes.HasPrefix(head, []byte("\xFF\xD8\xFF")):
		return "image/jpeg"
	case bytes.HasPrefix(head, []byte("\x89PNG\r\n\x1a\n")):
		return "image/png"
	case bytes.HasPrefix(head, []byte("GIF87a")), bytes.HasPrefix(head, []byte("GIF89a")):
		return "image/gif"
	case riffType(head, "WEBP"):
		return "image/webp"
	case riffType(head, "WAVE"):
		return "audio/wave"
	case riffType(head, "AVI "):
		return "video/avi"
	case bytes.HasPrefix(head, []byte("BM")):
		return "image/bmp"
	case bytes.HasPrefix(head, []byte("\x00\x00\x01\x00")):
		return "image/x-icon"
	case bytes.HasPrefix(head, []byte("%PDF-")):
		return "application/pdf"
	case bytes.HasPrefix(head, []byte("PK\x03\x04")):
		return "application/zip"
	case bytes.HasPrefix(head, []byte("\x1F\x8B\x08")):
		return "application/x-gzip"
	case bytes.HasPrefix(head, []byte("OggS")):
		return "application/ogg"
	case bytes.HasPrefix(head, []byte("fLaC")):
		return "audio/flac"
	case bytes.HasPrefix(head, []byte("ID3")):
		return "audio/mpeg"
	case bytes.HasPrefix(head, []byte("MThd")):
		return "audio/midi"
	case bytes.HasPrefix(head, []byte("\x1A\x45\xDF\xA3")):
		return "video/webm"
	case len(head) >= 12 && string(head[:4]) == "FORM" && string(head[8:12]) == "AIFF":
		return "audio/aiff"
	case len(head) >= 8 && string(head[4:8]) == "ftyp":
		return "video/mp4"
	case len(head) >= 2 && head[0] == 0xFF && (head[1] == 0xFB || head[1] == 0xF3 || head[1] == 0xF2):
		return "audio/mpeg"
	case looksText(head):
		return sniffText(head)
	default:
		return "application/octet-stream"
	}
}

func riffType(head []byte, sub string) bool {
	return len(head) >= 12 && string(head[:4]) == "RIFF" && string(head[8:12]) == sub
}

// looksText reports no WHATWG "binary data byte" in head; empty is not text.
func looksText(head []byte) bool {
	for _, c := range head {
		if c <= 0x08 || c == 0x0B || (c >= 0x0E && c <= 0x1A) || (c >= 0x1C && c <= 0x1F) {
			return false
		}
	}
	return len(head) > 0
}

// sniffText classifies control-character-free data. Anything whose first
// non-whitespace bytes open markup — '<' followed by an ASCII letter, '!', '/'
// or '?' (the HTML tokenizer's complete tag-open grammar) — is NEVER
// text/plain: active tags are an open set (<svg onload>, custom elements), so
// no fixed signature list can enumerate them; classifying by grammar is the
// only closed answer. Deliberately stricter than WHATWG/net/http, which match
// ~17 known tags and would let unlisted markup pass a text/plain allowlist.
// Literal uses like "a < b" or "<3" stay text/plain, exactly as the HTML
// parser treats them as text.
func sniffText(head []byte) string {
	t := head
	// strip a UTF-8 BOM: browsers do before parsing, so markup behind it stays markup
	if len(t) >= 3 && t[0] == 0xEF && t[1] == 0xBB && t[2] == 0xBF {
		t = t[3:]
	}
	for len(t) > 0 && (t[0] == '\t' || t[0] == '\n' || t[0] == '\x0c' || t[0] == '\r' || t[0] == ' ') {
		t = t[1:]
	}
	if len(t) >= 2 && t[0] == '<' {
		switch c := t[1]; {
		case c == '?':
			if len(t) >= 5 && string(t[:5]) == "<?xml" {
				return "text/xml"
			}
			return "text/html" // processing instruction / bogus comment
		case c == '!' || c == '/' || ('a' <= c && c <= 'z') || ('A' <= c && c <= 'Z'):
			return "text/html"
		}
	}
	return "text/plain"
}
