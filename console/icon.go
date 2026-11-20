package console

import (
	"context"
	"encoding/base64"
	"net/http"
	"slices"
	"strings"
)

var iconTypes = []string{"image/png", "image/jpeg", "image/gif", "image/webp", "image/x-icon", "image/vnd.microsoft.icon"}

type icon struct {
	body []byte
	kind string
}

// FetchIcon loads an untrusted icon as image bytes: https or a data URI, at most 512 KiB, and only PNG, JPEG, GIF, WebP or ICO by content sniffing; SVG is refused.
func FetchIcon(ctx context.Context, raw string) ([]byte, string, error) {
	var body []byte
	if rest, ok := strings.CutPrefix(raw, "data:"); ok {
		meta, data, found := strings.Cut(rest, ",")
		if !found || !strings.HasSuffix(meta, ";base64") {
			return nil, "", invalid("only base64 data URIs are read")
		}
		var err error
		if body, err = base64.StdEncoding.DecodeString(data); err != nil {
			return nil, "", err
		}
		if len(body) > maxIconBytes {
			return nil, "", ErrTooLarge
		}
	} else {
		var err error
		if body, _, err = (Reach{}).Get(ctx, raw, maxIconBytes, "image/png, image/webp, image/jpeg, image/gif, image/x-icon"); err != nil {
			return nil, "", err
		}
	}
	kind := http.DetectContentType(body)
	if !slices.Contains(iconTypes, kind) {
		return nil, "", invalid("the icon is not a PNG, JPEG, GIF, WebP or ICO image")
	}
	return body, kind, nil
}

func (c *Console) icon(w http.ResponseWriter, r *http.Request) {
	raw := r.URL.Query().Get("u")
	c.iconMu.Lock()
	ic, ok := c.icons[raw]
	c.iconMu.Unlock()
	if !ok {
		body, kind, err := FetchIcon(r.Context(), raw)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		ic = icon{body: body, kind: kind}
		c.iconMu.Lock()
		if len(c.icons) >= maxIconCache {
			clear(c.icons)
		}
		c.icons[raw] = ic
		c.iconMu.Unlock()
	}
	h := w.Header()
	h.Set("Content-Type", ic.kind)
	h.Set("Content-Security-Policy", "default-src 'none'; sandbox")
	h.Set("Content-Disposition", "inline; filename=icon")
	h.Set("Cache-Control", "private, max-age=3600")
	_, _ = w.Write(ic.body)
}
