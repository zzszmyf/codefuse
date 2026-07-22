package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/yifanmeng/codefuse/internal/index"
)

// =============================================================================
// MCP Server — Model Context Protocol over stdio.
//
// Keeps the index loaded in memory for zero-latency queries.
// Exposes tools: find_symbol, find_callers, find_callees, get_outline.
//
// Claude Code / Cursor / other MCP clients connect via:
//   codefuse serve
// =============================================================================

// JSON-RPC 2.0 types
type rpcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      interface{}     `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type rpcResponse struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      interface{} `json:"id"`
	Result  interface{} `json:"result,omitempty"`
	Error   *rpcError   `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type rpcNotification struct {
	JSONRPC string          `json:"jsonrpc"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// MCP tool definitions
type mcpTool struct {
	Name        string      `json:"name"`
	Description string      `json:"description"`
	InputSchema inputSchema `json:"inputSchema"`
}

type inputSchema struct {
	Type       string              `json:"type"`
	Properties map[string]property `json:"properties"`
	Required   []string            `json:"required,omitempty"`
}

type property struct {
	Type        string `json:"type"`
	Description string `json:"description"`
}

// mcpServer holds the in-memory graph and handles MCP requests.
type mcpServer struct {
	graph      *index.Graph
	projectDir string
	stdin      *bufio.Reader
}

func newMCPServer(projectDir string) (*mcpServer, error) {
	indexDir := filepath.Join(projectDir, ".codefuse")
	graph, err := index.LoadGraph(indexDir)
	if err != nil {
		graph, err = index.LoadGraphNodes(indexDir)
		if err != nil {
			return nil, fmt.Errorf("no index found in %s. Run 'codefuse index .' first", projectDir)
		}
	}
	return &mcpServer{
		graph:      graph,
		projectDir: projectDir,
		stdin:      bufio.NewReader(os.Stdin),
	}, nil
}

func (s *mcpServer) serve() error {
	scanner := bufio.NewScanner(s.stdin)
	// MCP messages can be large (tool results). Use 10MB buffer.
	scanner.Buffer(make([]byte, 1024*1024), 10*1024*1024)

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}

		var req rpcRequest
		if err := json.Unmarshal([]byte(line), &req); err != nil {
			continue
		}

		s.handleRequest(req)
	}
	return scanner.Err()
}

func (s *mcpServer) handleRequest(req rpcRequest) {
	switch req.Method {
	case "initialize":
		s.handleInitialize(req)
	case "tools/list":
		s.handleToolsList(req)
	case "tools/call":
		s.handleToolsCall(req)
	case "notifications/initialized":
		// No response needed for notifications.
	default:
		s.sendError(req.ID, -32601, fmt.Sprintf("unknown method: %s", req.Method))
	}
}

func (s *mcpServer) handleInitialize(req rpcRequest) {
	result := map[string]interface{}{
		"protocolVersion": "2024-11-05",
		"capabilities": map[string]interface{}{
			"tools": map[string]bool{},
		},
		"serverInfo": map[string]interface{}{
			"name":    "codefuse",
			"version": version,
		},
	}
	s.sendResult(req.ID, result)
}

func (s *mcpServer) handleToolsList(req rpcRequest) {
	tools := []mcpTool{
		{
			Name:        "find_symbol",
			Description: "Find symbol definitions by name. Supports exact match, prefix (foo*), glob (*bar), camelCase (PA→PageAttention), and substring search. Returns file:line:column positions.",
			InputSchema: inputSchema{
				Type: "object",
				Properties: map[string]property{
					"name":       {Type: "string", Description: "Symbol name to search for"},
					"ignoreCase": {Type: "boolean", Description: "Case-insensitive search (default: false)"},
				},
				Required: []string{"name"},
			},
		},
		{
			Name:        "find_callers",
			Description: "Find all functions/methods that call the given symbol. The symbol must be specified by its file:line:column ID (from find_symbol results).",
			InputSchema: inputSchema{
				Type: "object",
				Properties: map[string]property{
					"nodeId": {Type: "string", Description: "Node ID in format 'file:line:col' from find_symbol"},
				},
				Required: []string{"nodeId"},
			},
		},
		{
			Name:        "find_callees",
			Description: "Find all functions/methods called by the given symbol.",
			InputSchema: inputSchema{
				Type: "object",
				Properties: map[string]property{
					"nodeId": {Type: "string", Description: "Node ID in format 'file:line:col' from find_symbol"},
				},
				Required: []string{"nodeId"},
			},
		},
		{
			Name:        "get_outline",
			Description: "List all symbols in a file, sorted by line number. Useful for understanding file structure.",
			InputSchema: inputSchema{
				Type: "object",
				Properties: map[string]property{
					"file": {Type: "string", Description: "File path (relative to project root or absolute)"},
				},
				Required: []string{"file"},
			},
		},
	}

	s.sendResult(req.ID, map[string]interface{}{"tools": tools})
}

func (s *mcpServer) handleToolsCall(req rpcRequest) {
	var params struct {
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
	}
	if err := json.Unmarshal(req.Params, &params); err != nil {
		s.sendError(req.ID, -32602, "invalid params")
		return
	}

	var result interface{}
	var err error

	switch params.Name {
	case "find_symbol":
		result, err = s.toolFindSymbol(params.Arguments)
	case "find_callers":
		result, err = s.toolFindCallers(params.Arguments)
	case "find_callees":
		result, err = s.toolFindCallees(params.Arguments)
	case "get_outline":
		result, err = s.toolGetOutline(params.Arguments)
	default:
		s.sendError(req.ID, -32601, fmt.Sprintf("unknown tool: %s", params.Name))
		return
	}

	if err != nil {
		s.sendError(req.ID, -32000, err.Error())
		return
	}

	s.sendResult(req.ID, map[string]interface{}{
		"content": []map[string]interface{}{
			{"type": "text", "text": toJSON(result)},
		},
	})
}

// =============================================================================
// Tool implementations
// =============================================================================

func (s *mcpServer) toolFindSymbol(args json.RawMessage) (interface{}, error) {
	var p struct {
		Name       string `json:"name"`
		IgnoreCase bool   `json:"ignoreCase"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return nil, fmt.Errorf("invalid arguments: %w", err)
	}
	if p.Name == "" {
		return nil, fmt.Errorf("name is required")
	}

	nodes := s.graph.Query(p.Name, p.IgnoreCase)

	type result struct {
		Name   string `json:"name"`
		File   string `json:"file"`
		Line   int    `json:"line"`
		Column int    `json:"column"`
		ID     string `json:"id"`
	}
	var results []result
	for _, n := range nodes {
		results = append(results, result{
			Name:   n.Name,
			File:   n.File,
			Line:   n.Line,
			Column: n.Column,
			ID:     n.ID,
		})
	}
	return map[string]interface{}{
		"count":   len(results),
		"symbols": results,
	}, nil
}

