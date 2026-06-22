package server

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync/atomic"

	"g0litellama/internal/tui/store"
)

// ObservingRoundTripper wraps an http.RoundTripper and dispatches proxy
// observation actions into a store.CommandBus for every proxied request and
// response. Dispatch is synchronous so replay order matches proxy stream order.
type ObservingRoundTripper struct {
	inner      http.RoundTripper
	commandBus *store.CommandBus
}

// NewObservingRoundTripper returns a RoundTripper that observes all requests
// and responses passing through inner. When commandBus is nil the observer
// is a no-op pass-through.
func NewObservingRoundTripper(inner http.RoundTripper, commandBus *store.CommandBus) *ObservingRoundTripper {
	return &ObservingRoundTripper{inner: inner, commandBus: commandBus}
}

// RoundTrip dispatches a request-start observation, forwards the request to
// the inner transport, and wraps the response body to capture streaming chunks.
func (o *ObservingRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) {
	if o.commandBus == nil || o.inner == nil {
		if o.inner != nil {
			return o.inner.RoundTrip(r)
		}
		return http.DefaultTransport.RoundTrip(r)
	}

	cid := newCorrelationID()
	role := roleForProxyPath(r.URL.Path)

	o.dispatchRequestStart(r, cid, role)

	resp, err := o.inner.RoundTrip(r)
	if err != nil {
		o.dispatchError(cid, err)
		return resp, err
	}

	// Wrap the response body to capture chunks as they stream through.
	if resp.Body != nil {
		resp.Body = &observingBody{
			body:          resp.Body,
			commandBus:    o.commandBus,
			correlationID: cid,
			statusCode:    resp.StatusCode,
			contentType:   resp.Header.Get("Content-Type"),
		}
	}
	return resp, nil
}

func (o *ObservingRoundTripper) dispatchRequestStart(r *http.Request, cid store.ActionID, role string) {
	payload := store.ProxyRequestStartPayload{
		Method: r.Method,
		Path:   r.URL.Path,
		Role:   role,
	}
	env := store.ActionEnvelope{
		Type:          store.ActionTypeProxyRequestStart,
		Source:        store.SourceOpenAI,
		CorrelationID: cid,
		Payload:       mustMarshal(payload),
	}
	_, _ = o.commandBus.Dispatch(context.Background(), env)
}

func (o *ObservingRoundTripper) dispatchError(cid store.ActionID, err error) {
	payload := store.ProxyResponseErrorPayload{
		CorrelationID: string(cid),
		Error:         err.Error(),
	}
	env := store.ActionEnvelope{
		Type:          store.ActionTypeProxyResponseError,
		Source:        store.SourceOpenAI,
		CorrelationID: cid,
		Payload:       mustMarshal(payload),
	}
	_, _ = o.commandBus.Dispatch(context.Background(), env)
}

// observingBody wraps an io.ReadCloser and dispatches a response-chunk action
// for each successful Read call. On Close it dispatches response-end.
type observingBody struct {
	body          io.ReadCloser
	commandBus    *store.CommandBus
	correlationID store.ActionID
	statusCode    int
	contentType   string
	chunkIndex    atomic.Int64
	closed        atomic.Bool
}

func (o *observingBody) Read(p []byte) (int, error) {
	n, err := o.body.Read(p)
	if n > 0 {
		data := make([]byte, n)
		copy(data, p[:n])
		idx := int(o.chunkIndex.Add(1) - 1)
		payload := store.ProxyResponseChunkPayload{
			CorrelationID: string(o.correlationID),
			Data:          data,
			Index:         idx,
		}
		env := store.ActionEnvelope{
			Type:          store.ActionTypeProxyResponseChunk,
			Source:        store.SourceOpenAI,
			CorrelationID: o.correlationID,
			Payload:       mustMarshal(payload),
		}
		_, _ = o.commandBus.Dispatch(context.Background(), env)
	}
	return n, err
}

func (o *observingBody) Close() error {
	err := o.body.Close()
	if o.closed.CompareAndSwap(false, true) {
		payload := store.ProxyResponseEndPayload{
			CorrelationID: string(o.correlationID),
			StatusCode:    o.statusCode,
			ContentType:   o.contentType,
		}
		env := store.ActionEnvelope{
			Type:          store.ActionTypeProxyResponseEnd,
			Source:        store.SourceOpenAI,
			CorrelationID: o.correlationID,
			Payload:       mustMarshal(payload),
		}
		_, _ = o.commandBus.Dispatch(context.Background(), env)
	}
	return err
}

// roleForProxyPath maps a /v1/* path to the pinned runner role. Mirrors the
// logic in internal/supervisor/supervisor.go roleForPath.
func roleForProxyPath(path string) string {
	switch {
	case path == "/v1/embeddings" || strings.HasPrefix(path, "/v1/embeddings/"):
		return "embedding"
	case path == "/v1/rerank" || strings.HasPrefix(path, "/v1/rerank/"):
		return "reranking"
	default:
		return "main"
	}
}

func newCorrelationID() store.ActionID {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return store.ActionID(hex.EncodeToString(b))
}

func mustMarshal(v any) json.RawMessage {
	data, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return data
}
