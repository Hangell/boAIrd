package api

import (
	"encoding/hex"
	"io"
	"net/http"
	"net/http/httptest"
)

const testLocalAuthToken = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

func authorizedTestRequest(method, target string, body io.Reader) *http.Request {
	request := httptest.NewRequest(method, target, body)
	request.Header.Set("Authorization", "Bearer "+testLocalAuthToken)
	return request
}

func testLocalAuthTokenBytes() []byte {
	token, _ := hex.DecodeString(testLocalAuthToken)
	return token
}
