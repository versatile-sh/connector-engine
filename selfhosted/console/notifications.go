package console

import (
	"context"
	"fmt"
	"log/slog"
	"maps"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/versatile-sh/connector-engine/selfhosted/surface"
)

const (
	maxLabel          = 60
	defaultDeviceName = "This computer"
)

var (
	digits    = regexp.MustCompile(`^[0-9]{1,24}$`)
	slackUser = regexp.MustCompile(`^[UW][A-Z0-9]{2,20}$`)
)

type kindSpec struct {
	required []string
	optional []string
	secrets  []string
	owner    *regexp.Regexp
	hosts    []string
}

var kinds = map[string]kindSpec{
	"discord":         {required: []string{"token", "owner_user_id"}, secrets: []string{"token"}, owner: digits},
	"slack":           {required: []string{"app_token", "bot_token", "owner_user_id"}, secrets: []string{"bot_token", "app_token"}, owner: slackUser},
	"telegram":        {required: []string{"token", "owner_user_id"}, secrets: []string{"token"}, owner: digits},
	"ntfy":            {required: []string{"url"}, optional: []string{"token"}, secrets: []string{"url", "token"}},
	"discord_webhook": {required: []string{"url"}, secrets: []string{"url"}, hosts: []string{"discord.com", "discordapp.com"}},
	"slack_webhook":   {required: []string{"url"}, secrets: []string{"url"}, hosts: []string{"hooks.slack.com"}},
}

func secretValues(kind string, settings map[string]string) []string {
	var out []string
	for _, k := range kinds[kind].secrets {
		out = append(out, settings[k])
	}
	return out
}

func splitSecrets(kind string, all map[string]string) (plain, secret map[string]string) {
	plain, secret = map[string]string{}, map[string]string{}
	for k, v := range all {
		if slices.Contains(kinds[kind].secrets, k) {
			secret[k] = v
		} else {
			plain[k] = v
		}
	}
	return plain, secret
}

func (c *Console) secrets() channelSecrets { return channelSecrets{state: c.State, box: c.Secrets} }

func (c *Console) settingsOf(ctx context.Context, sc StoredChannel) (map[string]string, error) {
	sec, err := c.secrets().get(ctx, sc.ID)
	if err != nil {
		return nil, err
	}
	all := maps.Clone(sc.Settings)
	if all == nil {
		all = map[string]string{}
	}
	maps.Copy(all, sec)
	return all, nil
}

func checkSettings(kind string, in map[string]string) (map[string]string, error) {
	spec, ok := kinds[kind]
	if !ok {
		return nil, invalid("unknown channel kind %q", kind)
	}
	out := map[string]string{}
	for _, k := range append(slices.Clone(spec.required), spec.optional...) {
		v := strings.TrimSpace(in[k])
		if v == "" && slices.Contains(spec.required, k) {
			return nil, invalid("%s is required", k)
		}
		if strings.ContainsAny(v, " \r\n\t") || len(v) > 2048 {
			return nil, invalid("%s has spaces or is too long", k)
		}
		if v != "" {
			out[k] = v
		}
	}
	if spec.owner != nil && !spec.owner.MatchString(out["owner_user_id"]) {
		return nil, invalid("owner_user_id does not look like a %s user id", kind)
	}
	if raw, ok := out["url"]; ok {
		u, err := url.Parse(raw)
		if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil {
			return nil, invalid("the URL is an https address")
		}
		if len(spec.hosts) > 0 && !slices.Contains(spec.hosts, strings.ToLower(u.Hostname())) {
			return nil, invalid("a %s URL is on %s", kind, strings.Join(spec.hosts, " or "))
		}
	}
	return out, nil
}

