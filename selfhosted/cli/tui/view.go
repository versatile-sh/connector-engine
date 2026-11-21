package tui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/versatile-sh/connector-engine/selfhosted/console"
)

var (
	ink   = lipgloss.Color("#0e1116")
	paper = lipgloss.Color("#f5f5f2")
	muted = lipgloss.Color("#8a919c")
	lime  = lipgloss.Color("#c6f432")

	base   = lipgloss.NewStyle().Background(ink).Foreground(paper)
	dim    = base.Foreground(muted)
	strong = base.Bold(true)
	focus  = base.Foreground(lime)
	chosen = base.Bold(true).Underline(true)
)

func dot(on bool) string {
	if on {
		return focus.Render("●") + base.Render(" ")
	}
	return base.Render("  ")
}

func clip(s string, w int) string {
	if w <= 1 {
		return ""
	}
	r := []rune(s)
	if len(r) > w {
		return string(r[:w-1]) + "…"
	}
	return s
}

// View renders the screen at the terminal's size on the alternate screen.
func (m Model) View() tea.View {
	v := tea.NewView(m.render())
	v.AltScreen = true
	return v
}

func (m Model) render() string {
	w, h := max(m.w, 20), max(m.h, 6)
	foot := wrap(m.hints(), w)
	room := max(h-3-len(foot), 1)
	var body []string
	sel := 0
	switch m.screen {
	case screenConnectors:
		if m.open != "" {
			body, sel = m.detailView(w)
		} else {
			body, sel = m.listView(w)
		}
	case screenKeys:
		body, sel = m.keysView(w)
	case screenApprovals:
		body, sel = m.approvalsView(w)
	case screenActivity:
		body, sel = m.activityView(w)
	}
	pinned := 0
	if m.screen == screenConnectors && m.open != "" {
		pinned = min(2, len(body))
	}
	lines := append([]string{m.header(w), ""}, body[:pinned]...)
	lines = append(lines, window(body[pinned:], sel-pinned, max(room-pinned, 1))...)
	for len(lines) < h-1-len(foot) {
		lines = append(lines, "")
	}
	lines = append(lines, m.statusLine(w))
	for _, f := range foot {
		lines = append(lines, dim.Render(f))
	}
	for i, l := range lines {
		lines[i] = base.Width(w).MaxWidth(w).Render(l)
	}
	return strings.Join(lines, "\n")
}

func window(lines []string, sel, room int) []string {
	if len(lines) <= room {
		return lines
	}
	start := min(max(sel-room/2, 0), len(lines)-room)
	return lines[start : start+room]
}

func wrap(items []string, w int) []string {
	var out []string
	line := ""
	for _, it := range items {
		it = clip(it, w)
		switch {
		case line == "":
			line = it
		case len([]rune(line))+3+len([]rune(it)) <= w:
			line += " · " + it
		default:
			out, line = append(out, line), it
		}
	}
	return append(out, line)
}

func chunks(s string, w int) []string {
	r := []rune(s)
	var out []string
	for len(r) > w {
		out, r = append(out, string(r[:w])), r[w:]
	}
	return append(out, string(r))
}

func (m Model) header(w int) string {
	parts := []string{strong.Render("vsh") + base.Render("  ")}
	for i, name := range screens {
		label := name
		if screen(i) == screenApprovals && len(m.approvals) > 0 {
			label = fmt.Sprintf("%s %d", name, len(m.approvals))
		}
		if screen(i) == m.screen {
			parts = append(parts, focus.Render("●")+strong.Render(" "+label)+base.Render("  "))
		} else {
			parts = append(parts, dim.Render("  "+label+"  "))
		}
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, parts...)
}

func (m Model) statusLine(w int) string {
	if m.status == "" {
		return ""
	}
	if m.failed {
		return strong.Render(clip("error: "+m.status, w))
	}
	return dim.Render(clip(m.status, w))
}

func (m Model) hints() []string {
	switch {
	case m.token != "":
		return []string{"copy the key now: it is shown once", "any key closes"}
	case m.filtering:
		return []string{"type to filter", "enter keep", "esc clear"}
	case m.naming:
		return []string{"enter create", "esc cancel"}
	case m.revoking != "":
		return []string{"y revoke", "any other key keeps it"}
	}
	common := []string{"tab screens", "q quit"}
	switch m.screen {
	case screenConnectors:
		if m.open != "" {
			return append([]string{"↑↓ move", "←→/space off·on·ask", "[ ] key", "f follow/custom", "r read on", "w write ask", "x all off", "esc back"}, common...)
		}
		return append([]string{"↑↓ move", "enter open", "/ filter"}, common...)
	case screenKeys:
		return append([]string{"↑↓ move", "n new key", "d revoke"}, common...)
	case screenApprovals:
		return append([]string{"↑↓ move", "a approve", "d deny"}, common...)
	}
	return append([]string{"live, newest last"}, common...)
}

