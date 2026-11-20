// Package wetest provides an in-process plugintest harness for FastAsyncWorldEdit.
package wetest

import (
	"strings"
	"testing"

	"github.com/golem-mc/golem/plugintest"
	_ "github.com/golem-mc/worldedit"
	"github.com/golem-mc/worldedit/core"
)

// Env is a running golem test server with FastAsyncWorldEdit enabled.
type Env struct {
	T   *testing.T
	Srv *plugintest.Server
	WE  *core.WorldEdit
}

// Client is a joined test player.
type Client struct {
	*plugintest.Player
	env *Env
}

// Start boots a flat test server in a temporary directory with FastAsyncWorldEdit enabled.
func Start(t *testing.T, opts ...plugintest.Option) *Env {
	t.Helper()
	t.Chdir(t.TempDir())
	srv := plugintest.Start(t, opts...)
	we, ok := core.Get()
	if !ok {
		t.Fatal("worldedit did not enable")
	}
	t.Cleanup(func() {
		we.Disable()
	})
	return &Env{T: t, Srv: srv, WE: we}
}

// Join connects a player; op grants operator level 4.
func (env *Env) Join(name string, op bool) *Client {
	env.T.Helper()
	c := &Client{Player: env.Srv.Join(name), env: env}
	if op {
		c.Op()
	}
	c.Messages(3)
	return c
}

// Run executes a slash command (including leading "/" or "//") as c and returns received messages.
func (env *Env) Run(c *Client, line string) []string {
	env.T.Helper()
	if !strings.HasPrefix(line, "/") {
		line = "/" + line
	}
	c.Run(line)
	return c.Messages(5)
}

// Console runs a command at the server console and returns its output lines.
func (env *Env) Console(line string) []string {
	env.T.Helper()
	return env.Srv.Console(line)
}

// Has reports whether any message contains sub.
func Has(msgs []string, sub string) bool {
	for _, m := range msgs {
		if strings.Contains(m, sub) {
			return true
		}
	}
	return false
}

// Expect fails the test if no message contains sub.
func (env *Env) Expect(msgs []string, sub string) {
	env.T.Helper()
	if !Has(msgs, sub) {
		env.T.Fatalf("expected message containing %q; got %q", sub, msgs)
	}
}
