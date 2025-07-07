package extractor

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
)

func StartGopls(rootPath string) (io.WriteCloser, io.Reader, error) {
	cmd := exec.Command("gopls")

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, nil, err
	}
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		return nil, nil, err
	}

	// Send initialize request
	initReq := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "initialize",
		"params": map[string]interface{}{
			"rootUri":      "file://" + rootPath,
			"capabilities": map[string]interface{}{},
		},
	}
	sendLSPMessage(stdin, initReq)

	return stdin, stdout, nil
}

func sendLSPMessage(w io.Writer, msg interface{}) {
	data, _ := json.Marshal(msg)
	fmt.Fprintf(w, "Content-Length: %d\r\n\r\n%s", len(data), data)
}

func (c *GoplsClient) Close() error {
	if closer, ok := c.stdin.(io.Closer); ok {
		return closer.Close()
	}
	return nil
}

func readLSPMessages(r io.Reader) {
	scanner := bufio.NewScanner(r)
	scanner.Split(splitLSP)
	for scanner.Scan() {
		fmt.Println("LSP Response:\n", scanner.Text())
	}
}

func splitLSP(data []byte, atEOF bool) (int, []byte, error) {
	const sep = "\r\n\r\n"
	i := strings.Index(string(data), sep)
	if i == -1 {
		return 0, nil, nil
	}
	var length int
	fmt.Sscanf(string(data[:i]), "Content-Length: %d", &length)
	start := i + len(sep)
	end := start + length
	if len(data) < end {
		return 0, nil, nil
	}
	return end, data[start:end], nil
}
