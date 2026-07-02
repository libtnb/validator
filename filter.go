package validator

import (
	"errors"
	"fmt"
	"reflect"
	"strings"

	"github.com/libtnb/validator/conv"
)

type filterSpec struct {
	sig  string
	args []string
}

// AddFilters attaches filter chains to a field; an unknown filter errors, leaving the field unchanged.
func (vd *validation) AddFilters(field string, filters ...string) error {
	candidate := vd.filters[field]
	for _, f := range filters {
		if f == "" {
			continue
		}
		candidate = joinFilter(candidate, f)
	}
	if candidate == vd.filters[field] {
		return nil
	}
	for _, spec := range parseFilterChain(candidate) {
		if _, ok := vd.validator.registry.filter(spec.sig); !ok {
			return fmt.Errorf("validator: unknown filter %q", spec.sig)
		}
	}
	if vd.filters == nil {
		vd.filters = make(map[string]string)
	}
	vd.filters[field] = candidate
	return nil
}

// RemoveFilters removes named filters from a field's chain.
func (vd *validation) RemoveFilters(field string, filters ...string) error {
	if len(filters) == 0 {
		return nil
	}
	cur, ok := vd.filters[field]
	if !ok {
		return nil
	}
	drop := make(map[string]bool, len(filters))
	for _, f := range filters {
		drop[strings.TrimSpace(f)] = true
	}
	if nw := removeFilterSegments(cur, drop); nw == "" {
		delete(vd.filters, field)
	} else {
		vd.filters[field] = nw
	}
	return nil
}

// ClearFilters drops a field's entire filter chain.
func (vd *validation) ClearFilters(field string) error {
	delete(vd.filters, field)
	return nil
}

func (vd *validation) Filters() map[string]string { return copyStrMap(vd.filters) }

// prepareFilters computes every filtered value once, serially before any concurrent access (race-free).
func (vd *validation) prepareFilters() {
	if vd.filtersPrepared {
		return
	}
	vd.filtersPrepared = true
	if len(vd.filters) == 0 {
		return
	}
	vd.filtered = make(map[string]any, len(vd.filters))
	for name, chain := range vd.filters {
		if chain == "" {
			continue
		}
		// Skip diving fields: a scalar filter would stringify the collection.
		if ds := vd.validator.splitDive(vd.rules[name]); ds.err == nil && ds.hasDive {
			continue
		}
		if raw, ok := vd.srcLookup(name); ok {
			out, err := vd.applyFilters(chain, valToAny(raw))
			if err != nil {
				// a failed filter is a validation failure, never a silent raw value
				if vd.filterErrs == nil {
					vd.filterErrs = make(map[string]string)
				}
				vd.filterErrs[name] = err.Error()
				continue
			}
			vd.filtered[name] = unwrap(out)
		}
	}
}

func (vd *validation) fieldValue(name string) (reflect.Value, bool) {
	if v, ok := vd.filtered[name]; ok {
		return toValue(v), true
	}
	return vd.srcLookup(name)
}

// applyFilters folds a value through a '|'-chain; the first failing (or
// panicking) filter stops the chain and surfaces as a field error.
func (vd *validation) applyFilters(chain string, val any) (any, error) {
	for _, spec := range parseFilterChain(chain) {
		f, ok := vd.validator.registry.filter(spec.sig)
		if !ok {
			// unreachable via AddFilters (validated); defend against races
			return nil, fmt.Errorf("validator: unknown filter %q", spec.sig)
		}
		out, err := safeHandle(f, val, spec.args)
		if err != nil {
			return nil, fmt.Errorf("validator: filter %q failed: %v", spec.sig, err)
		}
		val = out
	}
	return val, nil
}

// joinFilter joins with "|", escaping a trailing odd backslash run so it can't swallow the separator.
func joinFilter(existing, add string) string {
	if existing == "" {
		return add
	}
	n := 0
	for i := len(existing) - 1; i >= 0 && existing[i] == '\\'; i-- {
		n++
	}
	if n%2 == 1 {
		existing += "\\"
	}
	return existing + "|" + add
}

func removeFilterSegments(chain string, drop map[string]bool) string {
	result := ""
	start := 0
	flush := func(end int) {
		seg := strings.TrimSpace(chain[start:end])
		if seg == "" {
			return
		}
		if drop[filterName(seg)] || drop[seg] {
			return
		}
		result = joinFilter(result, seg)
	}
	for i := 0; i < len(chain); i++ {
		if chain[i] == '\\' && i+1 < len(chain) {
			i++
			continue
		}
		if chain[i] == '|' {
			flush(i)
			start = i + 1
		}
	}
	flush(len(chain))
	return result
}

func filterName(seg string) string {
	for i := 0; i < len(seg); i++ {
		if seg[i] == '\\' && i+1 < len(seg) {
			i++
			continue
		}
		if seg[i] == ':' {
			return strings.TrimSpace(seg[:i])
		}
	}
	return strings.TrimSpace(seg)
}

// parseFilterChain splits an unescaped '|'/':'/',' chain into filter specs; '\' escapes a separator.
func parseFilterChain(chain string) []filterSpec {
	var specs []filterSpec
	var name, arg strings.Builder
	var args []string
	inArgs := false

	write := func(b byte) {
		if inArgs {
			arg.WriteByte(b)
		} else {
			name.WriteByte(b)
		}
	}
	flush := func() {
		n := strings.TrimSpace(name.String())
		if inArgs {
			args = append(args, strings.TrimSpace(arg.String()))
		}
		if n != "" {
			specs = append(specs, filterSpec{sig: n, args: args})
		}
		name.Reset()
		arg.Reset()
		args = nil
		inArgs = false
	}

	for i := 0; i < len(chain); i++ {
		c := chain[i]
		if c == '\\' && i+1 < len(chain) {
			switch n := chain[i+1]; n {
			case '|', ',', ':', '\\':
				write(n)
				i++
			default:
				write(c)
			}
			continue
		}
		switch c {
		case '|':
			flush()
		case ':':
			if inArgs {
				arg.WriteByte(c)
			} else {
				inArgs = true
			}
		case ',':
			if inArgs {
				args = append(args, strings.TrimSpace(arg.String()))
				arg.Reset()
			} else {
				name.WriteByte(c)
			}
		default:
			write(c)
		}
	}
	flush()
	return specs
}

// safeHandle invokes a filter, recovering a panic into an error so it can't crash the process.
func safeHandle(f Filter, val any, args []string) (out any, err error) {
	defer func() {
		if r := recover(); r != nil {
			// conv.ToString, not %v: a panic value whose own String() panics
			// must not re-escape the recovery
			err = errors.New("validator: filter panicked: " + conv.ToString(r))
		}
	}()
	return f.Handle(val, args...)
}
