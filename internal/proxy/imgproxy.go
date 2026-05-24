package proxy

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
)

type ImgproxyClient struct {
	baseURL string
	key     []byte
	salt    []byte
}

func NewImgproxyClient(baseURL, hexKey, hexSalt string) *ImgproxyClient {
	c := &ImgproxyClient{baseURL: strings.TrimRight(baseURL, "/")}
	if hexKey != "" {
		var err error
		c.key, err = hexDecode(hexKey)
		if err != nil {
			c.key = nil
		}
	}
	if hexSalt != "" {
		var err error
		c.salt, err = hexDecode(hexSalt)
		if err != nil {
			c.salt = nil
		}
	}
	return c
}

func hexDecode(s string) ([]byte, error) {
	if len(s)%2 != 0 {
		return nil, fmt.Errorf("invalid hex length")
	}
	b := make([]byte, len(s)/2)
	for i := 0; i < len(s); i += 2 {
		hi := hexVal(s[i])
		lo := hexVal(s[i+1])
		if hi < 0 || lo < 0 {
			return nil, fmt.Errorf("invalid hex character")
		}
		b[i/2] = byte(hi<<4 | lo)
	}
	return b, nil
}

func hexVal(c byte) int {
	switch {
	case '0' <= c && c <= '9':
		return int(c - '0')
	case 'a' <= c && c <= 'f':
		return int(c - 'a' + 10)
	case 'A' <= c && c <= 'F':
		return int(c - 'A' + 10)
	}
	return -1
}

func (c *ImgproxyClient) BuildURL(path string, query string) string {
	u := c.baseURL + "/insecure" + path
	if query != "" {
		u += "?" + query
	}
	return u
}

func (c *ImgproxyClient) BuildSignedURL(path string) string {
	segment := path
	if c.key == nil || c.salt == nil {
		return c.baseURL + "/insecure" + segment
	}
	salt := c.salt
	key := c.key
	mac := hmac.New(sha256.New, key)
	mac.Write(salt)
	mac.Write([]byte(segment))
	sig := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	return c.baseURL + "/" + sig + segment
}

func (c *ImgproxyClient) BuildProcessURL(resize, gravity, quality, format, sourceURL string) string {
	var parts []string
	if resize != "" {
		parts = append(parts, resize)
	}
	if gravity != "" {
		parts = append(parts, gravity)
	}
	if quality != "" {
		parts = append(parts, quality)
	}
	if format != "" {
		parts = append(parts, format)
	}
	opts := strings.Join(parts, "/")
	sourceEncoded := base64.RawURLEncoding.EncodeToString([]byte(sourceURL))
	var path string
	if opts != "" {
		path = "/" + opts + "/" + sourceEncoded
	} else {
		path = "/" + sourceEncoded
	}
	return c.BuildSignedURL(path)
}

func (c *ImgproxyClient) ProxyTo(w http.ResponseWriter, r *http.Request) {
	target, _ := url.Parse(c.baseURL)
	proxy := httputil.NewSingleHostReverseProxy(target)
	proxy.ServeHTTP(w, r)
}

func (c *ImgproxyClient) Fetch(urlStr string, w http.ResponseWriter) error {
	resp, err := http.Get(urlStr)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	for k, vv := range resp.Header {
		for _, v := range vv {
			w.Header().Add(k, v)
		}
	}
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)
	return nil
}

func (c *ImgproxyClient) Forward(w http.ResponseWriter, r *http.Request) {
	target, _ := url.Parse(c.baseURL)
	r.URL.Scheme = target.Scheme
	r.URL.Host = target.Host
	r.Host = target.Host
	r.RequestURI = ""

	resp, err := http.DefaultTransport.RoundTrip(r)
	if err != nil {
		http.Error(w, "Bad Gateway", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()
	for k, vv := range resp.Header {
		for _, v := range vv {
			w.Header().Add(k, v)
		}
	}
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)
}
