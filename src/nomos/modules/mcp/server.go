package mcp

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"

	"github.com/mgantlett/nomos-commons/src/nomos/core/workspace"
)

type jsonRpcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
	ID      interface{}     `json:"id,omitempty"`
}

type jsonRpcResponse struct {
	JSONRPC string        `json:"jsonrpc"`
	Result  interface{}   `json:"result,omitempty"`
	Error   *jsonRpcError `json:"error,omitempty"`
	ID      interface{}   `json:"id"`
}

type jsonRpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type McpTool struct {
	Name        string      `json:"name"`
	Description string      `json:"description"`
	InputSchema interface{} `json:"inputSchema"`
}

type mcpToolListResult struct {
	Tools []McpTool `json:"tools"`
}

type mcpCallResultContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type mcpCallResult struct {
	Content []mcpCallResultContent `json:"content"`
	IsError bool                   `json:"isError,omitempty"`
}

type Server struct {
	ctx   *workspace.WorkspaceContext
	in    io.Reader
	out   io.Writer
	tools map[string]ToolHandler
}

type ToolHandler interface {
	Definition() McpTool
	Handle(ctx *workspace.WorkspaceContext, args json.RawMessage) (string, error)
}

func NewServer(ctx *workspace.WorkspaceContext, in io.Reader, out io.Writer) *Server {
	return &Server{
		ctx:   ctx,
		in:    in,
		out:   out,
		tools: make(map[string]ToolHandler),
	}
}

func (s *Server) RegisterTool(handler ToolHandler) {
	s.tools[handler.Definition().Name] = handler
}

func (s *Server) Run() error {
	scanner := bufio.NewScanner(s.in)
	// Optionally increase scanner buffer size for large payloads
	buf := make([]byte, 1024*1024)
	scanner.Buffer(buf, 10*1024*1024)

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var req jsonRpcRequest
		if err := json.Unmarshal(line, &req); err != nil {
			s.sendError(nil, -32700, "Parse error")
			continue
		}

		s.handleRequest(&req)
	}

	return scanner.Err()
}

func (s *Server) sendError(id interface{}, code int, message string) {
	resp := jsonRpcResponse{
		JSONRPC: "2.0",
		ID:      id,
		Error: &jsonRpcError{
			Code:    code,
			Message: message,
		},
	}
	bytes, _ := json.Marshal(resp)
	fmt.Fprintln(s.out, string(bytes))
}

func (s *Server) sendResult(id interface{}, result interface{}) {
	resp := jsonRpcResponse{
		JSONRPC: "2.0",
		ID:      id,
		Result:  result,
	}
	bytes, _ := json.Marshal(resp)
	fmt.Fprintln(s.out, string(bytes))
}

func (s *Server) handleRequest(req *jsonRpcRequest) {
	switch req.Method {
	case "initialize":
		result := map[string]interface{}{
			"protocolVersion": "2024-11-05",
			"capabilities":    map[string]interface{}{},
			"serverInfo": map[string]string{
				"name":    "nomos-mcp",
				"version": "1.0.0",
			},
		}
		s.sendResult(req.ID, result)

	case "tools/list":
		var tools []McpTool
		for _, handler := range s.tools {
			tools = append(tools, handler.Definition())
		}
		s.sendResult(req.ID, mcpToolListResult{Tools: tools})

	case "tools/call":
		var callArgs struct {
			Name      string          `json:"name"`
			Arguments json.RawMessage `json:"arguments"`
		}
		if err := json.Unmarshal(req.Params, &callArgs); err != nil {
			s.sendError(req.ID, -32602, "Invalid params")
			return
		}

		s.handleToolCall(req.ID, callArgs.Name, callArgs.Arguments)

	default:
		s.sendError(req.ID, -32601, "Method not found")
	}
}

func (s *Server) handleToolCall(id interface{}, name string, args json.RawMessage) {
	handler, exists := s.tools[name]
	if !exists {
		s.sendError(id, -32601, "Tool not found")
		return
	}

	text, err := handler.Handle(s.ctx, args)
	if err != nil {
		s.sendToolCallError(id, err.Error())
		return
	}
	s.sendToolCallResult(id, text)
}

func (s *Server) sendToolCallResult(id interface{}, text string) {
	s.sendToolCallResultContent(id, text, false)
}

func (s *Server) sendToolCallError(id interface{}, text string) {
	s.sendToolCallResultContent(id, text, true)
}

func (s *Server) sendToolCallResultContent(id interface{}, text string, isError bool) {
	res := mcpCallResult{
		Content: []mcpCallResultContent{
			{
				Type: "text",
				Text: text,
			},
		},
		IsError: isError,
	}
	s.sendResult(id, res)
}
