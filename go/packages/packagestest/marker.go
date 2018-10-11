// Copyright 2018 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package packagestest

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"io/ioutil"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

var (
	markerComment = []byte("//@")
	testingType   = reflect.TypeOf((*testing.T)(nil))
	markersType   = reflect.TypeOf((*Markers)(nil))
	positionType  = reflect.TypeOf(Position{})
)

// Markers collects and then invokes a set of marker expressions in go source
// code.
// This is intended for use in tests that manipulate go code and have to care
// about markers relative to specific locations within that code.
type Markers struct {
	anchors map[string]Position
	markers []*marker
}

// Position represents a position within a source file.
// It extends token.Position with the offset within the file that the
// position represents.
type Position struct {
	token.Position
	Offset int
}

// marker is the internal representation of a marker within a source file
type marker struct {
	line   *line      // the line on which the marker occurred
	method string     // the method the marker invokes
	args   []ast.Expr // the arguments to pass to that method
}

// line represents a single line within a source file
type line struct {
	file   *file  // the file the line is part of
	offset int    // the byte offset to the start of the line within the file
	number int    // 1 based line number within the file
	value  []byte // the contents of the line
}

// represents a source file that was scanned for markers
type file struct {
	name    string // the name of the file, often its actual full path on disk
	content []byte // the contents of the file
}

// converter from a marker arguments parsed from the comment to reflect values
// passed to the method during Invoke
type converter func(*testing.T, *marker, []ast.Expr) (reflect.Value, []ast.Expr)

// method is used to track expensive to calculate information about Invoke
// methods so that we can work it out once rather than per marker.
type method struct {
	f          reflect.Value // the reflect value of the passed in method
	converters []converter   // the parameter converters for the method
}

// Extract can be called to collect all the markers present in a file.
// This should only be called before the first call to Invoke.
// Markers are a special comment that starts with //@ where the text of the
// comment is parsed as go expressions.
// When the comment body is an identifier, it is treated as syntactic sugar for
// the very common case of declaring an anchor of the same name as the matched
// string. So for instanced
//    //@Name
// is rewritten to be
//    //@mark(Name, "Name")
// If the expression is a call expression, of the form
//    //@method(params)
// then the parameters are also normal go expressions of a limited sub-set.
// There is some extra handling when a parameter is being coerced into a
// Position type by the target method.
// If the parameter is an identifier, it will be treated as the name of an
// anchor to look up (as if anchors were global variables).
// If it is a double quoted string, then it will be treated as a string to match
// in the current line, and the position will be the one at the start of that
// match.
// If it is a backtick string, then it will be interpreted as a regular
// expression to match against the current line, and the position will be the
// one at the start of the pattern match.
func (m *Markers) Extract(filename string, content []byte) error {
	f := &file{name: filename, content: content}
	if f.content == nil {
		var err error
		if f.content, err = ioutil.ReadFile(f.name); err != nil {
			return fmt.Errorf("Could not read test file: %v", err)
		}
	}
	offset := 0
	// iterate over all the lines
	// we presume that all source files are small enough to fit in memory easily
	for n, lValue := range bytes.SplitAfter(f.content, []byte("\n")) {
		// split on the special comment markers
		parts := bytes.Split(lValue, markerComment)
		l := &line{
			file:   f,
			number: n + 1,
			value:  parts[0], // we store the line without markers, this prevents markers from matching themselves
			offset: offset,
		}
		offset += len(lValue)
		for _, part := range parts[1:] {

			body := strings.TrimSpace(string(part))
			expr, err := parser.ParseExpr(body)
			if err != nil {
				return fmt.Errorf("%v:%v", l, err)
			}
			switch expr := expr.(type) {
			case *ast.Ident:
				s := &ast.BasicLit{
					ValuePos: expr.NamePos,
					Kind:     token.STRING,
					Value:    strconv.Quote(expr.Name),
				}
				m.markers = append(m.markers, &marker{method: "mark", args: []ast.Expr{expr, s}, line: l})
			case *ast.CallExpr:
				name, ok := expr.Fun.(*ast.Ident)
				if !ok {
					return fmt.Errorf("Function must be an identifier, got %T in %s at %v", expr.Fun, body, l)
				}
				m.markers = append(m.markers, &marker{method: name.Name, args: expr.Args, line: l})
			default:
				return fmt.Errorf("Unhandled marker expression type %T in %s at %v", expr, body, l)
			}
		}
	}
	return nil
}

// Anchors returns the set of anchors that were present in the files processed.
// It is not safe to add any more files after this method has been called.
// Anchors are declared with either the
//    //@Name
// or
//    //@mark(Name, pattern)
// forms.
func (m *Markers) Anchors(t *testing.T) map[string]Position {
	if m.anchors == nil {
		// no anchors yet, pre invoke the special mark marker.
		m.anchors = make(map[string]Position)
		m.Invoke(t, map[string]interface{}{
			"mark": func(t *testing.T, m *Markers, name string, pos Position) {
				if old, found := m.anchors[name]; found {
					t.Errorf("Anchor %v already exists at %v, found %v", name, old, pos)
					return
				}
				m.anchors[name] = pos
			},
		})
	}
	return m.anchors
}

