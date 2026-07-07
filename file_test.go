package validator

import (
	"bytes"
	"context"
	"mime/multipart"
	"reflect"
	"testing"
)

// makeFileHeader builds a real *multipart.FileHeader carrying content.
func makeFileHeader(t *testing.T, filename string, content []byte) *multipart.FileHeader {
	t.Helper()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	fw, err := w.CreateFormFile("file", filename)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fw.Write(content); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	form, err := multipart.NewReader(&buf, w.Boundary()).ReadForm(1 << 20)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = form.RemoveAll() })
	return form.File["file"][0]
}

var pngHead = []byte("\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR")

func TestExtRule(t *testing.T) {
	fh := makeFileHeader(t, "photo.PNG", pngHead)
	cases := []struct {
		val   any
		attrs []string
		want  bool
	}{
		{fh, []string{"jpg", "png"}, true}, // case-insensitive
		{fh, []string{".png"}, true},       // leading dot ignored
		{fh, []string{"jpg", "gif"}, false},
		{"doc.pdf", []string{"pdf"}, true}, // plain string filename
		{"noext", []string{"pdf"}, false},
		{"", []string{"pdf"}, true}, // omitempty
		{nil, []string{"pdf"}, true},
		{42, []string{"pdf"}, false}, // non-file fails closed
	}
	r := &extRule{}
	for _, c := range cases {
		if got := r.Passes(fakeField{val: reflect.ValueOf(c.val), attrs: c.attrs}); got != c.want {
			t.Errorf("ext(%v, %v)=%v want %v", c.val, c.attrs, got, c.want)
		}
	}
}

func TestMimetypesSniffs(t *testing.T) {
	png := makeFileHeader(t, "a.png", pngHead)
	r := &mimetypesRule{}
	if !r.Passes(fakeField{val: reflect.ValueOf(png), attrs: []string{"image/png"}}) {
		t.Error("png content must match image/png")
	}
	if !r.Passes(fakeField{val: reflect.ValueOf(png), attrs: []string{"image/*"}}) {
		t.Error("png content must match the image/* wildcard")
	}
	if r.Passes(fakeField{val: reflect.ValueOf(png), attrs: []string{"application/pdf"}}) {
		t.Error("png content must not match application/pdf")
	}
	// content sniffing, not the filename: a "renamed" executable-ish payload
	fake := makeFileHeader(t, "totally-a.png", []byte("\x00\x01\x02\x03 binary junk"))
	if fake == nil || r.Passes(fakeField{val: reflect.ValueOf(fake), attrs: []string{"image/png"}}) {
		t.Error("non-png content named .png must fail mimetypes:image/png")
	}
}

func TestFileMinMax(t *testing.T) {
	fh := makeFileHeader(t, "a.bin", make([]byte, 2048)) // 2kb
	fmin, fmax := &fileminRule{}, &filemaxRule{}
	if !fmin.Passes(fakeField{val: reflect.ValueOf(fh), attrs: []string{"1kb"}}) {
		t.Error("2kb file must pass filemin:1kb")
	}
	if fmin.Passes(fakeField{val: reflect.ValueOf(fh), attrs: []string{"4kb"}}) {
		t.Error("2kb file must fail filemin:4kb")
	}
	if !fmax.Passes(fakeField{val: reflect.ValueOf(fh), attrs: []string{"1mb"}}) {
		t.Error("2kb file must pass filemax:1mb")
	}
	if fmax.Passes(fakeField{val: reflect.ValueOf(fh), attrs: []string{"1kb"}}) {
		t.Error("2kb file must fail filemax:1kb")
	}
	// bad size arg is a compile-time error via CheckArgs
	if err := fmax.CheckArgs([]string{"10potato"}); err == nil {
		t.Error("CheckArgs must reject a malformed size")
	}
	if err := fmax.CheckArgs([]string{"1.5mb"}); err != nil {
		t.Errorf("CheckArgs must accept 1.5mb: %v", err)
	}
}

func TestParseByteSize(t *testing.T) {
	cases := []struct {
		in   string
		want int64
		ok   bool
	}{
		{"1024", 1024, true},
		{"1kb", 1024, true},
		{"10MB", 10 << 20, true},
		{"1.5kb", 1536, true},
		{"2GB", 2 << 30, true},
		{"1tb", 1 << 40, true},
		{"512b", 512, true},
		{"", 0, false},
		{"-1kb", 0, false},
		{"kb", 0, false},
		{"1e3", 0, false}, // strict decimal, no exponent
	}
	for _, c := range cases {
		got, ok := parseByteSize(c.in)
		if ok != c.ok || (ok && got != c.want) {
			t.Errorf("parseByteSize(%q)=(%d,%v) want (%d,%v)", c.in, got, ok, c.want, c.ok)
		}
	}
}

