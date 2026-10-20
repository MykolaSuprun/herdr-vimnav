// osc-theme-probe: does the hosting terminal (and herdr inside it) answer the
// OSC 10/11 color queries, and does a herdr PANE see herdr's cached colors?
//
// Usage:
//
//	go run ./tools/osc-theme-probe        # query stdin/stdout directly
//
// Reads replies with a short deadline so it also works in non-interactive
// contexts (where it just reports "no reply").
package main

import (
	"bufio"
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"
)

func main() {
	// OSC 10 (fg) + OSC 11 (bg) + DA1 sentinel; ST-terminated like herdr does.
	query := "\x1b]10;?\x1b\\\x1b]11;?\x1b\\"
	if _, err := os.Stdout.WriteString(query); err != nil {
		fmt.Fprintln(os.Stderr, "write error:", err)
		os.Exit(1)
	}

	re := regexp.MustCompile(`\x1b\](10|11);rgb:([0-9a-fA-F]{2,4})/([0-9a-fA-F]{2,4})/([0-9a-fA-F]{2,4})\x1b\\`)
	deadline := time.Now().Add(700 * time.Millisecond)
	_ = os.Stdin.SetReadDeadline(deadline)

	r := bufio.NewReader(os.Stdin)
	var buf strings.Builder
	for time.Now().Before(deadline) {
		b, err := r.ReadByte()
		if err != nil {
			break
		}
		buf.WriteByte(b)
		if strings.HasSuffix(buf.String(), "\x1b\\") {
			// keep reading a moment for the second reply
			if time.Now().After(deadline.Add(-400 * time.Millisecond)) {
				break
			}
		}
	}

	got := re.FindAllStringSubmatch(buf.String(), -1)
	if len(got) == 0 {
		fmt.Println("NO REPLY (no OSC 10/11 answer within deadline)")
		fmt.Printf("raw (escaped): %q\n", buf.String())
		return
	}
	for _, m := range got {
		what := map[string]string{"10": "foreground", "11": "background"}[m[1]]
		fmt.Printf("%s = #%s%s%s (rgb:%s/%s/%s)\n", what, m[2], m[3], m[4], m[2], m[3], m[4])
	}
}
