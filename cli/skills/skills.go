// Package skills is the host-side `cnctl skills export`: it loads the skills granted to an agent through the agent endpoint and writes their exact bytes into a folder clients read.
package skills

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/agents-control/connector-engine/cli/tools"
	"github.com/agents-control/connector-engine/core/listing"
	"github.com/agents-control/connector-engine/port"
)

// ManifestFile is the file an export writes in its folder, naming every file it wrote.
const ManifestFile = ".cnctl-skills.json"

const usage = `usage: skills export --to DIR [--skill NAME] [--prune]
  DIR is a skills folder such as ~/.claude/skills or ~/.agents/skills.
environment: CN_ENDPOINT (the /mcp address), CN_AGENT_TOKEN (the token of this agent)`

// Manifest records what an export wrote, so a later export overwrites and prunes only its own files.
type Manifest struct {
	Endpoint string            `json:"endpoint"`
	Exported time.Time         `json:"exported_at"`
	Skills   map[string]string `json:"skills"`
	Files    []string          `json:"files"`
}

// Command is the `skills` group of cnctl; it is an MCP client of the agent endpoint.
type Command struct {
	getenv func(string) string
}

// New returns the command; a nil getenv reads the process environment.
func New(getenv func(string) string) *Command {
	if getenv == nil {
		getenv = os.Getenv
	}
	return &Command{getenv: getenv}
}

type loaded struct {
	Skill          string              `json:"skill"`
	Path           string              `json:"path"`
	SHA256         string              `json:"sha256"`
	Encoding       string              `json:"encoding"`
	Content        string              `json:"content"`
	Redistribution port.Redistribution `json:"redistribution"`
	Status         string              `json:"status"`
	Files          []struct {
		Path   string `json:"path"`
		SHA256 string `json:"sha256"`
	} `json:"files"`
}

// Run executes `skills export`.
func (c *Command) Run(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	if len(args) == 0 || args[0] != "export" {
		fmt.Fprintln(stderr, usage)
		return fmt.Errorf("the subcommand is export")
	}
	fs := flag.NewFlagSet("skills export", flag.ContinueOnError)
	fs.SetOutput(stderr)
	to := fs.String("to", "", "the skills folder to write into")
	only := fs.String("skill", "", "export this skill alone, by the name skill_list gives")
	prune := fs.Bool("prune", false, "remove the files an earlier export wrote that this one did not")
	timeout := fs.Duration("timeout", 2*time.Minute, "time allowed for the whole command")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	if *to == "" || fs.NArg() > 0 {
		fmt.Fprintln(stderr, usage)
		return fmt.Errorf("export takes --to DIR")
	}
	ctx, cancel := context.WithTimeout(ctx, *timeout)
	defer cancel()
	cs, err := tools.Dial(ctx, c.getenv)
	if err != nil {
		return err
	}
	defer cs.Close()
	return Export(ctx, cs, Options{To: *to, Only: *only, Prune: *prune, Endpoint: c.getenv("CN_ENDPOINT")}, stdout)
}

// Options are the choices of one export.
type Options struct {
	To       string
	Only     string
	Prune    bool
	Endpoint string
}

func call(ctx context.Context, cs *mcp.ClientSession, name string, args map[string]any, into any) error {
	res, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		return err
	}
	raw, err := json.Marshal(res.StructuredContent)
	if err != nil {
		return err
	}
	if res.IsError {
		return fmt.Errorf("%s: %s", name, raw)
	}
	return json.Unmarshal(raw, into)
}

func load(ctx context.Context, cs *mcp.ClientSession, skill, path string) (loaded, []byte, error) {
	var l loaded
	if err := call(ctx, cs, "skill_load", map[string]any{"skill": skill, "path": path}, &l); err != nil {
		return l, nil, err
	}
	if l.Status == "pending_approval" {
		return l, nil, fmt.Errorf("skill %s waits for your approval; approve it in the console and export again", skill)
	}
	if l.Redistribution == port.RedistributionMetadataOnly {
		return l, nil, nil
	}
	data := []byte(l.Content)
	if l.Encoding == "base64" {
		var err error
		if data, err = base64.StdEncoding.DecodeString(l.Content); err != nil {
			return l, nil, err
		}
	}
	sum := sha256.Sum256(data)
	if hex.EncodeToString(sum[:]) != l.SHA256 {
		return l, nil, fmt.Errorf("skill %s file %s does not match its sha256", skill, l.Path)
	}
	return l, data, nil
}

