package logging

import (
	"log"

	"local-imap-mcp/internal/config"
)

func Startup(cfg *config.Config) {
	log.Printf("starting local-imap-mcp http=%s mcp_path=%s imap=%s secure=%t default_mailbox=%s max_results=%d read_only=%t",
		cfg.HTTPAddr(),
		cfg.Server.MCPPath,
		cfg.IMAPAddr(),
		cfg.IMAP.Secure,
		cfg.IMAP.DefaultMailbox,
		cfg.IMAP.MaxResults,
		cfg.Safety.ReadOnly,
	)
}
