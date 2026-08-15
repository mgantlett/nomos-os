package cmd

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/mgantlett/nomos-commons/src/nomos/core/workspace"

	"github.com/mgantlett/nomos-commons/src/nomos/core/ast"
	"github.com/mgantlett/nomos-os/src/nomos/modules/verify"
	"github.com/spf13/cobra"
)

var mcpCmd = &cobra.Command{
	Use:   "mcp",
	Short: "Start a Go-native Model Context Protocol (MCP) server over stdin/stdout",
	RunE: func(cmd *cobra.Command, args []string) error {
		wd, err := os.Getwd()
		if err != nil {
			return err
		}
		repoRoot := findRepoRoot(wd)

		// Disable printing log messages to stdout to prevent corrupting JSON-RPC stream
		os.Stdout = os.NewFile(1, "/dev/stdout")

		return runMcpServer(func() *workspace.WorkspaceContext { c, _ := workspace.NewContext(repoRoot); return c }(), os.Stdin, os.Stdout)
	},
}

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

type mcpTool struct {
	Name        string      `json:"name"`
	Description string      `json:"description"`
	InputSchema interface{} `json:"inputSchema"`
}

type mcpToolListResult struct {
	Tools []mcpTool `json:"tools"`
}

type mcpCallResultContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type mcpCallResult struct {
	Content []mcpCallResultContent `json:"content"`
	IsError bool                   `json:"isError,omitempty"`
}

func runMcpServer(ctx *workspace.WorkspaceContext, r io.Reader, w io.Writer) error {
	repoRoot := ctx.RepoRoot
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var req jsonRpcRequest
		if err := json.Unmarshal(line, &req); err != nil {
			sendError(w, nil, -32700, "Parse error")
			continue
		}

		handleRequest(repoRoot, w, &req)
	}

	return scanner.Err()
}

func sendError(w io.Writer, id interface{}, code int, message string) {
	resp := jsonRpcResponse{
		JSONRPC: "2.0",
		ID:      id,
		Error: &jsonRpcError{
			Code:    code,
			Message: message,
		},
	}
	bytes, _ := json.Marshal(resp)
	fmt.Fprintln(w, string(bytes))
}

func sendResult(w io.Writer, id interface{}, result interface{}) {
	resp := jsonRpcResponse{
		JSONRPC: "2.0",
		ID:      id,
		Result:  result,
	}
	bytes, _ := json.Marshal(resp)
	fmt.Fprintln(w, string(bytes))
}

func handleRequest(repoRoot string, w io.Writer, req *jsonRpcRequest) {
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
		sendResult(w, req.ID, result)

	case "tools/list":
		astSchema := map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"filePath": map[string]string{"type": "string"},
			},
			"required": []string{"filePath"},
		}
		emptySchema := map[string]interface{}{
			"type": "object",
		}

		tools := []mcpTool{
			{
				Name:        "parse_ast",
				Description: "Perform AST symbol extraction on Go/Python/JS/TS source file",
				InputSchema: astSchema,
			},
			{
				Name:        "verify_dod",
				Description: "Execute the Go-native concurrent Definition of Done (DoD) verification checks",
				InputSchema: emptySchema,
			},
		}
		sendResult(w, req.ID, mcpToolListResult{Tools: tools})

	case "tools/call":
		var callArgs struct {
			Name      string          `json:"name"`
			Arguments json.RawMessage `json:"arguments"`
		}
		if err := json.Unmarshal(req.Params, &callArgs); err != nil {
			sendError(w, req.ID, -32602, "Invalid params")
			return
		}

		handleToolCall(repoRoot, w, req.ID, callArgs.Name, callArgs.Arguments)

	default:
		sendError(w, req.ID, -32601, "Method not found")
	}
}

func handleToolCall(repoRoot string, w io.Writer, id interface{}, name string, args json.RawMessage) {
	switch name {

	case "parse_ast":
		var p struct {
			FilePath string `json:"filePath"`
		}
		_ = json.Unmarshal(args, &p)
		absPath := filepath.Join(repoRoot, p.FilePath)
		res, err := ast.ParseFile(absPath)
		if err != nil {
			sendToolCallError(w, id, err.Error())
			return
		}

		bytes, _ := json.MarshalIndent(res.Symbols, "", "  ")
		sendToolCallResult(w, id, string(bytes))

	case "verify_dod":
		err := verify.VerifyDoD(func() *workspace.WorkspaceContext { c, _ := workspace.NewContext(repoRoot); return c }())
		if err != nil {
			sendToolCallResultContent(w, id, fmt.Sprintf("❌ Definition of Done verification failed: %v", err), true)
			return
		}
		sendToolCallResult(w, id, "✅ Definition of Done verification succeeded!")

	default:
		sendError(w, id, -32601, "Tool not found")
	}
}

func sendToolCallResult(w io.Writer, id interface{}, text string) {
	sendToolCallResultContent(w, id, text, false)
}

func sendToolCallError(w io.Writer, id interface{}, text string) {
	sendToolCallResultContent(w, id, text, true)
}

func sendToolCallResultContent(w io.Writer, id interface{}, text string, isError bool) {
	res := mcpCallResult{
		Content: []mcpCallResultContent{
			{
				Type: "text",
				Text: text,
			},
		},
		IsError: isError,
	}
	sendResult(w, id, res)
}

func init() {
	RootCmd.AddCommand(mcpCmd)
}
