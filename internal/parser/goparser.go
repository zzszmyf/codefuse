package parser

import (
	"go/ast"
	"go/parser"
	"go/token"

	"github.com/yifanmeng/codefuse/pkg/types"
)

// ExtractGoSymbols uses go/ast (the official Go parser) to extract symbols.
// This is 100% accurate and requires zero external dependencies.
func ExtractGoSymbols(filePath string, src []byte) ([]types.Symbol, error) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, filePath, src, parser.ParseComments)
	if err != nil {
		return nil, err
	}

	var syms []types.Symbol

	// Package name
	syms = append(syms, types.Symbol{
		Name: f.Name.Name,
		Kind: types.KindPackage,
		File: filePath,
		Line: fset.Position(f.Name.Pos()).Line,
	})

	// Walk declarations
	for _, decl := range f.Decls {
		switch d := decl.(type) {
		case *ast.FuncDecl:
			sym := types.Symbol{
				Name:    d.Name.Name,
				File:    filePath,
				Line:    fset.Position(d.Pos()).Line,
				EndLine: fset.Position(d.End()).Line,
			}
			if d.Recv != nil && len(d.Recv.List) > 0 {
				sym.Kind = types.KindMethod
				sym.Parent = extractReceiverType(d.Recv.List[0].Type)
			} else {
				sym.Kind = types.KindFunction
			}
			if d.Doc != nil {
				sym.Docstring = d.Doc.Text()
			}
			if d.Type != nil {
				sym.Signature = extractFuncSignature(d)
			}
			syms = append(syms, sym)

		case *ast.GenDecl:
			for _, spec := range d.Specs {
				switch s := spec.(type) {
				case *ast.TypeSpec:
					kind := types.KindType
					switch s.Type.(type) {
					case *ast.StructType:
						kind = types.KindStruct
					case *ast.InterfaceType:
						kind = types.KindInterface
					}
					sym := types.Symbol{
						Name:    s.Name.Name,
						Kind:    kind,
						File:    filePath,
						Line:    fset.Position(s.Pos()).Line,
						EndLine: fset.Position(s.End()).Line,
					}
					if d.Doc != nil {
						sym.Docstring = d.Doc.Text()
					}
					syms = append(syms, sym)

				case *ast.ValueSpec:
					for _, name := range s.Names {
						kind := types.KindVariable
						if d.Tok == token.CONST {
							kind = types.KindConstant
						}
						syms = append(syms, types.Symbol{
							Name: name.Name,
							Kind: kind,
							File: filePath,
							Line: fset.Position(name.Pos()).Line,
						})
					}
				}
			}
		}
	}

	return syms, nil
}

func extractReceiverType(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.StarExpr:
		return extractReceiverType(t.X)
	case *ast.IndexExpr:
		return extractReceiverType(t.X)
	case *ast.IndexListExpr:
		return extractReceiverType(t.X)
	}
	return ""
}

func extractFuncSignature(f *ast.FuncDecl) string {
	// Build a simplified signature: func Name(params) returns
	var sig string
	if f.Recv != nil && len(f.Recv.List) > 0 {
		sig = "func (" + extractReceiverType(f.Recv.List[0].Type) + ") " + f.Name.Name + "("
	} else {
		sig = "func " + f.Name.Name + "("
	}
	if f.Type.Params != nil {
		sig += "...)"
	} else {
		sig += ")"
	}
	return sig
}

// =============================================================================
// Graph Model: Node + Edge extraction for v0.2
// =============================================================================

// ExtractGoNodes extracts Node objects and the package name from a Go file.
func ExtractGoNodes(filePath string, src []byte) ([]types.Node, string, error) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, filePath, src, parser.ParseComments)
	if err != nil {
		return nil, "", err
	}

	pkgName := f.Name.Name
	var nodes []types.Node

	// Package node
	nodes = append(nodes, types.Node{
		ID:     types.GoNodeID(pkgName, "", f.Name.Name),
		Name:   f.Name.Name,
		Kind:   types.KindPackage,
		File:   filePath,
		Line:   fset.Position(f.Name.Pos()).Line,
		Column: fset.Position(f.Name.Pos()).Column,
	})

	for _, decl := range f.Decls {
		switch d := decl.(type) {
		case *ast.FuncDecl:
			node := types.Node{
				Name:    d.Name.Name,
				File:    filePath,
				Line:    fset.Position(d.Pos()).Line,
				EndLine: fset.Position(d.End()).Line,
			}
			var receiver string
			if d.Recv != nil && len(d.Recv.List) > 0 {
				node.Kind = types.KindMethod
				receiver = extractReceiverType(d.Recv.List[0].Type)
				node.Parent = receiver
			} else {
				node.Kind = types.KindFunction
			}
			if d.Doc != nil {
				node.Docstring = d.Doc.Text()
			}
			if d.Type != nil {
				node.Signature = extractFuncSignature(d)
			}
			node.ID = types.GoNodeID(pkgName, receiver, d.Name.Name)
			node.Column = fset.Position(d.Pos()).Column
			nodes = append(nodes, node)

		case *ast.GenDecl:
			for _, spec := range d.Specs {
				switch s := spec.(type) {
				case *ast.TypeSpec:
					kind := types.KindType
					switch s.Type.(type) {
					case *ast.StructType:
						kind = types.KindStruct
					case *ast.InterfaceType:
						kind = types.KindInterface
					}
					node := types.Node{
						ID:      types.GoNodeID(pkgName, "", s.Name.Name),
						Name:    s.Name.Name,
						Kind:    kind,
						File:    filePath,
						Line:    fset.Position(s.Pos()).Line,
						EndLine: fset.Position(s.End()).Line,
						Column:  fset.Position(s.Pos()).Column,
					}
					if d.Doc != nil {
						node.Docstring = d.Doc.Text()
					}
					nodes = append(nodes, node)

				case *ast.ValueSpec:
					for _, name := range s.Names {
						kind := types.KindVariable
						if d.Tok == token.CONST {
							kind = types.KindConstant
						}
						nodes = append(nodes, types.Node{
							ID:     types.GoNodeID(pkgName, "", name.Name),
							Name:   name.Name,
							Kind:   kind,
							File:   filePath,
							Line:   fset.Position(name.Pos()).Line,
							Column: fset.Position(name.Pos()).Column,
						})
					}
				}
			}
		}
	}

	return nodes, pkgName, nil
}

