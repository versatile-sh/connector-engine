// Command testmcp is a tiny MCP server for the packaging check: four tools, server instructions, two prompts, a resource and a resource template.
package main

import (
	"flag"
	"log"
	"net/http"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/agents-control/connector-engine/console/testmcp/notes"
)

func main() {
	listen := flag.String("listen", "0.0.0.0:7999", "address to listen on")
	flag.Parse()
	srv := notes.New(notes.Options{})
	h := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return srv }, nil)
	log.Fatal((&http.Server{Addr: *listen, Handler: h, ReadHeaderTimeout: 10 * time.Second}).ListenAndServe())
}
