package storetest

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

// Image is the Postgres image the conformance runs use.
const Image = "postgres:16-alpine"

// Postgres is a throwaway Postgres container started for one test binary.
type Postgres struct {
	name string
	addr string
	n    int
}

// StartPostgres starts the container, or returns an error when CN_PG=0, docker or the image is unavailable.
func StartPostgres() (*Postgres, error) {
	if os.Getenv("CN_PG") == "0" || os.Getenv("CN_DOCKER") == "0" {
		return nil, fmt.Errorf("postgres runs are switched off")
	}
	suffix := make([]byte, 4)
	_, _ = rand.Read(suffix)
	p := &Postgres{name: fmt.Sprintf("cn-test-pg-%d-%s", os.Getpid(), hex.EncodeToString(suffix))}
	if out, err := exec.Command("docker", "run", "-d", "--rm", "--name", p.name, "-e", "POSTGRES_PASSWORD=x", "-p", "127.0.0.1:0:5432", Image).CombinedOutput(); err != nil {
		return nil, fmt.Errorf("docker run: %w: %s", err, out)
	}
	port, err := exec.Command("docker", "port", p.name, "5432/tcp").Output()
	if err != nil {
		p.Stop()
		return nil, fmt.Errorf("docker port: %w", err)
	}
	p.addr = strings.TrimSpace(strings.SplitN(string(port), "\n", 2)[0])
	for i := 0; ; i++ {
		if _, err = p.psql("select 1"); err == nil {
			break
		}
		if i == 60 {
			p.Stop()
			return nil, fmt.Errorf("postgres not ready: %w", err)
		}
		time.Sleep(500 * time.Millisecond)
	}
	if _, err := p.psql("CREATE ROLE cn LOGIN PASSWORD 'cn' NOSUPERUSER NOBYPASSRLS"); err != nil {
		p.Stop()
		return nil, err
	}
	return p, nil
}

// Stop removes the container this value started, by its exact name.
func (p *Postgres) Stop() { _ = exec.Command("docker", "rm", "-f", p.name).Run() }

// SuperuserDSN connects as the bootstrap superuser, which bypasses row-level security.
func (p *Postgres) SuperuserDSN() string { return "postgres://postgres:x@" + p.addr + "/postgres" }

// NewDatabase creates an empty database owned by the unprivileged role cn and returns its DSN.
func (p *Postgres) NewDatabase() (string, error) {
	p.n++
	name := fmt.Sprintf("t%d", p.n)
	if _, err := p.psql("CREATE DATABASE " + name + " OWNER cn"); err != nil {
		return "", err
	}
	return "postgres://cn:cn@" + p.addr + "/" + name, nil
}

func (p *Postgres) psql(sql string) (string, error) {
	out, err := exec.Command("docker", "exec", p.name, "psql", "-h", "127.0.0.1", "-U", "postgres", "-v", "ON_ERROR_STOP=1", "-tAc", sql).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("psql: %w: %s", err, out)
	}
	return string(out), nil
}