func TestSniffMIME(t *testing.T) {
	cases := []struct {
		head []byte
		want string
	}{
		{pngHead, "image/png"},
		{[]byte("\xFF\xD8\xFF\xE0JFIF"), "image/jpeg"},
		{[]byte("GIF89a..."), "image/gif"},
		{[]byte("%PDF-1.7"), "application/pdf"},
		{[]byte("PK\x03\x04zipdata"), "application/zip"},
		{[]byte("plain text here"), "text/plain"},
		{[]byte("\x00\x01\x02"), "application/octet-stream"},
		{[]byte("RIFF\x00\x00\x00\x00WEBPVP8 "), "image/webp"},
		{[]byte("\x00\x00\x00\x18ftypmp42"), "video/mp4"},
		// markup-looking content must NOT pass as text/plain: classified by the
		// tag-open grammar ('<' + letter/'!'/'/'/'?'), not a tag denylist, so
		// unlisted/custom/future tags cannot slip through
		{[]byte("<script>alert(1)</script>"), "text/html"},
		{[]byte("<!DOCTYPE html><html>"), "text/html"},
		{[]byte("  \n<HTML><head>"), "text/html"},
		{[]byte("<iframe src=x>"), "text/html"},
		{[]byte("<!-- comment -->"), "text/html"},
		{[]byte("<?xml version=\"1.0\"?><svg/>"), "text/xml"},
		{[]byte("<?php echo 1 ?>"), "text/html"},
		{[]byte("<script\n>alert(1)</script>"), "text/html"},
		{[]byte("<script/xss>alert(1)</script>"), "text/html"},
		{[]byte("<svg onload=alert(1)>"), "text/html"},
		{[]byte("<video onerror=alert(1)><source>"), "text/html"},
		{[]byte("<x onclick=alert(1)>custom</x>"), "text/html"},
		{[]byte("</div>tail fragment"), "text/html"},
		{[]byte("<abc def"), "text/html"},                              // custom elements are tags
		{[]byte("\xEF\xBB\xBF<script>alert(1)</script>"), "text/html"}, // BOM stripped first
		// literal '<' in prose stays text/plain, exactly like the HTML parser
		{[]byte("a < b and c > d"), "text/plain"},
		{[]byte("<3 you"), "text/plain"},
		{[]byte("1<2"), "text/plain"},
		{[]byte("<> <- <_ <5"), "text/plain"},
	}
	for _, c := range cases {
		if got := sniffMIME(c.head); got != c.want {
			t.Errorf("sniffMIME(%q)=%q want %q", c.head, got, c.want)
		}
	}
}

// Active markup uploaded as .txt must fail mimetypes:text/plain (content
// sniffing, not extension) — including tags outside any signature list.
func TestMimetypesRejectsHTMLAsText(t *testing.T) {
	r := &mimetypesRule{}
	for _, payload := range []string{
		"<script>alert(1)</script>",
		"<svg onload=alert(1)>",
		"<details ontoggle=alert(1) open>",
	} {
		fh := makeFileHeader(t, "notes.txt", []byte(payload))
		if r.Passes(fakeField{val: reflect.ValueOf(fh), attrs: []string{"text/plain"}}) {
			t.Errorf("active markup %q must not pass mimetypes:text/plain", payload)
		}
	}
	plain := makeFileHeader(t, "notes.txt", []byte("just plain notes, 1 < 2"))
	if !r.Passes(fakeField{val: reflect.ValueOf(plain), attrs: []string{"text/plain"}}) {
		t.Error("actual plain text must pass mimetypes:text/plain")
	}
}

func TestFileRulesEndToEnd(t *testing.T) {
	type Upload struct {
		Avatar *multipart.FileHeader `validate:"required && ext:png,jpg && mimetypes:image/* && filemax:1mb"`
	}
	v := NewValidator()
	good := v.Struct(&Upload{Avatar: makeFileHeader(t, "a.png", pngHead)})
	good.Validate(context.Background())
	if good.Fails() {
		t.Fatalf("valid upload failed: %v", good.Errors().All())
	}
	missing := v.Struct(&Upload{})
	missing.Validate(context.Background())
	if !missing.Fails() {
		t.Fatal("nil *FileHeader must fail required")
	}
	badExt := v.Struct(&Upload{Avatar: makeFileHeader(t, "a.exe", pngHead)})
	badExt.Validate(context.Background())
	if !badExt.Fails() {
		t.Fatal(".exe must fail ext:png,jpg")
	}
}
