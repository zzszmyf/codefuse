package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestLookup(t *testing.T) {
	assert.NotNil(t, Lookup(".go"))
	assert.NotNil(t, Lookup(".py"))
	assert.NotNil(t, Lookup(".java"))
	assert.NotNil(t, Lookup(".rs"))
	assert.NotNil(t, Lookup(".js"))
	assert.NotNil(t, Lookup(".ts"))
	assert.Nil(t, Lookup(".xyz"))
}

func TestLookup_ReturnsCorrectConfig(t *testing.T) {
	goCfg := Lookup(".go")
	assert.Equal(t, "go", goCfg.Name)
	assert.Contains(t, goCfg.DeclNodes, "function_declaration")
	assert.Contains(t, goCfg.DeclNodes, "method_declaration")
	assert.Contains(t, goCfg.DeclNodes, "type_spec")

	javaCfg := Lookup(".java")
	assert.Equal(t, "java", javaCfg.Name)
	assert.Contains(t, javaCfg.DeclNodes, "method_declaration")
	assert.Contains(t, javaCfg.DeclNodes, "class_declaration")
	assert.Contains(t, javaCfg.CallNodes, "method_invocation")
}

func TestExtToLang(t *testing.T) {
	assert.Equal(t, "go", ExtToLang[".go"])
	assert.Equal(t, "python", ExtToLang[".py"])
	assert.Equal(t, "java", ExtToLang[".java"])
	assert.Equal(t, "javascript", ExtToLang[".js"])
	assert.Equal(t, "typescript", ExtToLang[".ts"])
}

func TestBuiltin_AllLanguagesHaveExtensions(t *testing.T) {
	for name, cfg := range Builtin {
		assert.NotEmpty(t, cfg.Extensions, "language %s has no extensions", name)
		assert.NotEmpty(t, cfg.DeclNodes, "language %s has no decl nodes", name)
	}
}

func TestBuiltin_JavaSupport(t *testing.T) {
	cfg, ok := Builtin["java"]
	assert.True(t, ok, "java should be in builtin config")
	assert.Contains(t, cfg.DeclNodes, "method_declaration")
	assert.Contains(t, cfg.DeclNodes, "class_declaration")
	assert.Contains(t, cfg.DeclNodes, "interface_declaration")
	assert.Contains(t, cfg.DeclNodes, "constructor_declaration")
	assert.Contains(t, cfg.CallNodes, "method_invocation")
}
