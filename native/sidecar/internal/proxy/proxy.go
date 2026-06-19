package proxy

import (
	"net/http"
	"net/http/httputil"
	"net/url"
	"sync"
)

type TargetResolver func(*http.Request) (string, bool)

type Proxy struct {
	target   *url.URL
	resolver TargetResolver
	reverse  *httputil.ReverseProxy
	mu       sync.RWMutex
	lastErr  string
}

func New(target string) (*Proxy, error) {
	targetURL, err := url.Parse(target)
	if err != nil {
		return nil, err
	}

	proxy := &Proxy{
		target: targetURL,
	}
	reverse := httputil.NewSingleHostReverseProxy(targetURL)
	reverse.Director = func(req *http.Request) {
		nextTarget := proxy.targetForRequest(req)
		req.URL.Scheme = nextTarget.Scheme
		req.URL.Host = nextTarget.Host
		req.Host = nextTarget.Host
	}
	reverse.ErrorHandler = func(w http.ResponseWriter, _ *http.Request, err error) {
		proxy.recordError(err)
		http.Error(w, "upstream litert-lm server unavailable", http.StatusBadGateway)
	}
	proxy.reverse = reverse

	return proxy, nil
}

func (p *Proxy) SetTarget(target string) error {
	targetURL, err := url.Parse(target)
	if err != nil {
		return err
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	p.target = targetURL
	p.lastErr = ""
	return nil
}

func (p *Proxy) SetTargetResolver(resolver TargetResolver) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.resolver = resolver
	p.lastErr = ""
}

func (p *Proxy) Target() string {
	target := p.targetSnapshot()
	return target.String()
}

func (p *Proxy) TargetForPath(path string) string {
	target := p.targetForRequest(&http.Request{URL: &url.URL{Path: path}})
	return target.String()
}

func (p *Proxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	p.reverse.ServeHTTP(w, r)
}

func (p *Proxy) LastError() string {
	p.mu.RLock()
	defer p.mu.RUnlock()

	return p.lastErr
}

func (p *Proxy) targetSnapshot() *url.URL {
	p.mu.RLock()
	defer p.mu.RUnlock()

	target := *p.target
	return &target
}

func (p *Proxy) targetForRequest(r *http.Request) *url.URL {
	p.mu.RLock()
	resolver := p.resolver
	fallback := *p.target
	p.mu.RUnlock()

	if resolver == nil {
		return &fallback
	}

	target, ok := resolver(r)
	if !ok || target == "" {
		return &fallback
	}

	targetURL, err := url.Parse(target)
	if err != nil {
		p.recordError(err)
		return &fallback
	}

	return targetURL
}

func (p *Proxy) recordError(err error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.lastErr = err.Error()
}
