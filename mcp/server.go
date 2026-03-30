package mcp

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"sync"
)

// Tool defines an MCP tool.
type Tool struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	InputSchema map[string]interface{} `json:"inputSchema"`
}

// ContentBlock represents a content block in a tool result.
type ContentBlock struct {
	Type     string `json:"type"`               // "text" or "image"
	Text     string `json:"text,omitempty"`      // for text blocks
	Data     string `json:"data,omitempty"`      // base64 for images
	MimeType string `json:"mimeType,omitempty"`  // for images
}

// ToolResult is the result of executing a tool.
type ToolResult struct {
	Content []ContentBlock `json:"content"`
	IsError bool           `json:"isError,omitempty"`
}

// ToolHandler is the function signature for tool implementations.
type ToolHandler func(args map[string]interface{}) ToolResult

// jsonRPCRequest is an incoming JSON-RPC request.
type jsonRPCRequest struct {
	JSONRPC string                 `json:"jsonrpc"`
	ID      *json.RawMessage       `json:"id,omitempty"` // nil for notifications
	Method  string                 `json:"method"`
	Params  map[string]interface{} `json:"params,omitempty"`
}

// jsonRPCResponse is an outgoing JSON-RPC response.
type jsonRPCResponse struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      interface{} `json:"id"`
	Result  interface{} `json:"result,omitempty"`
	Error   interface{} `json:"error,omitempty"`
}

// Server is the MCP protocol server.
type Server struct {
	tools    map[string]Tool
	handlers map[string]ToolHandler
	mu       sync.RWMutex
	writer   *json.Encoder
	writerMu sync.Mutex
}

// NewServer creates a new MCP server.
func NewServer() *Server {
	return &Server{
		tools:    make(map[string]Tool),
		handlers: make(map[string]ToolHandler),
	}
}

// RegisterTool registers a tool with its handler.
func (s *Server) RegisterTool(tool Tool, handler ToolHandler) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tools[tool.Name] = tool
	s.handlers[tool.Name] = handler
}

// Run starts the MCP server on stdio.
func (s *Server) Run() {
	log.SetOutput(os.Stderr)
	log.SetPrefix("[cdp-mcp] ")
	log.Printf("MCP server starting on stdio")

	s.writer = json.NewEncoder(os.Stdout)

	scanner := bufio.NewScanner(os.Stdin)
	// Allow up to 10MB messages
	scanner.Buffer(make([]byte, 0, 10*1024*1024), 10*1024*1024)

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var req jsonRPCRequest
		if err := json.Unmarshal(line, &req); err != nil {
			log.Printf("Failed to parse request: %v", err)
			continue
		}

		s.handleRequest(req)
	}

	if err := scanner.Err(); err != nil && err != io.EOF {
		log.Printf("Scanner error: %v", err)
	}

	log.Printf("MCP server shutting down (stdin closed)")
}

func (s *Server) handleRequest(req jsonRPCRequest) {
	switch req.Method {
	case "initialize":
		s.respond(req.ID, map[string]interface{}{
			"protocolVersion": "2024-11-05",
			"capabilities": map[string]interface{}{
				"tools": map[string]interface{}{},
			},
			"serverInfo": map[string]interface{}{
				"name":    "cdp-mcp",
				"version": "1.0.0",
			},
		})

	case "notifications/initialized":
		log.Printf("Client initialized")
		// No response for notifications

	case "tools/list":
		s.mu.RLock()
		toolList := make([]Tool, 0, len(s.tools))
		for _, t := range s.tools {
			toolList = append(toolList, t)
		}
		s.mu.RUnlock()
		s.respond(req.ID, map[string]interface{}{
			"tools": toolList,
		})

	case "tools/call":
		name, _ := req.Params["name"].(string)
		args, _ := req.Params["arguments"].(map[string]interface{})
		if args == nil {
			args = make(map[string]interface{})
		}

		s.mu.RLock()
		handler, ok := s.handlers[name]
		s.mu.RUnlock()

		if !ok {
			s.respondError(req.ID, -32601, fmt.Sprintf("Unknown tool: %s", name))
			return
		}

		log.Printf("Calling tool: %s with args: %v", name, args)
		result := handler(args)
		s.respond(req.ID, result)

	case "ping":
		s.respond(req.ID, map[string]interface{}{})

	default:
		if req.ID != nil {
			s.respondError(req.ID, -32601, fmt.Sprintf("Method not found: %s", req.Method))
		}
	}
}

func (s *Server) respond(id *json.RawMessage, result interface{}) {
	if id == nil {
		return
	}
	resp := jsonRPCResponse{
		JSONRPC: "2.0",
		ID:      id,
		Result:  result,
	}
	s.writerMu.Lock()
	defer s.writerMu.Unlock()
	if err := s.writer.Encode(resp); err != nil {
		log.Printf("Failed to write response: %v", err)
	}
}

func (s *Server) respondError(id *json.RawMessage, code int, message string) {
	if id == nil {
		return
	}
	resp := jsonRPCResponse{
		JSONRPC: "2.0",
		ID:      id,
		Error: map[string]interface{}{
			"code":    code,
			"message": message,
		},
	}
	s.writerMu.Lock()
	defer s.writerMu.Unlock()
	if err := s.writer.Encode(resp); err != nil {
		log.Printf("Failed to write error response: %v", err)
	}
}

// TextResult is a helper to create a text-only ToolResult.
func TextResult(text string) ToolResult {
	return ToolResult{
		Content: []ContentBlock{
			{Type: "text", Text: text},
		},
	}
}

// ErrorResult is a helper to create an error ToolResult.
func ErrorResult(text string) ToolResult {
	return ToolResult{
		Content: []ContentBlock{
			{Type: "text", Text: text},
		},
		IsError: true,
	}
}

// ImageResult creates a ToolResult with an image block.
func ImageResult(base64Data, mimeType string) ToolResult {
	return ToolResult{
		Content: []ContentBlock{
			{Type: "image", Data: base64Data, MimeType: mimeType},
		},
	}
}

// MixedResult creates a ToolResult with both text and image.
func MixedResult(text, base64Data, mimeType string) ToolResult {
	return ToolResult{
		Content: []ContentBlock{
			{Type: "text", Text: text},
			{Type: "image", Data: base64Data, MimeType: mimeType},
		},
	}
}
