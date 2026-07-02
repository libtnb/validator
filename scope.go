package validator

import (
	"reflect"
	"strings"
)

func (vd *validation) srcLookup(name string) (reflect.Value, bool) {
	if vd.ssPlan != nil {
		return structSource{val: vd.ssVal, plan: vd.ssPlan}.lookup(name)
	}
	if vd.isVar {
		// only its own name resolves: cross-field refs miss rather than self-compare
		if name == varFieldName {
			return unwrapValue(vd.ssVal), true
		}
		return reflect.Value{}, false
	}
	if vd.src == nil {
		return reflect.Value{}, false
	}
	return vd.src.lookup(name)
}

func (vd *validation) rawData() any {
	if vd.ssPlan != nil {
		return vd.ssVal.Interface()
	}
	if vd.isVar {
		return valToAny(vd.ssVal)
	}
	if vd.src != nil {
		return vd.src.raw()
	}
	return nil
}

// resolveScoped resolves a cross-field name relative-first, then absolute.
func (vd *validation) resolveScoped(scope, name string) (reflect.Value, bool) {
	// only nested sources (inline struct or map) get scoping.
	scoped := vd.ssPlan != nil
	if !scoped {
		if _, ok := vd.src.(mapSource); ok {
			scoped = true
		}
	}
	if scoped {
		// relative prefix only for a bare name; dotted names are already absolute.
		if dot := strings.LastIndex(scope, "."); dot >= 0 && !strings.Contains(name, ".") {
			if s := scope[:dot+1] + name; s != scope { // never self (blocks flat dotted-key self-compare)
				if v, ok := vd.srcLookup(s); ok {
					return v, true
				}
			}
		}
	}
	return vd.srcLookup(name)
}
