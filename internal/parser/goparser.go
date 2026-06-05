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


