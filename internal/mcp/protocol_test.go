package mcp_test

import (
	"context"
	"testing"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/freemanjava/ha-explorer-mcp/internal/mcp"
)

// TestSDKProtocolVersion connects an in-memory client and server using the
// pinned SDK and asserts the negotiated protocol version is the one this
// project expects. A future SDK bump that changes the wire version fails this
// test instead of silently shipping a mismatched server.
func TestSDKProtocolVersion(t *testing.T) {
	ctx := context.Background()

	server := sdkmcp.NewServer(&sdkmcp.Implementation{Name: "ha-inspector-mcp-test", Version: "0.0.0"}, nil)
	client := sdkmcp.NewClient(&sdkmcp.Implementation{Name: "ha-inspector-mcp-test-client", Version: "0.0.0"}, nil)

	serverTransport, clientTransport := sdkmcp.NewInMemoryTransports()

	serverSession, err := server.Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatalf("server.Connect: %v", err)
	}
	defer serverSession.Close()

	clientSession, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("client.Connect: %v", err)
	}
	defer clientSession.Close()

	got := clientSession.InitializeResult().ProtocolVersion
	if got != mcp.SupportedProtocolVersion {
		t.Fatalf("SDK negotiated protocol version %q, want %q — an SDK bump changed the wire version; review the change before updating SupportedProtocolVersion", got, mcp.SupportedProtocolVersion)
	}
}
