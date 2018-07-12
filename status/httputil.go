package status

import (
	"bytes"
	"encoding/base64"
	"net/http"
	p "path"
	"strings"
)

var (
	basicAuthPassword = []byte(":api_token")
)

func Post(path, body string) (*http.Response, error) {
	// Create HTTP request
	req, err := http.NewRequest("POST",
		p.Join("https://www.toggl.com/api/v8/", path),
		strings.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	// Add basic auth header
	apiToken := []byte{} // TODO fill this in (read from env or file?)
	buf := bytes.NewBuffer([]byte("Basic "))
	base64.NewEncoder(base64.URLEncoding, buf).Write(apiToken)
	base64.NewEncoder(base64.URLEncoding, buf).Write(basicAuthPassword)
	req.Header.Set("Authorization", buf.String())
	return http.DefaultClient.Do(req)
}
