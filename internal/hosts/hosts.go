package hosts

import (
	"fmt"
	"log"
	"os"
	"strings"

	"xgate/internal/config"
)

const (
	hostsFile   = "/etc/hosts"
	markerBegin = "# local-router:begin"
	markerEnd   = "# local-router:end"
)

// Add rewrites the managed block in /etc/hosts to contain one entry per
// non-wildcard route. Wildcard routes are skipped — they require a
// real DNS resolver.
func Add(routes []config.Route) error {
	var hosts []string
	for _, r := range routes {
		if !strings.HasPrefix(r.Host, "*.") {
			hosts = append(hosts, r.Host)
		}
	}
	if len(hosts) == 0 {
		return nil
	}

	content, err := os.ReadFile(hostsFile)
	if err != nil {
		return fmt.Errorf("read %s: %w", hostsFile, err)
	}

	// Remove stale block if present
	cleaned := removeMarkerBlock(string(content))

	var block strings.Builder
	block.WriteString(markerBegin + "\n")
	for _, h := range hosts {
		block.WriteString(fmt.Sprintf("127.0.0.1 %s\n", h))
	}
	block.WriteString(markerEnd + "\n")

	final := strings.TrimRight(cleaned, "\n") + "\n\n" + block.String()
	if err := os.WriteFile(hostsFile, []byte(final), 0644); err != nil {
		return fmt.Errorf("write %s: %w", hostsFile, err)
	}

	log.Printf("Added %d entries to %s", len(hosts), hostsFile)
	return nil
}

// Remove strips the managed block from /etc/hosts, if present.
func Remove() error {
	content, err := os.ReadFile(hostsFile)
	if err != nil {
		return fmt.Errorf("read %s: %w", hostsFile, err)
	}

	cleaned := removeMarkerBlock(string(content))
	if cleaned == string(content) {
		return nil // nothing to remove
	}

	final := strings.TrimRight(cleaned, "\n") + "\n"
	if err := os.WriteFile(hostsFile, []byte(final), 0644); err != nil {
		return fmt.Errorf("write %s: %w", hostsFile, err)
	}

	log.Printf("Removed managed entries from %s", hostsFile)
	return nil
}

func removeMarkerBlock(content string) string {
	beginIdx := strings.Index(content, markerBegin)
	if beginIdx == -1 {
		return content
	}
	endIdx := strings.Index(content, markerEnd)
	if endIdx == -1 {
		return content
	}
	endIdx += len(markerEnd)
	if endIdx < len(content) && content[endIdx] == '\n' {
		endIdx++
	}
	// Trim one extra blank line before the block
	if beginIdx > 0 && content[beginIdx-1] == '\n' {
		beginIdx--
	}
	return content[:beginIdx] + content[endIdx:]
}
