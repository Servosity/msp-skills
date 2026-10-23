module fixture

go 1.25

require (
	github.com/mark3labs/mcp-go v0.0.0
	github.com/spf13/cobra v1.9.1
)

require (
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/spf13/pflag v1.0.6 // indirect
)

// A local stand-in for the MCP request type (see mcpgostub/mcp).
replace github.com/mark3labs/mcp-go => ./mcpgostub
