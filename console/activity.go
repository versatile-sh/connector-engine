package console

import (
	"bufio"
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"
)

const (
	activityRetention = 30 * 24 * time.Hour
	activityQueue     = 1024
	activityDay       = "2006-01-02"
	activityExt       = ".jsonl"
	activitySettings  = "settings.json"
	activityLineMax   = 4 << 20
)

// CallOutcome is what a call came to, as the activity page shows it.
type CallOutcome string

// Outcomes of a call; running is an engine row not finished yet.
const (
	OutcomeOK      CallOutcome = "ok"
	OutcomeError   CallOutcome = "error"
	OutcomeDenied  CallOutcome = "denied"
	OutcomePending CallOutcome = "pending"
	OutcomeRunning CallOutcome = "running"
)

// ActivitySettings is what the owner keeps of each call, and when the log was last cleared.
type ActivitySettings struct {
	RecordArguments bool      `json:"record_arguments"`
	RecordResults   bool      `json:"record_results"`
	ClearedAt       time.Time `json:"cleared_at,omitzero"`
}

// Capture is what the console saw of one tools/call on its /mcp proxy; a Late capture is the answer approval_status gave for an earlier held call.
type Capture struct {
	ID            string          `json:"id"`
	At            time.Time       `json:"at"`
	Done          time.Time       `json:"done,omitzero"`
	Agent         string          `json:"agent"`
	Session       string          `json:"session,omitempty"`
	MCPSession    string          `json:"mcp_session,omitempty"`
	RPCID         json.RawMessage `json:"rpc_id,omitempty"`
	Tool          string          `json:"tool"`
	Outcome       CallOutcome     `json:"outcome"`
	Code          string          `json:"code,omitempty"`
	Detail        string          `json:"detail,omitempty"`
	Approval      string          `json:"approval,omitempty"`
	Late          bool            `json:"late,omitempty"`
	KeptArguments bool            `json:"kept_arguments,omitempty"`
	Arguments     string          `json:"arguments,omitempty"`
	ArgumentsCut  bool            `json:"arguments_cut,omitempty"`
	KeptResult    bool            `json:"kept_result,omitempty"`
	Result        string          `json:"result,omitempty"`
	ResultCut     bool            `json:"result_cut,omitempty"`
	ResultBytes   int             `json:"result_bytes,omitempty"`
}

type activityJob struct {
	capture Capture
	flushed chan struct{}
}

// Activity is the console's log of captured calls: one JSON line per call in a file per UTC day, kept 30 days, written by one goroutine so the proxy never waits on the disk.
type Activity struct {
	dir  string
	log  *slog.Logger
	now  func() time.Time
	jobs chan activityJob
	done chan struct{}

	sendMu   sync.RWMutex
	closed   bool
	mu       sync.Mutex
	settings ActivitySettings
}