// ChannelView is a configured channel as the settings page shows it; it never carries a secret.
type ChannelView struct {
	ID          string    `json:"id"`
	Kind        string    `json:"kind"`
	Label       string    `json:"label"`
	Interactive bool      `json:"interactive"`
	Owner       string    `json:"owner,omitempty"`
	SecretHint  string    `json:"secret_hint,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	Status      string    `json:"status"`
	Error       string    `json:"error,omitempty"`
}

// DeviceView is a paired host helper as the settings page shows it.
type DeviceView struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
	LastSeen  time.Time `json:"last_seen,omitzero"`
}

func (c *Console) channelView(ctx context.Context, sc StoredChannel) ChannelView {
	v := ChannelView{ID: sc.ID, Kind: sc.Kind, Label: sc.Label, Interactive: interactiveKinds[sc.Kind], Owner: sc.Settings["owner_user_id"], CreatedAt: sc.CreatedAt}
	if s := kinds[sc.Kind].secrets; len(s) > 0 {
		sec, _ := c.secrets().get(ctx, sc.ID)
		if secret := sec[s[0]]; len(secret) > 8 {
			v.SecretHint = secret[len(secret)-4:]
		}
	}
	st := c.surf.statusOf(sc.ID)
	v.Status, v.Error = st.State, st.Error
	if v.Status == "" {
		v.Status = map[bool]string{true: "connecting", false: "notify_only"}[v.Interactive]
	}
	return v
}

func (c *Console) logger() *slog.Logger {
	if c.Log == nil {
		return slog.New(slog.DiscardHandler)
	}
	return c.Log
}

func (c *Console) surfaceRoutes(rt *router) {
	rt.handle("GET /api/approvals/{id}", c.approvalPage)
	rt.handle("POST /api/approvals/{id}/decide", c.decideChoice)
	rt.handle("GET /api/notifications", c.listNotifications)
	rt.handle("POST /api/notifications/channels", c.addChannel)
	rt.handle("DELETE /api/notifications/channels/{id}", c.removeChannel)
	rt.handle("POST /api/notifications/channels/{id}/test", c.testChannel)
	rt.handle("PUT /api/notifications/apps/{name}", c.setElicit)
	rt.handle("POST /api/notifications/devices", c.pairDevice)
	rt.handle("PATCH /api/notifications/devices/{id}", c.renameDevice)
	rt.handle("DELETE /api/notifications/devices/{id}", c.revokeDevice)
	rt.handle("GET /api/notify/stream", c.notifyStream)
}

func me(r *http.Request) string {
	u, _ := signedInUser(r.Context())
	return u.Person
}

func (c *Console) listNotifications(w http.ResponseWriter, r *http.Request) {
	chans := c.ownChannels(me(r))
	slices.SortFunc(chans, func(a, b StoredChannel) int { return a.CreatedAt.Compare(b.CreatedAt) })
	views := make([]ChannelView, 0, len(chans))
	for _, sc := range chans {
		views = append(views, c.channelView(r.Context(), sc))
	}
	devices := []DeviceView{}
	for _, d := range c.State.Devices() {
		if d.Owner != me(r) {
			continue
		}
		devices = append(devices, DeviceView{ID: d.ID, Name: d.Name, CreatedAt: d.CreatedAt, LastSeen: d.LastSeen})
	}
	slices.SortFunc(devices, func(a, b DeviceView) int { return a.CreatedAt.Compare(b.CreatedAt) })
	elicit := c.State.ElicitApps()
	type app struct {
		Name   string `json:"name"`
		Elicit bool   `json:"elicit"`
	}
	apps := []app{}
	for _, a := range c.State.Agents() {
		if a.Owner == me(r) {
			apps = append(apps, app{Name: a.Name, Elicit: elicit[a.Name]})
		}
	}
	slices.SortFunc(apps, func(a, b app) int { return strings.Compare(a.Name, b.Name) })
	writeJSON(w, http.StatusOK, map[string]any{"channels": views, "devices": devices, "apps": apps})
}

func (c *Console) addChannel(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Kind     string            `json:"kind"`
		Label    string            `json:"label"`
		Settings map[string]string `json:"settings"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	settings, err := checkSettings(in.Kind, in.Settings)
	if err == nil {
		_, err = c.factory()(in.Kind, settings)
	}
	if err != nil {
		writeError(w, http.StatusBadRequest, tracked{secrets: secretValues(in.Kind, in.Settings)}.scrub(err))
		return
	}
	label := strings.TrimSpace(in.Label)
	if label == "" {
		label = in.Kind
	}
	if len([]rune(label)) > maxLabel {
		label = string([]rune(label)[:maxLabel])
	}
	plain, secret := splitSecrets(in.Kind, settings)
	sc := StoredChannel{ID: randomID()[:12], Owner: me(r), Kind: in.Kind, Label: label, Settings: plain, CreatedAt: time.Now().UTC().Truncate(time.Second)}
	if err := c.secrets().put(r.Context(), sc.ID, secret); err != nil {
		c.fail(w, err)
		return
	}
	if err := c.State.PutChannel(sc); err != nil {
		c.fail(w, err)
		return
	}
	c.surf.wake()
	writeJSON(w, http.StatusCreated, c.channelView(r.Context(), sc))
}

