package index

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/yifanmeng/codefuse/internal/parser"
	"github.com/yifanmeng/codefuse/pkg/types"
)

func TestExtractVarMap_Python_Assignment(t *testing.T) {
	content := "dao = UserDao()\nresult = dao.findById(token)\n"
	vm := parser.ExtractVarMap(content, "python")
	assert.Contains(t, vm, "dao")
	assert.Equal(t, "UserDao", vm["dao"])
}

func TestExtractVarMap_Java_LocalVariable(t *testing.T) {
	content := "UserDao userDao = new UserDao();\nString result = userDao.findById(token);\n"
	vm := parser.ExtractVarMap(content, "java")
	assert.Contains(t, vm, "userDao")
	assert.Equal(t, "UserDao", vm["userDao"])
}

func TestExtractVarMap_Python_Parameter(t *testing.T) {
	content := "def authenticate(dao: UserDao, token: str):\n    return dao.find(token)\n"
	vm := parser.ExtractVarMap(content, "python")
	assert.Contains(t, vm, "dao")
	assert.Equal(t, "UserDao", vm["dao"])
}

func TestResolveEdge_TypeInference_DottedChain(t *testing.T) {
	g := &Graph{
		Graph: types.Graph{
			Nodes: []types.Node{
				{ID: "svc/auth.py:5:1", Name: "login", File: "svc/auth.py", Line: 5, Column: 1},
				{ID: "db/dao.py:10:1", Name: "findById", File: "db/dao.py", Line: 10, Column: 1},
			},
		},
	}
	g.BuildIndexes()
	g.BuildTrie()

	imports := []types.FileImport{{ShortName: "UserDao", FullPath: "db/dao.py"}}
	varMap := map[string]string{"dao": "UserDao"}

	edge := types.Edge{
		From: "svc/auth.py:5:1", To: "dao.findById",
		Kind: types.EdgeKindCalls, File: "svc/auth.py", Line: 6,
	}

	resolved := resolveEdgeWithTypes(edge, imports, varMap, nil, &g.Graph)
	assert.True(t, len(resolved) > 0, "should resolve dao→UserDao→import→findById")
	assert.Equal(t, "db/dao.py:10:1", resolved[0].To)
}

func TestResolveEdge_TypeInference_GenericType(t *testing.T) {
	g := &Graph{
		Graph: types.Graph{
			Nodes: []types.Node{
				{ID: "svc/auth.py:3:1", Name: "authenticate", File: "svc/auth.py", Line: 3, Column: 1},
				{ID: "db/dao.py:8:1", Name: "query", File: "db/dao.py", Line: 8, Column: 1},
			},
		},
	}
	g.BuildIndexes()
	g.BuildTrie()

	imports := []types.FileImport{{ShortName: "UserDao", FullPath: "db/dao.py"}}
	varMap := map[string]string{"list": "UserDao"}

	edge := types.Edge{
		From: "svc/auth.py:3:1", To: "list.query",
		Kind: types.EdgeKindCalls, File: "svc/auth.py", Line: 4,
	}

	resolved := resolveEdgeWithTypes(edge, imports, varMap, nil, &g.Graph)
	assert.True(t, len(resolved) > 0, "List<UserDao> → resolve to UserDao → find query")
	assert.Equal(t, "db/dao.py:8:1", resolved[0].To)
}

var _ = parser.ExtractVarMap