func readManifest(dir string) (Manifest, error) {
	m := Manifest{Skills: map[string]string{}}
	raw, err := os.ReadFile(filepath.Join(dir, ManifestFile))
	if os.IsNotExist(err) {
		return m, nil
	}
	if err != nil {
		return m, err
	}
	if err := json.Unmarshal(raw, &m); err != nil {
		return m, fmt.Errorf("%s: %w", ManifestFile, err)
	}
	return m, nil
}

func noLinks(root, rel string) error {
	cur := root
	for _, part := range strings.Split(filepath.ToSlash(rel), "/") {
		cur = filepath.Join(cur, part)
		fi, err := os.Lstat(cur)
		if os.IsNotExist(err) {
			return nil
		}
		if err != nil {
			return err
		}
		if fi.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("%s is a symlink; the export never writes through one", cur)
		}
	}
	return nil
}

func place(root, rel string, data []byte, owned bool) error {
	if err := noLinks(root, rel); err != nil {
		return err
	}
	dest := filepath.Join(root, filepath.FromSlash(rel))
	if _, err := os.Lstat(dest); err == nil && !owned {
		return fmt.Errorf("%s exists and an earlier export did not write it; move it away first", dest)
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	return os.WriteFile(dest, data, 0o644)
}

// Export writes every granted skill, or the one named, as a folder under o.To, records the files in ManifestFile, and with Prune removes the files it wrote before and not now.
func Export(ctx context.Context, cs *mcp.ClientSession, o Options, stdout io.Writer) error {
	root, err := filepath.Abs(o.To)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		return err
	}
	if fi, err := os.Lstat(root); err != nil || fi.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("%s is not a plain folder", root)
	}
	before, err := readManifest(root)
	if err != nil {
		return err
	}
	var listed struct {
		Skills []struct {
			Name string `json:"name"`
		} `json:"skills"`
	}
	if err := call(ctx, cs, "skill_list", map[string]any{}, &listed); err != nil {
		return err
	}
	after := Manifest{Endpoint: o.Endpoint, Exported: time.Now().UTC(), Skills: map[string]string{}}
	scripts := false
	for _, s := range listed.Skills {
		if o.Only != "" && s.Name != o.Only {
			continue
		}
		entry, data, err := load(ctx, cs, s.Name, "")
		if err != nil {
			return err
		}
		if entry.Redistribution == port.RedistributionMetadataOnly {
			fmt.Fprintf(stdout, "skipped %s: listed without its files (no redistributable licence)\n", s.Name)
			continue
		}
		if !listing.SkillName(entry.Skill) || after.Skills[entry.Skill] != "" {
			return fmt.Errorf("skill %s has the folder name %q, which is invalid or taken", s.Name, entry.Skill)
		}
		after.Skills[entry.Skill] = s.Name
		for _, f := range entry.Files {
			if err := listing.SkillPath(f.Path); err != nil {
				return err
			}
			body := data
			if f.Path != entry.Path {
				var l loaded
				if l, body, err = load(ctx, cs, s.Name, f.Path); err != nil {
					return err
				}
				if l.SHA256 != f.SHA256 {
					return fmt.Errorf("skill %s file %s changed while it was exported", s.Name, f.Path)
				}
			}
			rel := entry.Skill + "/" + f.Path
			if err := place(root, rel, body, slices.Contains(before.Files, rel)); err != nil {
				return err
			}
			after.Files = append(after.Files, rel)
			scripts = scripts || listing.LooksExecutable(port.SkillFile{Path: f.Path, Content: body})
		}
		fmt.Fprintf(stdout, "exported %s to %s\n", s.Name, filepath.Join(root, entry.Skill))
	}
	if o.Only != "" && len(after.Skills) == 0 {
		return fmt.Errorf("no exported skill is named %s", o.Only)
	}
	for _, rel := range before.Files {
		if slices.Contains(after.Files, rel) {
			continue
		}
		if !o.Prune {
			after.Files = append(after.Files, rel)
			continue
		}
		if err := listing.SkillPath(rel); err != nil {
			return err
		}
		if err := noLinks(root, rel); err != nil {
			return err
		}
		if err := os.Remove(filepath.Join(root, filepath.FromSlash(rel))); err != nil && !os.IsNotExist(err) {
			return err
		}
		fmt.Fprintf(stdout, "pruned %s\n", rel)
	}
	if !o.Prune {
		for name, alias := range before.Skills {
			if after.Skills[name] == "" {
				after.Skills[name] = alias
			}
		}
	}
	slices.Sort(after.Files)
	raw, err := json.MarshalIndent(after, "", "  ")
	if err != nil {
		return err
	}
	if err := place(root, ManifestFile, raw, true); err != nil {
		return err
	}
	if scripts {
		fmt.Fprintln(stdout, "warning: the exported scripts are now files your AI apps may run with the trust of local skills; the gateway never ran them")
	}
	return nil
}
