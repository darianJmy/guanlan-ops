// Command generate-operation-catalog builds the reviewed operation allowlist.
// Run from the repository root: go run ./cmd/generate-operation-catalog
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
)

type operation struct {
	Name, Handler, Method, Path, Permission, Title string
	AdminOnly                                      bool `json:"admin_only"`
}

var types = map[string]ast.Expr{}
var funcs = map[string]*ast.FuncDecl{}

type object = map[string]interface{}

func obj() object {
	return object{"type": "object", "properties": object{}, "additionalProperties": false}
}
func literal(e ast.Expr) string {
	if b, ok := e.(*ast.BasicLit); ok {
		s, _ := strconv.Unquote(b.Value)
		return s
	}
	return ""
}
func schema(e ast.Expr, pkg, tag string, depth int) object {
	if depth > 15 {
		panic("recursive request type")
	}
	switch t := e.(type) {
	case *ast.StarExpr:
		return schema(t.X, pkg, tag, depth+1)
	case *ast.Ident:
		switch t.Name {
		case "string":
			return object{"type": "string", "maxLength": 65536}
		case "bool":
			return object{"type": "boolean"}
		case "float32", "float64":
			return object{"type": "number"}
		case "int", "int64", "int32", "uint", "uint64", "uint32":
			return object{"type": "integer"}
		}
		if v, ok := types[pkg+"."+t.Name]; ok {
			return schema(v, pkg, tag, depth+1)
		}
		panic("unresolved type " + pkg + "." + t.Name)
	case *ast.SelectorExpr:
		p := t.X.(*ast.Ident).Name
		if p == "time" && t.Sel.Name == "Time" {
			return object{"type": "string"}
		}
		return schema(&ast.Ident{Name: t.Sel.Name}, p, tag, depth+1)
	case *ast.ArrayType:
		return object{"type": "array", "items": schema(t.Elt, pkg, tag, depth+1), "maxItems": 1000}
	case *ast.MapType:
		return object{"type": "object", "additionalProperties": schema(t.Value, pkg, tag, depth+1)}
	case *ast.InterfaceType:
		return object{}
	case *ast.StructType:
		out := obj()
		props := out["properties"].(object)
		req := []string{}
		for _, f := range t.Fields.List {
			if f.Tag == nil {
				continue
			}
			raw, _ := strconv.Unquote(f.Tag.Value)
			st := reflect.StructTag(raw)
			name := strings.Split(st.Get(tag), ",")[0]
			if name == "" || name == "-" {
				continue
			}
			props[name] = schema(f.Type, pkg, tag, depth+1)
			if strings.Contains(","+st.Get("binding")+",", ",required,") {
				req = append(req, name)
			}
		}
		if len(req) > 0 {
			out["required"] = req
		}
		return out
	}
	panic(fmt.Sprintf("unsupported request type %T", e))
}
func functionSchema(name string) (object, object) {
	return functionSchemaRecursive(name, map[string]bool{})
}
func functionSchemaRecursive(name string, visited map[string]bool) (object, object) {
	if visited[name] {
		return obj(), obj()
	}
	visited[name] = true
	fn := funcs[name]
	if fn == nil {
		panic("missing handler " + name)
	}
	vars := map[string]ast.Expr{}
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		switch v := n.(type) {
		case *ast.ValueSpec:
			for _, id := range v.Names {
				vars[id.Name] = v.Type
			}
		case *ast.AssignStmt:
			for i, r := range v.Rhs {
				if i >= len(v.Lhs) {
					continue
				}
				id, ok := v.Lhs[i].(*ast.Ident)
				if !ok {
					continue
				}
				if c, ok := r.(*ast.CompositeLit); ok {
					vars[id.Name] = c.Type
				}
			}
		}
		return true
	})
	body, query := obj(), obj()
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		if id, ok := call.Fun.(*ast.Ident); ok && funcs[id.Name] != nil {
			passesContext := false
			for _, arg := range call.Args {
				if id, ok := arg.(*ast.Ident); ok && id.Name == "c" {
					passesContext = true
				}
			}
			if passesContext {
				childBody, childQuery := functionSchemaRecursive(id.Name, visited)
				for key, value := range childQuery["properties"].(object) {
					query["properties"].(object)[key] = value
				}
				if props, _ := childBody["properties"].(object); len(props) > 0 || childBody["additionalProperties"] != false {
					body = childBody
				}
			}
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		if len(call.Args) == 0 {
			return true
		}
		switch sel.Sel.Name {
		case "ShouldBindJSON", "BindJSON", "ShouldBindQuery":
			u, ok := call.Args[0].(*ast.UnaryExpr)
			if !ok {
				panic("nonvariable binding " + name)
			}
			id, ok := u.X.(*ast.Ident)
			if !ok {
				panic("binding " + name)
			}
			expr := vars[id.Name]
			if expr == nil {
				panic("missing request type " + name + " " + id.Name)
			}
			if sel.Sel.Name == "ShouldBindQuery" {
				query = schema(expr, "handlers", "form", 0)
			} else {
				body = schema(expr, "handlers", "json", 0)
			}
		case "Query", "DefaultQuery", "GetQuery":
			key := literal(call.Args[0])
			if key != "" {
				query["properties"].(object)[key] = object{"type": "string", "description": "URL 查询参数，以字符串传递"}
			}
		}
		return true
	})
	return body, query
}
func main() {
	for _, pkg := range []string{"handlers", "models"} {
		paths, _ := filepath.Glob("internal/" + pkg + "/*.go")
		for _, p := range paths {
			if strings.HasSuffix(p, "_test.go") || strings.HasSuffix(p, "_generated.go") {
				continue
			}
			f, err := parser.ParseFile(token.NewFileSet(), p, nil, 0)
			if err != nil {
				panic(err)
			}
			for _, d := range f.Decls {
				switch v := d.(type) {
				case *ast.GenDecl:
					for _, s := range v.Specs {
						if t, ok := s.(*ast.TypeSpec); ok {
							types[pkg+"."+t.Name.Name] = t.Type
						}
					}
				case *ast.FuncDecl:
					if pkg == "handlers" {
						funcs[v.Name.Name] = v
					}
				}
			}
		}
	}
	raw, err := os.ReadFile("cmd/generate-operation-catalog/manifest.json")
	if err != nil {
		panic(err)
	}
	var manifest struct{ Operations []operation }
	if err = json.Unmarshal(raw, &manifest); err != nil {
		panic(err)
	}
	var out bytes.Buffer
	out.WriteString("// Code generated by go run ./cmd/generate-operation-catalog; DO NOT EDIT.\npackage handlers\n\nfunc generatedPlatformOperations() []platformOperation { return []platformOperation{\n")
	for _, op := range manifest.Operations {
		bindingHandler := op.Handler
		if op.Handler == "CreateAlertGroupingRule" || op.Handler == "UpdateAlertGroupingRule" {
			bindingHandler = "bindAlertGroupingRule"
		}
		body, query := functionSchema(bindingHandler)
		args := obj()
		props := args["properties"].(object)
		path := obj()
		required := []string{}
		for _, part := range strings.Split(op.Path, "/") {
			if strings.HasPrefix(part, ":") {
				key := part[1:]
				path["properties"].(object)[key] = object{"type": "integer", "minimum": 1}
				if strings.HasPrefix(op.Path, "/api-tokens/") || strings.HasPrefix(op.Path, "/mcp-clients/") {
					path["properties"].(object)[key] = object{"type": "string", "maxLength": 32}
				}
				required = append(required, key)
			}
		}
		if len(required) > 0 {
			path["required"] = required
			props["path"] = path
			args["required"] = []string{"path"}
		}
		if len(query["properties"].(object)) > 0 {
			props["query"] = query
		}
		bp, _ := body["properties"].(object)
		if len(bp) > 0 || body["additionalProperties"] != false {
			props["body"] = body
			req, _ := args["required"].([]string)
			args["required"] = append(req, "body")
		}
		b, _ := json.Marshal(args)
		fmt.Fprintf(&out, "{Name:%q, Title:%q, Permission:%q, Method:%q, Path:%q, AdminOnly:%t, Handler:%s, SchemaJSON:%q},\n", op.Name, op.Title, op.Permission, op.Method, op.Path, op.AdminOnly, op.Handler, string(b))
	}
	out.WriteString("}}\n")
	formatted, err := format.Source(out.Bytes())
	if err != nil {
		panic(err)
	}
	if err = os.WriteFile("internal/handlers/mcp_operations_generated.go", formatted, 0644); err != nil {
		panic(err)
	}
	fmt.Printf("Generated %d operations\n", len(manifest.Operations))
}