// Invoke is called to evaluate the markers found.
// It is passed the methods, which will be bound by name to the marker functions being
// invoked.
// Markers that do not have a matching method will be skipped.
// It is not safe to add any more files after this method has been called.
// It is safe to all this as many times as you like, and you can repeat the same method name with
// a different implementation if you like.
func (m *Markers) Invoke(t *testing.T, methods map[string]interface{}) {
	m.Anchors(t) // Make sure we have collected the anchors so we can refer to them by name
	ms := make(map[string]method, len(methods))
	for name, f := range methods {
		mi := method{f: reflect.ValueOf(f)}
		mi.converters = make([]converter, mi.f.Type().NumIn())
		for i := 0; i < len(mi.converters); i++ {
			mi.converters[i] = m.buildConverter(t, mi.f.Type().In(i))
		}
		ms[name] = mi
	}
	for _, a := range m.markers {
		mi, ok := ms[a.method]
		if !ok {
			continue
		}
		params := make([]reflect.Value, len(mi.converters))
		args := a.args
		for i, convert := range mi.converters {
			params[i], args = convert(t, a, args)
		}
		if len(args) > 0 {
			t.Fatalf("Unwanted args got %+v extra to %v", sprintArgs(args...), a)
		}
		mi.f.Call(params)
	}
}

// buildConverter works out what function should be used to go from an ast expressions to a reflect
// value of the type expected by a method.
// It is called when only the target type is know, it returns converters that are flexible across
// all supported expression types for that target type.
func (m *Markers) buildConverter(t *testing.T, pt reflect.Type) converter {
	switch {
	case pt == testingType:
		return func(t *testing.T, a *marker, args []ast.Expr) (reflect.Value, []ast.Expr) {
			return reflect.ValueOf(t), args
		}
	case pt == markersType:
		return func(t *testing.T, a *marker, args []ast.Expr) (reflect.Value, []ast.Expr) {
			return reflect.ValueOf(m), args
		}
	case pt == positionType:
		return func(t *testing.T, a *marker, args []ast.Expr) (reflect.Value, []ast.Expr) {
			if len(args) < 1 {
				t.Fatalf("Missing argument for %v", a)
			}
			arg := args[0]
			args = args[1:]
			switch arg := arg.(type) {
			case *ast.Ident:
				// look up an anchor by name
				p, ok := m.anchors[arg.Name]
				if !ok {
					t.Fatalf("Cannot find anchor %v for %v", arg.Name, a)
				}
				return reflect.ValueOf(p), args
			case *ast.BasicLit:
				s, err := strconv.Unquote(arg.Value)
				if err != nil {
					t.Fatalf("Invalid string literal %v for %v", arg.Value, a)
				}
				p := Position{
					Position: token.Position{
						Filename: a.line.file.name,
						Line:     a.line.number,
					},
				}
				i := -1
				if arg.Value[0] == '`' {
					re, err := regexp.Compile(s)
					if err != nil {
						t.Fatalf("%v in %v", err, a.line)
					}
					if m := re.FindIndex(a.line.value); m != nil {
						i = m[0]
					}
				} else {
					i = bytes.Index(a.line.value, []byte(s))
				}
				if i < 0 {
					t.Fatalf("Pattern %v was not present in line %v", s, a.line)
				}
				p.Offset = a.line.offset + i
				p.Column = len(string(a.line.value[:i])) + 1
				return reflect.ValueOf(p), args
			default:
				t.Fatalf("Cannot convert %s to position for %v", sprintArgs(arg), a)
				panic("unreachable")
			}
		}
	case pt.Kind() == reflect.String:
		return func(t *testing.T, a *marker, args []ast.Expr) (reflect.Value, []ast.Expr) {
			arg := args[0]
			args = args[1:]
			switch arg := arg.(type) {
			case *ast.Ident:
				return reflect.ValueOf(arg.Name), args
			case *ast.BasicLit:
				if arg.Kind != token.STRING {
					t.Fatalf("Non string literal %v", sprintArgs(arg))
				}
				s, err := strconv.Unquote(arg.Value)
				if err != nil {
					t.Fatalf("Invalid string literal %v", arg.Value)
				}
				return reflect.ValueOf(s), args
			default:
				t.Fatalf("Cannot convert %v to string", sprintArgs(arg))
				panic("unreachable")
			}
		}
	case pt.Kind() == reflect.Int:
		return func(t *testing.T, a *marker, args []ast.Expr) (reflect.Value, []ast.Expr) {
			arg := args[0]
			args = args[1:]
			lit, ok := arg.(*ast.BasicLit)
			if !ok {
				t.Fatalf("Integer args must be a literal, got %v", sprintArgs(arg))
			}
			if lit.Kind != token.INT {
				t.Fatalf("Non integer literal %v", sprintArgs(arg))
			}
			v, err := strconv.Atoi(lit.Value)
			if err != nil {
				t.Fatalf("Cannot convert %v to int: %v", sprintArgs(arg), err)
			}
			return reflect.ValueOf(v), args
		}
	default:
		t.Fatalf("Action param has invalid type %v(%T)", pt, pt)
		panic("unreachable")
	}
}

// sprintArgs is a small helper for pretty printing the ast expression list in error messages.
func sprintArgs(args ...ast.Expr) string {
	fset := token.NewFileSet()
	buf := &bytes.Buffer{}
	buf.WriteString("[")
	for i, arg := range args {
		if i > 0 {
			buf.WriteString(", ")
		}
		printer.Fprint(buf, fset, arg)
	}
	buf.WriteString("]")
	return buf.String()
}

func (p Position) Format(f fmt.State, c rune) {
	fmt.Fprintf(f, "%d=%v", p.Offset, p.Position)
}

func (l line) Format(f fmt.State, c rune) {
	fmt.Fprintf(f, "%s:%d", l.file.name, l.number)
}

func (a *marker) Format(f fmt.State, c rune) {
	fmt.Fprintf(f, "%s@%v", a.method, a.line)
}