// ExtractGoCallGraph extracts call relationships (edges) from a Go file.
// It uses heuristic matching since full type checking requires go/types + all dependencies.
//
// Resolution strategy:
//   - *ast.Ident: same-package call → lookup "pkg.FuncName"
//   - *ast.SelectorExpr where X is a known package: cross-package call → lookup "otherpkg.FuncName"
//   - *ast.SelectorExpr where X is a local variable: method call → skipped (needs type inference)
//   - *ast.FuncLit, *ast.CallExpr inside composite literals: skipped
func ExtractGoCallGraph(filePath string, src []byte, pkgName string, pkgNames map[string]string, graph *types.Graph) ([]types.Edge, error) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, filePath, src, 0)
	if err != nil {
		return nil, err
	}

	// Build reverse lookup: package name -> known (used for cross-package resolution).
	knownPackages := make(map[string]bool)
	for _, pn := range pkgNames {
		knownPackages[pn] = true
	}

	var edges []types.Edge

	ast.Inspect(f, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}

		callerID := findEnclosingFuncID(f, call.Pos(), pkgName)
		if callerID == "" {
			return true
		}

		calleeID := resolveCalleeID(call.Fun, pkgName, knownPackages, graph)
		if calleeID != "" {
			edges = append(edges, types.Edge{
				From: callerID,
				To:   calleeID,
				Kind: types.EdgeKindCalls,
				File: filePath,
				Line: fset.Position(call.Pos()).Line,
			})
		}
		return true
	})

	return edges, nil
}

// findEnclosingFuncID returns the Node ID of the function/method that contains pos.
func findEnclosingFuncID(f *ast.File, pos token.Pos, pkgName string) string {
	for _, decl := range f.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok {
			continue
		}
		if fn.Pos() <= pos && pos <= fn.End() {
			receiver := ""
			if fn.Recv != nil && len(fn.Recv.List) > 0 {
				receiver = extractReceiverType(fn.Recv.List[0].Type)
			}
			return types.GoNodeID(pkgName, receiver, fn.Name.Name)
		}
	}
	return ""
}

// resolveCalleeID determines the Node ID of a callee from a CallExpr.Fun expression.
func resolveCalleeID(fun ast.Expr, pkgName string, knownPackages map[string]bool, graph *types.Graph) string {
	switch expr := fun.(type) {
	case *ast.Ident:
		// Same-package function call: ValidateToken(user)
		calleeID := types.GoNodeID(pkgName, "", expr.Name)
		if graph.FindNodeByID(calleeID) != nil {
			return calleeID
		}
		// Also try as a method on the same package's types (heuristic)
		return ""

	case *ast.SelectorExpr:
		xIdent, ok := expr.X.(*ast.Ident)
		if !ok {
			return ""
		}
		// Case 1: X is a known package name → cross-package call: auth.Authenticate
		if knownPackages[xIdent.Name] {
			calleeID := types.GoNodeID(xIdent.Name, "", expr.Sel.Name)
			if graph.FindNodeByID(calleeID) != nil {
				return calleeID
			}
			// Maybe it's a method in that package? Try with common receiver names.
			// This is a heuristic fallback.
			return ""
		}

		// Case 2: X is a local variable → method call: s.GetName
		// Without type inference we cannot resolve this accurately.
		// Try a heuristic: look for any method named GetName in the graph.
		// But to avoid false positives, we skip this in v0.2 unless we can match unambiguously.
		candidates := graph.FindNodeByName(expr.Sel.Name, types.KindMethod)
		if len(candidates) == 1 {
			return candidates[0].ID
		}
		return ""

	case *ast.FuncLit:
		// Anonymous function invocation — skip.
		return ""

	case *ast.CallExpr:
		// Function returns a function: getFn()() — skip the outer, resolve the inner if possible.
		return ""

	default:
		return ""
	}
}
