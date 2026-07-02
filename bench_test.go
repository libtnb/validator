package validator

import (
	"context"
	"fmt"
	"testing"
)

type benchUser struct {
	Name  string `validate:"required && alpha"`
	Email string `validate:"required && email"`
	Age   int    `validate:"required && gte:18 && lte:120"`
}

func BenchmarkStructSuccess(b *testing.B) {
	v := NewValidator()
	u := benchUser{Name: "alice", Email: "a@b.com", Age: 30}
	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		vd := v.Struct(u)
		vd.Validate(ctx)
		if vd.Fails() {
			b.Fatal("should pass")
		}
	}
}

func BenchmarkStructFailure(b *testing.B) {
	v := NewValidator()
	u := benchUser{Name: "1", Email: "nope", Age: 5}
	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		vd := v.Struct(u)
		vd.Validate(ctx)
		if !vd.Fails() {
			b.Fatal("should fail")
		}
	}
}

func BenchmarkMapSuccess(b *testing.B) {
	v := NewValidator()
	data := map[string]any{"Name": "alice", "Email": "a@b.com", "Age": 30}
	sigs := map[string]string{"Name": "required && alpha", "Email": "required && email", "Age": "required && gte:18"}
	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		vd := v.Map(data, sigs)
		vd.Validate(ctx)
		if vd.Fails() {
			b.Fatal("should pass")
		}
	}
}

func BenchmarkVarSuccess(b *testing.B) {
	v := NewValidator()
	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		vd := v.Var("a@b.com", "required && email")
		vd.Validate(ctx)
		if vd.Fails() {
			b.Fatal("should pass")
		}
	}
}

func BenchmarkDiving(b *testing.B) {
	v := NewValidator()
	data := map[string]any{"tags": []any{"foo", "bar", "baz"}}
	sigs := map[string]string{"tags": "required && dive && alpha"}
	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		vd := v.Map(data, sigs)
		vd.Validate(ctx)
		if vd.Fails() {
			b.Fatal("should pass")
		}
	}
}

func BenchmarkCompileCold(b *testing.B) {
	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		v := NewValidator()
		vd := v.Var("a@b.com", "required && (email || regex:\"^x\")")
		vd.Validate(ctx)
	}
}

func BenchmarkCompileCached(b *testing.B) {
	v := NewValidator()
	// warm exprCache so the timed loop measures cache hits
	_, _ = v.compile("required && (email || regex:\"^x\")")
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = v.compile("required && (email || regex:\"^x\")")
	}
}

func BenchmarkParallelStruct(b *testing.B) {
	v := NewValidator(WithParallel(8))
	sigs := map[string]string{}
	data := map[string]any{}
	for i := 0; i < 20; i++ {
		f := fmt.Sprintf("f%02d", i)
		sigs[f] = "required && alpha"
		data[f] = "abc"
	}
	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		vd := v.Map(data, sigs)
		vd.Validate(ctx)
		if vd.Fails() {
			b.Fatal("should pass")
		}
	}
}
