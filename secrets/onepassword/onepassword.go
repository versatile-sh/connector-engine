// Package onepassword is a SecretStore whose root key rests wrapped by an unlock key read from a 1Password item through a 1Password Connect server.
package onepassword

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/versatile-sh/connector-engine/port"
	"github.com/versatile-sh/connector-engine/secrets/local"
)

// Config names the Connect server, its access token and the item field that holds the 64 hexadecimal characters of the unlock key.
type Config struct {
	ConnectURL string
	Token      string
	Vault      string
	Item       string
	Field      string
}

var id = regexp.MustCompile(`^[a-zA-Z0-9]{1,64}$`)

// Open reads the unlock key from 1Password and opens the key store in dir with it; a plaintext root key found there is migrated under it.
func Open(ctx context.Context, dir string, c Config, client *http.Client) (*local.Store, error) {
	key, err := fetch(ctx, c, client)
	if err != nil {
		return nil, err
	}
	s, err := local.OpenWrapped(dir, key)
	if err != nil {
		return nil, err
	}
	if s.Form() == local.Plaintext {
		if err := s.Migrate(key); err != nil {
			return nil, err
		}
	}
	return s, nil
}

func fetch(ctx context.Context, c Config, client *http.Client) ([]byte, error) {
	base, err := url.Parse(strings.TrimRight(c.ConnectURL, "/"))
	if err != nil || (base.Scheme != "https" && base.Scheme != "http") || base.Host == "" {
		return nil, port.Errorf(port.CodeInvalidArgument, "the 1Password Connect URL is an http or https address")
	}
	if !id.MatchString(c.Vault) || !id.MatchString(c.Item) || c.Field == "" || c.Token == "" {
		return nil, port.Errorf(port.CodeInvalidArgument, "1Password needs a token, a vault id, an item id and a field label")
	}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base.JoinPath("v1", "vaults", c.Vault, "items", c.Item).String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)
	res, err := client.Do(req)
	if err != nil {
		return nil, port.Errorf(port.CodeUnavailable, "1Password Connect: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, port.Errorf(port.CodeUnavailable, "1Password Connect answered %d for the item", res.StatusCode)
	}
	var item struct {
		Fields []struct {
			ID    string `json:"id"`
			Label string `json:"label"`
			Value string `json:"value"`
		} `json:"fields"`
	}
	if err := json.NewDecoder(io.LimitReader(res.Body, 1<<20)).Decode(&item); err != nil {
		return nil, port.Errorf(port.CodeUnavailable, "1Password Connect: %v", err)
	}
	for _, f := range item.Fields {
		if f.Label == c.Field || f.ID == c.Field {
			key, err := local.ParseUnlockKey(f.Value)
			if err != nil {
				return nil, port.Errorf(port.CodeInvalidArgument, "the 1Password field %q is not 64 hexadecimal characters", c.Field)
			}
			return key, nil
		}
	}
	return nil, port.Errorf(port.CodeNotFound, "the 1Password item has no field %q", c.Field)
}