func (s *mcpServer) toolFindCallers(args json.RawMessage) (interface{}, error) {
	var p struct {
		NodeID string `json:"nodeId"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return nil, fmt.Errorf("invalid arguments: %w", err)
	}

	edges := s.graph.FindCallers(p.NodeID)

	type result struct {
		CallerName string `json:"caller_name"`
		File       string `json:"file"`
		Line       int    `json:"line"`
	}
	var results []result
	for _, e := range edges {
		results = append(results, result{
			CallerName: e.Node.Name,
			File:       e.Edge.File,
			Line:       e.Edge.Line,
		})
	}
	return map[string]interface{}{
		"count":   len(results),
		"callers": results,
	}, nil
}

func (s *mcpServer) toolFindCallees(args json.RawMessage) (interface{}, error) {
	var p struct {
		NodeID string `json:"nodeId"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return nil, fmt.Errorf("invalid arguments: %w", err)
	}

	edges := s.graph.FindCallees(p.NodeID)

	type result struct {
		CalleeName string `json:"callee_name"`
		File       string `json:"file"`
		Line       int    `json:"line"`
	}
	var results []result
	for _, e := range edges {
		results = append(results, result{
			CalleeName: e.Node.Name,
			File:       e.Edge.File,
			Line:       e.Edge.Line,
		})
	}
	return map[string]interface{}{
		"count":   len(results),
		"callees": results,
	}, nil
}

func (s *mcpServer) toolGetOutline(args json.RawMessage) (interface{}, error) {
	var p struct {
		File string `json:"file"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return nil, fmt.Errorf("invalid arguments: %w", err)
	}

	type symbol struct {
		Name   string `json:"name"`
		Line   int    `json:"line"`
		Column int    `json:"column"`
	}
	var symbols []symbol
	for _, n := range s.graph.Nodes {
		if n.File == p.File || strings.HasSuffix(n.File, p.File) {
			symbols = append(symbols, symbol{Name: n.Name, Line: n.Line, Column: n.Column})
		}
	}

	// Sort by line.
	for i := 0; i < len(symbols); i++ {
		for j := i + 1; j < len(symbols); j++ {
			if symbols[i].Line > symbols[j].Line {
				symbols[i], symbols[j] = symbols[j], symbols[i]
			}
		}
	}

	return map[string]interface{}{
		"file":    p.File,
		"count":   len(symbols),
		"symbols": symbols,
	}, nil
}

// =============================================================================
// JSON-RPC helpers
// =============================================================================

func (s *mcpServer) sendResult(id interface{}, result interface{}) {
	resp := rpcResponse{
		JSONRPC: "2.0",
		ID:      id,
		Result:  result,
	}
	data, _ := json.Marshal(resp)
	fmt.Println(string(data))
}

func (s *mcpServer) sendError(id interface{}, code int, message string) {
	resp := rpcResponse{
		JSONRPC: "2.0",
		ID:      id,
		Error:   &rpcError{Code: code, Message: message},
	}
	data, _ := json.Marshal(resp)
	fmt.Println(string(data))
}

func toJSON(v interface{}) string {
	data, _ := json.MarshalIndent(v, "", "  ")
	return string(data)
}
