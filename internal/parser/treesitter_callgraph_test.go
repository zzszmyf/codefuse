package parser

import (
	"encoding/xml"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yifanmeng/codefuse/pkg/types"
)

// TestParseCallGraphFromXML_JavaScript tests call graph extraction from JS AST.
func TestParseCallGraphFromXML_JavaScript(t *testing.T) {
	// Mock tree-sitter XML output for:
	// function foo() { bar(); }
	// function bar() { }
	xmlData := `<?xml version="1.0"?>
<sources>
  <source name="test.js">
    <program>
      <function_declaration srow="0" scol="0" erow="0" ecol="25">
        <identifier field="name">foo</identifier>
        <statement_block>
          <call_expression srow="0" scol="17" erow="0" ecol="22">
            <identifier field="function">bar</identifier>
            <arguments></arguments>
          </call_expression>
        </statement_block>
      </function_declaration>
      <function_declaration srow="1" scol="0" erow="1" ecol="15">
        <identifier field="name">bar</identifier>
        <statement_block></statement_block>
      </function_declaration>
    </program>
  </source>
</sources>`

	graph := buildMockGraph([]types.Node{
		{ID: "test.js:1:1", Name: "foo", Kind: types.KindFunction, File: "test.js", Line: 1},
		{ID: "test.js:4:1", Name: "bar", Kind: types.KindFunction, File: "test.js", Line: 4},
	})

	edges, err := parseCallGraphFromXML([]byte(xmlData), "test.js", graph)
	require.NoError(t, err)
	require.Len(t, edges, 1)

	assert.Equal(t, "test.js:1:1", edges[0].From)
	assert.Equal(t, "test.js:4:1", edges[0].To)
	assert.Equal(t, types.EdgeKindCalls, edges[0].Kind)
	assert.Equal(t, "test.js", edges[0].File)
	assert.Equal(t, 1, edges[0].Line) // srow=0 → line 1
}

// TestParseCallGraphFromXML_MethodCall tests method call extraction.
func TestParseCallGraphFromXML_MethodCall(t *testing.T) {
	// class Calculator { add(a, b) { this.validate(); } validate() {} }
	xmlData := `<?xml version="1.0"?>
<sources>
  <source name="test.js">
    <program>
      <class_declaration srow="0" scol="0" erow="2" ecol="1">
        <identifier field="name">Calculator</identifier>
        <class_body>
          <method_definition srow="0" scol="21" erow="1" ecol="3">
            <property_identifier field="name">add</property_identifier>
            <statement_block>
              <call_expression srow="0" scol="36" erow="0" ecol="53">
                <member_expression>
                  <this></this>
                  <property_identifier field="property">validate</property_identifier>
                </member_expression>
                <arguments></arguments>
              </call_expression>
            </statement_block>
          </method_definition>
          <method_definition srow="1" scol="3" erow="2" ecol="1">
            <property_identifier field="name">validate</property_identifier>
            <statement_block></statement_block>
          </method_definition>
        </class_body>
      </class_declaration>
    </program>
  </source>
</sources>`

	graph := buildMockGraph([]types.Node{
		{ID: "test.js:2:3", Name: "add", Kind: types.KindMethod, File: "test.js", Line: 2},
		{ID: "test.js:4:3", Name: "validate", Kind: types.KindMethod, File: "test.js", Line: 4},
	})

	edges, err := parseCallGraphFromXML([]byte(xmlData), "test.js", graph)
	require.NoError(t, err)
	require.Len(t, edges, 1)

	assert.Equal(t, "test.js:2:3", edges[0].From)
	assert.Equal(t, "test.js:4:3", edges[0].To)
}

// TestParseCallGraphFromXML_NoMatch tests that unmatched calls are skipped.
func TestParseCallGraphFromXML_NoMatch(t *testing.T) {
	// function foo() { unknownFunc(); }
	xmlData := `<?xml version="1.0"?>
<sources>
  <source name="test.js">
    <program>
      <function_declaration srow="0" scol="0" erow="0" ecol="25">
        <identifier field="name">foo</identifier>
        <statement_block>
          <call_expression srow="0" scol="17" erow="0" ecol="31">
            <identifier field="function">unknownFunc</identifier>
            <arguments></arguments>
          </call_expression>
        </statement_block>
      </function_declaration>
    </program>
  </source>
</sources>`

	graph := buildMockGraph([]types.Node{
		{ID: "test.js:1:1", Name: "foo", Kind: types.KindFunction, File: "test.js", Line: 1},
	})

	edges, err := parseCallGraphFromXML([]byte(xmlData), "test.js", graph)
	require.NoError(t, err)
	assert.Empty(t, edges, "unmatched callee should not produce edge")
}

// TestExtractFunctionName covers various declaration types.
func TestExtractFunctionName(t *testing.T) {
	tests := []struct {
		tag      string
		childTag string
		childName string
		expected string
	}{
		{"function_declaration", "identifier", "foo", "foo"},
		{"function_definition", "identifier", "bar", "bar"},
		{"method_definition", "property_identifier", "baz", "baz"},
		{"class_declaration", "identifier", "Cls", ""},
	}

	for _, tt := range tests {
		node := tsNode{
			XMLName: xml.Name{Local: tt.tag},
			Nodes: []tsNode{
				{XMLName: xml.Name{Local: tt.childTag}, Name: tt.childName},
			},
		}
		got := extractFunctionName(node)
		assert.Equal(t, tt.expected, got, "tag=%s", tt.tag)
	}
}

// TestExtractCalleeName covers identifier and member_expression calls.
func TestExtractCalleeName(t *testing.T) {
	// identifier call: foo()
	identCall := tsNode{
		XMLName: xml.Name{Local: "call_expression"},
		Nodes: []tsNode{
			{XMLName: xml.Name{Local: "identifier"}, Name: "foo"},
			{XMLName: xml.Name{Local: "arguments"}},
		},
	}
	assert.Equal(t, "foo", extractCalleeName(identCall))

	// member_expression call: obj.foo()
	memberCall := tsNode{
		XMLName: xml.Name{Local: "call_expression"},
		Nodes: []tsNode{
			{
				XMLName: xml.Name{Local: "member_expression"},
				Nodes: []tsNode{
					{XMLName: xml.Name{Local: "this"}},
					{XMLName: xml.Name{Local: "property_identifier"}, Name: "foo"},
				},
			},
			{XMLName: xml.Name{Local: "arguments"}},
		},
	}
	assert.Equal(t, "foo", extractCalleeName(memberCall))
}

// buildMockGraph creates a Graph with indexes built.
func buildMockGraph(nodes []types.Node) *types.Graph {
	g := &types.Graph{
		Nodes: nodes,
	}
	g.BuildIndexes()
	return g
}


