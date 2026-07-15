package errors

import (
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"strconv"
	"strings"
	"testing"

	qt "github.com/frankban/quicktest"
)

// TestErrorCodesAreUnique parses the current package's source files,
// finds all vars initialized with an Error{...} composite literal,
// pulls out the Code field, and fails if there are duplicates.
func TestErrorCodesAreUnique(t *testing.T) {
	// Reflection can’t list all package-level vars,
	// so the only way is to scan the package’s AST

	fset := token.NewFileSet()

	// Parse all non-test .go files in this directory
	pkgs, err := parser.ParseDir(fset, ".", func(info fs.FileInfo) bool {
		name := info.Name()
		return strings.HasSuffix(name, ".go") && !strings.HasSuffix(name, "_test.go")
	}, 0)
	if err != nil {
		t.Fatalf("parse dir: %v", err)
	}

	// Find the current package (named "errors")
	pkg, ok := pkgs["errors"]
	if !ok {
		t.Fatalf("package 'errors' not found; got: %v", keys(pkgs))
	}

	type occ struct {
		varName string
		pos     token.Position
	}
	byCode := map[int][]occ{}

	for _, f := range pkg.Files {
		ast.Inspect(f, func(n ast.Node) bool {
			gd, ok := n.(*ast.GenDecl)
			if !ok || gd.Tok != token.VAR {
				return true
			}

			for _, spec := range gd.Specs {
				vs, ok := spec.(*ast.ValueSpec)
				if !ok {
					continue
				}
				// We expect Name = Value pairs.
				for i, name := range vs.Names {
					if i >= len(vs.Values) {
						continue
					}
					cl, ok := vs.Values[i].(*ast.CompositeLit)
					if !ok {
						continue
					}
					// Only consider composite literals of type Error (or pkg-qualified ...Error)
					if !isErrorComposite(cl) {
						continue
					}

					// Find Code: <int> inside the literal.
					if code, ok := extractCodeField(cl); ok {
						byCode[code] = append(byCode[code], occ{
							varName: name.Name,
							pos:     fset.Position(name.Pos()),
						})
					}
				}
			}
			return true
		})
	}

	var dups []string
	for code, occs := range byCode {
		if len(occs) > 1 {
			var refs []string
			for _, o := range occs {
				refs = append(refs, o.varName+"@"+o.pos.String())
			}
			dups = append(dups, strconv.Itoa(code)+": "+strings.Join(refs, ", "))
		}
	}
	if len(dups) > 0 {
		t.Fatalf("duplicate Error.Code values found:\n  %s", strings.Join(dups, "\n  "))
	}
}

// isErrorComposite returns true if the composite literal's type is named "Error"
// (either unqualified or selector-qualified, e.g., errors.Error).
func isErrorComposite(cl *ast.CompositeLit) bool {
	switch t := cl.Type.(type) {
	case *ast.Ident:
		return t.Name == "Error"
	case *ast.SelectorExpr:
		// e.g., somepkg.Error
		return t.Sel.Name == "Error"
	default:
		return false
	}
}

// extractCodeField looks for a "Code: <int>" entry in the composite literal.
func extractCodeField(cl *ast.CompositeLit) (int, bool) {
	for _, elt := range cl.Elts {
		kv, ok := elt.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		keyIdent, ok := kv.Key.(*ast.Ident)
		if !ok || keyIdent.Name != "Code" {
			continue
		}
		if v, ok := kv.Value.(*ast.BasicLit); ok {
			if v.Kind == token.INT {
				// Accept 10, 0x..., with underscores.
				txt := strings.ReplaceAll(v.Value, "_", "")
				n, err := strconv.ParseInt(txt, 0, 32)
				if err == nil {
					return int(n), true
				}
			}
		}
	}
	return 0, false
}

func keys[M ~map[K]V, K comparable, V any](m M) []K {
	out := make([]K, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

// TestMarshalJSON_BaseShape verifies that marshaled output contains "error" and "code",
// and that HTTPstatus is never included in the JSON.
func TestMarshalJSON_BaseShape(t *testing.T) {
	c := qt.New(t)
	e := Error{Err: fmt.Errorf("account not found"), Code: 4003, HTTPstatus: 404}
	data, err := json.Marshal(e)
	c.Assert(err, qt.IsNil)
	var m map[string]any
	c.Assert(json.Unmarshal(data, &m), qt.IsNil)
	c.Assert(m["error"], qt.Equals, "account not found")
	c.Assert(m["code"], qt.Equals, float64(4003))
	_, hasHTTPstatus := m["HTTPstatus"]
	c.Assert(hasHTTPstatus, qt.IsFalse)
	_, hasHTTPstatusLower := m["httpstatus"]
	c.Assert(hasHTTPstatusLower, qt.IsFalse)
}

// TestMarshalJSON_DataNil verifies that the "data" key is absent when Data is nil.
func TestMarshalJSON_DataNil(t *testing.T) {
	c := qt.New(t)
	e := Error{Err: fmt.Errorf("not found"), Code: 4004, HTTPstatus: 404}
	data, err := json.Marshal(e)
	c.Assert(err, qt.IsNil)
	var m map[string]any
	c.Assert(json.Unmarshal(data, &m), qt.IsNil)
	_, hasData := m["data"]
	c.Assert(hasData, qt.IsFalse)
}

// TestMarshalJSON_NilErr verifies that MarshalJSON does not panic and returns stable output
// when Err is nil.
func TestMarshalJSON_NilErr(t *testing.T) {
	c := qt.New(t)
	e := Error{Code: 4006, HTTPstatus: 500}
	data, err := json.Marshal(e)
	c.Assert(err, qt.IsNil)
	var m map[string]any
	c.Assert(json.Unmarshal(data, &m), qt.IsNil)
	c.Assert(m["error"], qt.Equals, "")
	c.Assert(m["code"], qt.Equals, float64(4006))
}

// TestMarshalJSON_DataSet verifies that Data is included under the "data" key when non-nil.
func TestMarshalJSON_DataSet(t *testing.T) {
	c := qt.New(t)
	e := Error{
		Err:        fmt.Errorf("validation failed"),
		Code:       4005,
		HTTPstatus: 400,
		Data:       map[string]string{"field": "email", "reason": "invalid"},
	}
	data, err := json.Marshal(e)
	c.Assert(err, qt.IsNil)
	var m map[string]any
	c.Assert(json.Unmarshal(data, &m), qt.IsNil)
	dataVal, hasData := m["data"]
	c.Assert(hasData, qt.IsTrue)
	nested, ok := dataVal.(map[string]any)
	c.Assert(ok, qt.IsTrue)
	c.Assert(nested["field"], qt.Equals, "email")
	c.Assert(nested["reason"], qt.Equals, "invalid")
}
