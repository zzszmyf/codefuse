package parser

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yifanmeng/codefuse/pkg/types"
)

func TestExtractGoSymbols_BasicFile(t *testing.T) {
	src := []byte(`package main

import "fmt"

// User represents a user in the system.
type User struct {
	Name string
}

// Hello greets someone.
func Hello(name string) string {
	return "Hello, " + name
}

// Greet is a method on User.
func (u *User) Greet() string {
	return "Hi, " + u.Name
}

const MaxUsers = 100

var globalCount = 0
`)

	syms, err := ExtractGoSymbols("test.go", src)
	require.NoError(t, err)

	// Debug
	for _, sym := range syms {
		t.Logf("%s %s line=%d parent=%s", sym.Kind, sym.Name, sym.Line, sym.Parent)
	}

	require.Len(t, syms, 6) // package, User, Hello, Greet, MaxUsers, globalCount

	// Check package
	assert.Equal(t, "main", syms[0].Name)
	assert.Equal(t, types.KindPackage, syms[0].Kind)

	// Check struct
	assert.Equal(t, "User", syms[1].Name)
	assert.Equal(t, types.KindStruct, syms[1].Kind)
	assert.Contains(t, syms[1].Docstring, "User represents")

	// Check function
	assert.Equal(t, "Hello", syms[2].Name)
	assert.Equal(t, types.KindFunction, syms[2].Kind)
	assert.Contains(t, syms[2].Docstring, "Hello greets")

	// Check method
	assert.Equal(t, "Greet", syms[3].Name)
	assert.Equal(t, types.KindMethod, syms[3].Kind)
	assert.Equal(t, "User", syms[3].Parent)

	// Check const
	assert.Equal(t, "MaxUsers", syms[4].Name)
	assert.Equal(t, types.KindConstant, syms[4].Kind)

	// Check var
	assert.Equal(t, "globalCount", syms[5].Name)
	assert.Equal(t, types.KindVariable, syms[5].Kind)
}

func TestExtractGoSymbols_Interface(t *testing.T) {
	src := []byte(`package main

type Greeter interface {
	Hello() string
}
`)

	syms, err := ExtractGoSymbols("test.go", src)
	require.NoError(t, err)

	found := false
	for _, sym := range syms {
		if sym.Name == "Greeter" {
			found = true
			assert.Equal(t, types.KindInterface, sym.Kind)
		}
	}
	assert.True(t, found)
}

func TestExtractGoSymbols_MethodWithPointerReceiver(t *testing.T) {
	src := []byte(`package main

type Calculator struct{}

func (c *Calculator) Add(a, b int) int {
	return a + b
}
`)

	syms, err := ExtractGoSymbols("test.go", src)
	require.NoError(t, err)

	var method *types.Symbol
	for i := range syms {
		if syms[i].Name == "Add" {
			method = &syms[i]
			break
		}
	}
	require.NotNil(t, method)
	assert.Equal(t, types.KindMethod, method.Kind)
	assert.Equal(t, "Calculator", method.Parent)
}
