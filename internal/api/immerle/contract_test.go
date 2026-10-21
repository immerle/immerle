package immerle

import (
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
)

// The OpenAPI spec is generated with --requiredByDefault, and swag only honors
// validate:"optional" to opt out (it ignores omitempty). A field of a spec type
// that can be absent (omitempty) or null (pointer, slice, map, interface)
// without that tag would be typed as always present in the generated TS client.
func TestOptionalWireFieldsAreMarkedOptional(t *testing.T) {
	raw, err := os.ReadFile("../docs/swagger.json")
	if err != nil {
		t.Fatal(err)
	}
	var spec struct {
		Components struct {
			Schemas map[string]json.RawMessage `json:"schemas"`
		} `json:"components"`
	}
	if err := json.Unmarshal(raw, &spec); err != nil {
		t.Fatal(err)
	}
	inSpec := map[string]bool{}
	for name := range spec.Components.Schemas {
		inSpec[name] = true
	}

	fset := token.NewFileSet()
	for pkg, dir := range map[string]string{"immerle": ".", "models": "../../models"} {
		files, err := filepath.Glob(filepath.Join(dir, "*.go"))
		if err != nil {
			t.Fatal(err)
		}
		for _, f := range files {
			if strings.HasSuffix(f, "_test.go") {
				continue
			}
			file, err := parser.ParseFile(fset, f, nil, 0)
			if err != nil {
				t.Fatal(err)
			}
			ast.Inspect(file, func(n ast.Node) bool {
				ts, ok := n.(*ast.TypeSpec)
				if !ok {
					return true
				}
				if inSpec[pkg+"."+ts.Name.Name] {
					checkOptionalFields(t, fset, ts.Type)
				}
				return false
			})
		}
	}
}

// checkOptionalFields walks a struct type, including anonymous nested structs.
func checkOptionalFields(t *testing.T, fset *token.FileSet, typ ast.Expr) {
	ast.Inspect(typ, func(n ast.Node) bool {
		st, ok := n.(*ast.StructType)
		if !ok {
			return true
		}
		for _, fld := range st.Fields.List {
			if fld.Tag == nil {
				continue
			}
			raw, _ := strconv.Unquote(fld.Tag.Value)
			tag := reflect.StructTag(raw)
			name, opts, _ := strings.Cut(tag.Get("json"), ",")
			if name == "-" || tag.Get("json") == "" {
				continue
			}
			mayBeMissing := strings.Contains(opts, "omitempty")
			switch ty := fld.Type.(type) {
			case *ast.StarExpr, *ast.ArrayType, *ast.MapType, *ast.InterfaceType:
				mayBeMissing = true
			case *ast.Ident:
				mayBeMissing = mayBeMissing || ty.Name == "any"
			}
			if mayBeMissing && tag.Get("validate") != "optional" {
				t.Errorf("%s: field %q can be absent or null but lacks validate:\"optional\"", fset.Position(fld.Pos()), name)
			}
		}
		return true
	})
}
