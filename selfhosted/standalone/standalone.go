// Package standalone runs a person install without Docker: the engine and the console in one process on the loopback, over a SQLite store in a per-user data directory.
package standalone

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/versatile-sh/connector-engine/app"
	"github.com/versatile-sh/connector-engine/config"
	"github.com/versatile-sh/connector-engine/internal/plugkit/adapters"
	"github.com/versatile-sh/connector-engine/selfhosted/cli/keychain"
	"github.com/versatile-sh/connector-engine/selfhosted/console"
)

// Options configure an install: its data directory, the console port on 127.0.0.1 (0 picks one), the OS keychain (nil keeps plaintext root keys), the build and the log.
type Options struct {
	Data     string
	Port     int
	Keychain keychain.Keychain
	Version  string
	Log      *slog.Logger
}

// Install is a running install: the console URL its browser and agents use; Stop ends it and Wait returns once it has stopped.
type Install struct {
	URL string

	o      Options
	paths  paths
	cfg    config.Config
	file   adapters.File
	reg    *app.Registry
	addr   string
	lis    net.Listener
	host   atomic.Pointer[http.Handler]
	stop   context.CancelFunc
	done   chan struct{}
	err    error
	readyc chan error
	once   sync.Once
}

// Start runs the engine and the console of o until ctx ends or Stop is called, restarting both after a backup is restored; it returns once both listen.
func Start(ctx context.Context, o Options) (*Install, error) {
	if o.Log == nil {
		o.Log = slog.Default()
	}
	p, err := prepare(o.Data)
	if err != nil {
		return nil, err
	}
	in := &Install{o: o, paths: p, reg: registry(o.Keychain, p.account), addr: net.JoinHostPort("127.0.0.1", strconv.Itoa(o.Port)), done: make(chan struct{}), readyc: make(chan error, 1)}
	if in.cfg, in.file, err = engineProfile(p, in.reg); err != nil {
		return nil, err
	}
	ctl, err := listenControl(p.control)
	if err != nil {
		return nil, err
	}
	if in.lis, err = net.Listen("tcp", in.addr); err != nil {
		ctl.Close()
		return nil, fmt.Errorf("console: %w", err)
	}
	in.addr = in.lis.Addr().String()
	in.URL = "http://" + in.addr
	ctx, in.stop = context.WithCancel(ctx)
	go in.run(ctx, ctl)
	if err := <-in.readyc; err != nil {
		<-in.done
		return nil, err
	}
	return in, nil
}

// Stop ends the install and waits until it has stopped.
func (in *Install) Stop() error {
	in.stop()
	return in.Wait()
}

// Wait returns once the install has stopped, with the error that stopped it.
func (in *Install) Wait() error {
	<-in.done
	return in.err
}

func (in *Install) listen() (net.Listener, error) {
	if lis := in.lis; lis != nil {
		in.lis = nil
		return lis, nil
	}
	return net.Listen("tcp", in.addr)
}

func (in *Install) ready(err error) { in.once.Do(func() { in.readyc <- err }) }

func (in *Install) run(ctx context.Context, ctl net.Listener) {
	defer close(in.done)
	srv := &http.Server{Handler: in.control(), ReadHeaderTimeout: 10 * time.Second}
	go func() { _ = srv.Serve(ctl) }()
	defer srv.Close()
	defer func() {
		if in.lis != nil {
			in.lis.Close()
		}
	}()
	for {
		err := in.generation(ctx)
		if err != nil || ctx.Err() != nil {
			in.err = err
			in.ready(err)
			return
		}
		in.o.Log.Info("restarting the engine and the console on the restored data")
	}
}

func (in *Install) generation(ctx context.Context) error {
	gen, cancel := context.WithCancel(ctx)
	defer cancel()
	a, err := app.New(gen, in.cfg, in.file, app.Options{Log: in.o.Log, Build: in.o.Version, Adapters: in.reg})
	if err != nil {
		return fmt.Errorf("engine: %w", err)
	}
	defer a.Close()
	l, err := a.Listen()
	if err != nil {
		return fmt.Errorf("engine: %w", err)
	}
	ran := make(chan error, 1)
	go func() { ran <- a.Run(gen, l); cancel() }()
	err = in.serveConsole(gen, cancel, "http://"+l.HTTP.Addr().String(), "http://"+l.Operator.Addr().String())
	cancel()
	return errors.Join(err, <-ran)
}

func (in *Install) serveConsole(gen context.Context, restart context.CancelFunc, engineURL, operatorURL string) error {
	lis, err := in.listen()
	if err != nil {
		return fmt.Errorf("console: %w", err)
	}
	c, closeConsole, err := console.Open(gen, console.Options{PublicURL: in.URL, EngineURL: engineURL, OperatorURL: operatorURL, Operator: in.paths.operator,
		StatePath: in.paths.consoleState, Unlock: in.unlocker(in.paths.consoleSecrets), Tenant: "local", Workspace: "default", Person: "owner",
		AgentTTL: in.cfg.Auth.AgentTokenMaxLifetime, Restart: restart, Log: in.o.Log})
	if err != nil {
		lis.Close()
		return fmt.Errorf("console: %w", err)
	}
	defer closeConsole()
	srv := &http.Server{Handler: c.Handler(), ReadHeaderTimeout: 10 * time.Second}
	go func() { _ = srv.Serve(lis) }()
	host := c.HostHandler()
	in.host.Store(&host)
	in.ready(nil)
	<-gen.Done()
	in.host.Store(nil)
	shut, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	return srv.Shutdown(shut)
}
