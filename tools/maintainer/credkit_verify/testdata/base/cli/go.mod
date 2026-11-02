module acme-pp-cli

go 1.26

require (
	example.com/httpx v0.0.0
	github.com/mark3labs/mcp-go v0.0.0
	golang.org/x/oauth2 v0.0.0
)

replace (
	example.com/httpx => ./third_party/httpx
	github.com/mark3labs/mcp-go => ./third_party/mcp-go
	golang.org/x/oauth2 => ./third_party/oauth2
)