func (m Model) listView(w int) ([]string, int) {
	var out []string
	if m.filtering || m.filter.Value() != "" {
		out = append(out, m.filter.View(), "")
	}
	list := m.visible()
	if len(list) == 0 {
		return append(out, dim.Render("no connectors: import one with vsh connectors import <listing-url>")), 0
	}
	sel := len(out) + m.cursor
	for i, k := range list {
		line := fmt.Sprintf("%-20s %-28s %2d functions  %d keys", clip(k.Alias, 20), clip(k.Title, 28), len(k.Functions), len(k.Profiles))
		out = append(out, dot(i == m.cursor)+base.Render(clip(line, w-2)))
	}
	return out, sel
}

func (m Model) tabs(k console.ConnectorSummary) string {
	names := []string{"Default"}
	for _, p := range k.Profiles {
		mode := "Follow"
		switch p.Mode {
		case console.ProfileCustom:
			mode = "Custom"
		case console.ProfileOff:
			mode = "Off"
		}
		names = append(names, p.Agent+" ("+mode+")")
	}
	var parts []string
	for i, n := range names {
		if i == m.tab {
			parts = append(parts, focus.Render("●")+strong.Render(" "+n)+base.Render("  "))
		} else {
			parts = append(parts, dim.Render("  "+n+"  "))
		}
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, parts...)
}

func (m Model) detailView(w int) ([]string, int) {
	i, ok := m.connector()
	if !ok {
		return nil, 0
	}
	k := m.conns[i]
	var p *console.Profile
	if pr, ok := m.key(k); ok {
		p = &pr
	}
	out := []string{strong.Render(clip(k.Title, w)) + dim.Render("  "+k.Alias), m.tabs(k)}
	kind, sel := "", 0
	nameW := max(min(w-28, 40), 10)
	for ri, f := range rows(k) {
		if string(f.Kind) != kind {
			kind = string(f.Kind)
			out = append(out, "", dim.Render(strings.ToUpper(kind)))
		}
		a := Access(k, p, f.Name)
		cells := []string{}
		for _, l := range levels {
			label := strings.ToUpper(string(l[:1])) + string(l[1:])
			if l == a {
				cells = append(cells, chosen.Render(label))
			} else {
				cells = append(cells, dim.Render(label))
			}
		}
		name := fmt.Sprintf("%-*s", nameW, clip(f.Name, nameW))
		if ri == m.row {
			sel = len(out)
		}
		out = append(out, dot(ri == m.row)+base.Render(name+" ")+strings.Join(cells, base.Render("  ")))
	}
	if len(k.Functions) == 0 {
		out = append(out, "", dim.Render("this connector has no functions"))
	}
	return out, sel
}

func (m Model) keysView(w int) ([]string, int) {
	var out []string
	if m.token != "" {
		out = append(out, strong.Render("new access key, shown once:"), "")
		for _, c := range chunks(m.token, w) {
			out = append(out, base.Render(c))
		}
		return out, 0
	}
	if m.naming {
		out = append(out, m.name.View(), "")
	}
	if m.revoking != "" {
		out = append(out, strong.Render("revoke "+m.revoking+"? agents using it lose access at once (y/n)"), "")
	}
	if len(m.keys) == 0 {
		return append(out, dim.Render("no access keys yet: n creates one")), 0
	}
	sel := len(out) + m.keyCursor
	for i, a := range m.keys {
		via := "key"
		if a.ViaBrowser {
			via = "browser sign-in"
		}
		line := fmt.Sprintf("%-28s expires %s  %s", clip(a.Name, 28), a.ExpiresAt.Local().Format("2006-01-02"), via)
		out = append(out, dot(i == m.keyCursor)+base.Render(clip(line, w-2)))
	}
	return out, sel
}

func (m Model) approvalsView(w int) ([]string, int) {
	if len(m.approvals) == 0 {
		return []string{dim.Render("nothing waits for approval")}, 0
	}
	alias := map[string]string{}
	for _, k := range m.conns {
		alias[k.ID] = k.Alias
	}
	var out []string
	for i, a := range m.approvals {
		conn := alias[string(a.Connector)]
		if conn == "" {
			conn = string(a.Connector)
		}
		line := fmt.Sprintf("%-20s %-20s %-24s expires %s", clip(string(a.Agent), 20), clip(conn, 20), clip(a.Function, 24), a.ExpiresAt.Local().Format("15:04:05"))
		out = append(out, dot(i == m.apCursor)+base.Render(clip(line, w-2)))
	}
	return out, m.apCursor
}

func (m Model) activityView(w int) ([]string, int) {
	if len(m.calls) == 0 {
		return []string{dim.Render("no calls in the last hour")}, 0
	}
	var out []string
	for _, c := range m.calls {
		where := c.Alias
		if where == "" {
			where = c.Connector
		}
		line := fmt.Sprintf("%s  %-16s %-16s %-22s %s", c.At.Local().Format("15:04:05"), clip(c.Agent, 16), clip(where, 16), clip(c.Function, 22), c.Outcome)
		out = append(out, base.Render(clip(line, w)))
	}
	return out, len(out) - 1
}