func (c *Console) ownChannels(person string) []StoredChannel {
	return slices.DeleteFunc(c.State.Channels(), func(sc StoredChannel) bool { return sc.Owner != person || person == "" })
}

func (c *Console) storedChannel(owner, id string) (StoredChannel, bool) {
	all := c.ownChannels(owner)
	i := slices.IndexFunc(all, func(sc StoredChannel) bool { return sc.ID == id })
	if i < 0 {
		return StoredChannel{}, false
	}
	return all[i], true
}

func (c *Console) removeChannel(w http.ResponseWriter, r *http.Request) {
	ok, err := c.State.DeleteChannel(me(r), r.PathValue("id"))
	switch {
	case err != nil:
		c.fail(w, err)
	case !ok:
		writeError(w, http.StatusNotFound, "no such channel")
	default:
		c.surf.wake()
		w.WriteHeader(http.StatusNoContent)
	}
}

func (c *Console) testChannel(w http.ResponseWriter, r *http.Request) {
	sc, ok := c.storedChannel(me(r), r.PathValue("id"))
	if !ok {
		writeError(w, http.StatusNotFound, "no such channel")
		return
	}
	settings, err := c.settingsOf(r.Context(), sc)
	if err != nil {
		c.fail(w, err)
		return
	}
	t := tracked{id: sc.ID, secrets: secretValues(sc.Kind, settings), surf: &c.surf}
	ch, err := c.factory()(sc.Kind, settings)
	if err == nil {
		ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
		defer cancel()
		p := surface.Prompt{Approval: "test", Connector: "versatile.sh", Function: "test_message", FunctionTitle: "Test message", Kind: "read", App: "console",
			ExpiresAt: time.Now().Add(10 * time.Minute), ConsoleURL: c.publicBase() + "/notifications"}
		_, err = ch.Notify(ctx, p)
	}
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "error": t.scrub(err)})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (c *Console) setElicit(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Elicit bool `json:"elicit"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	name := r.PathValue("name")
	if owner, ok := c.State.KeyOwner(name); !ok || owner != me(r) {
		writeError(w, http.StatusNotFound, "no such app")
		return
	}
	if err := c.State.SetElicit(name, in.Elicit); err != nil {
		c.fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (c *Console) pairDevice(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Name string `json:"name"`
		ID   string `json:"id"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	name, ok := deviceName(in.Name)
	if !ok {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("give the computer a name of at most %d characters", maxLabel))
		return
	}
	key := "cnk_" + randomID()
	d, again := c.State.Device(in.ID)
	if again && d.Owner != me(r) {
		writeError(w, http.StatusNotFound, "no such device")
		return
	}
	if !again {
		d = Device{ID: randomID()[:12], Owner: me(r), Name: name, CreatedAt: time.Now().UTC().Truncate(time.Second)}
	}
	d.KeyHash, d.LastSeen = keyHash(key), time.Time{}
	if err := c.State.PutDevice(d); err != nil {
		c.fail(w, err)
		return
	}
	c.surf.wake()
	cmd := fmt.Sprintf("umask 077 && mkdir -p ~/.config/versatile && printf '%%s' '%s' > ~/.config/versatile/notify.key && vsh notify --console %s", key, c.publicBase())
	status := http.StatusCreated
	if again {
		status = http.StatusOK
	}
	writeJSON(w, status, map[string]any{"device": DeviceView{ID: d.ID, Name: d.Name, CreatedAt: d.CreatedAt}, "key": key, "command": cmd})
}

func deviceName(raw string) (string, bool) {
	name := strings.TrimSpace(raw)
	if name == "" {
		name = defaultDeviceName
	}
	return name, len([]rune(name)) <= maxLabel
}

func (c *Console) renameDevice(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Name string `json:"name"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	name, ok := deviceName(in.Name)
	if !ok {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("give the computer a name of at most %d characters", maxLabel))
		return
	}
	found, err := c.State.RenameDevice(me(r), r.PathValue("id"), name)
	switch {
	case err != nil:
		c.fail(w, err)
	case !found:
		writeError(w, http.StatusNotFound, "no such device")
	default:
		w.WriteHeader(http.StatusNoContent)
	}
}

func (c *Console) revokeDevice(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	ok, err := c.State.DeleteDevice(me(r), id)
	switch {
	case err != nil:
		c.fail(w, err)
	case !ok:
		writeError(w, http.StatusNotFound, "no such device")
	default:
		c.surf.hub.drop(id)
		c.surf.wake()
		w.WriteHeader(http.StatusNoContent)
	}
}