// OpenActivity opens the log in dir, creating it with mode 0700, prunes what is past retention and starts the writer.
func OpenActivity(dir string, log *slog.Logger) (*Activity, error) {
	if log == nil {
		log = slog.New(slog.DiscardHandler)
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	a := &Activity{dir: dir, log: log, now: time.Now, jobs: make(chan activityJob, activityQueue), done: make(chan struct{}), settings: ActivitySettings{RecordArguments: true}}
	raw, err := os.ReadFile(filepath.Join(dir, activitySettings))
	switch {
	case err == nil:
		if err := json.Unmarshal(raw, &a.settings); err != nil {
			return nil, err
		}
	case !errors.Is(err, os.ErrNotExist):
		return nil, err
	}
	a.prune()
	go a.write()
	return a, nil
}

func newCaptureID(at time.Time) string {
	b := make([]byte, 8)
	rand.Read(b)
	return at.UTC().Format("20060102") + "-" + hex.EncodeToString(b)
}

func (a *Activity) write() {
	defer close(a.done)
	for job := range a.jobs {
		if job.flushed != nil {
			close(job.flushed)
			continue
		}
		if err := a.append(job.capture); err != nil {
			a.log.Warn("activity not recorded", "error", err)
		}
	}
}

func (a *Activity) append(c Capture) error {
	line, err := json.Marshal(c)
	if err != nil {
		return err
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if c.At.Before(a.settings.ClearedAt) {
		return nil
	}
	path := filepath.Join(a.dir, c.At.UTC().Format(activityDay)+activityExt)
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		a.pruneLocked()
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0o600)
	if err != nil {
		return err
	}
	if _, err := f.Write(append(line, '\n')); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

// Record queues a capture for the writer without blocking; a full queue drops it with a warning.
func (a *Activity) Record(c Capture) {
	a.sendMu.RLock()
	defer a.sendMu.RUnlock()
	if a.closed {
		return
	}
	select {
	case a.jobs <- activityJob{capture: c}:
	default:
		a.log.Warn("activity queue full; a call was not recorded", "tool", c.Tool)
	}
}

// Flush waits until every capture queued before it is on disk.
func (a *Activity) Flush() {
	a.sendMu.RLock()
	if a.closed {
		a.sendMu.RUnlock()
		return
	}
	ack := make(chan struct{})
	a.jobs <- activityJob{flushed: ack}
	a.sendMu.RUnlock()
	<-ack
}

// Close stops taking captures and returns once the queued ones are written.
func (a *Activity) Close() {
	a.sendMu.Lock()
	if !a.closed {
		a.closed = true
		close(a.jobs)
	}
	a.sendMu.Unlock()
	<-a.done
}

// Settings returns what the owner chose to keep.
func (a *Activity) Settings() ActivitySettings {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.settings
}

func (a *Activity) saveSettings(s ActivitySettings) error {
	raw, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	if err := writeAtomic(filepath.Join(a.dir, activitySettings), raw); err != nil {
		return err
	}
	a.settings = s
	return nil
}

// SetRecording chooses whether new captures keep their arguments and their result excerpts.
func (a *Activity) SetRecording(arguments, results bool) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	s := a.settings
	s.RecordArguments, s.RecordResults = arguments, results
	return a.saveSettings(s)
}

// DeleteAll removes every captured call and hides the engine's older rows from the activity page.
func (a *Activity) DeleteAll() error {
	a.mu.Lock()
	defer a.mu.Unlock()
	names, err := a.days()
	if err != nil {
		return err
	}
	for _, name := range names {
		if err := os.Remove(filepath.Join(a.dir, name)); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	s := a.settings
	s.ClearedAt = a.now().UTC()
	return a.saveSettings(s)
}

// Floor is the oldest time the activity page shows: retention or the last clearing, whichever is later.
func (a *Activity) Floor() time.Time {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.floorLocked()
}

func (a *Activity) floorLocked() time.Time {
	floor := a.now().Add(-activityRetention)
	if a.settings.ClearedAt.After(floor) {
		return a.settings.ClearedAt
	}
	return floor
}

func (a *Activity) days() ([]string, error) {
	entries, err := os.ReadDir(a.dir)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, e := range entries {
		day, ok := strings.CutSuffix(e.Name(), activityExt)
		if _, err := time.Parse(activityDay, day); ok && err == nil && e.Type().IsRegular() {
			out = append(out, e.Name())
		}
	}
	slices.Sort(out)
	return out, nil
}

func (a *Activity) prune() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.pruneLocked()
}

func (a *Activity) pruneLocked() {
	names, err := a.days()
	if err != nil {
		a.log.Warn("activity prune", "error", err)
		return
	}
	oldest := a.now().Add(-activityRetention).UTC().Format(activityDay) + activityExt
	for _, name := range names {
		if name < oldest {
			if err := os.Remove(filepath.Join(a.dir, name)); err != nil {
				a.log.Warn("activity prune", "error", err)
			}
		}
	}
}

// Captures returns the calls captured at or after from and not before the floor, oldest first, each with the later answer of its approval folded in.
func (a *Activity) Captures(from time.Time) ([]Capture, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.pruneLocked()
	floor := a.floorLocked()
	if from.Before(floor) {
		from = floor
	}
	names, err := a.days()
	if err != nil {
		return nil, err
	}
	first := from.UTC().Format(activityDay) + activityExt
	var calls, late []Capture
	for _, name := range names {
		if name < first {
			continue
		}
		if err := readCaptures(filepath.Join(a.dir, name), func(c Capture) {
			if c.Late {
				late = append(late, c)
			} else if !c.At.Before(from) {
				calls = append(calls, c)
			}
		}); err != nil {
			return nil, err
		}
	}
	for _, l := range late {
		for i := range calls {
			if calls[i].Approval == l.Approval && calls[i].Agent == l.Agent {
				calls[i].Outcome, calls[i].Code, calls[i].Detail, calls[i].Done = l.Outcome, l.Code, l.Detail, l.Done
				if l.KeptResult {
					calls[i].KeptResult, calls[i].Result, calls[i].ResultCut, calls[i].ResultBytes = true, l.Result, l.ResultCut, l.ResultBytes
				}
			}
		}
	}
	slices.SortStableFunc(calls, func(x, y Capture) int { return x.At.Compare(y.At) })
	return calls, nil
}

// Capture returns one captured call by its id.
func (a *Activity) Capture(id string) (Capture, bool, error) {
	day, err := time.Parse("20060102", strings.SplitN(id, "-", 2)[0])
	if err != nil {
		return Capture{}, false, nil
	}
	list, err := a.Captures(day)
	if err != nil {
		return Capture{}, false, err
	}
	for _, c := range list {
		if c.ID == id {
			return c, true, nil
		}
	}
	return Capture{}, false, nil
}

func readCaptures(path string, each func(Capture)) error {
	f, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64<<10), activityLineMax)
	for sc.Scan() {
		var c Capture
		if line := bytes.TrimSpace(sc.Bytes()); len(line) > 0 && json.Unmarshal(line, &c) == nil && c.ID != "" {
			each(c)
		}
	}
	return sc.Err()
}
