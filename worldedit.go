// Package worldedit is a port of FastAsyncWorldEdit (FAWE) to golem,
// registered as "worldedit".
//
// It exposes core.API via plugin.Service[worldedit.API](s) so other plugins
// can manage sessions, run transactional EditSessions, load/save schematics,
// subscribe to WorldEdit events, and register region restriction hooks.
package worldedit

import (
	"github.com/golem-mc/golem/plugin"
	"github.com/golem-mc/worldedit/commands"
	"github.com/golem-mc/worldedit/core"
)

// API re-exports core.API for callers importing the root worldedit package.
type API = core.API

// WorldEdit is the golem plugin entry point.
type WorldEdit struct {
	we *core.WorldEdit
}

func init() {
	plugin.Register("worldedit", &WorldEdit{},
		plugin.SoftDepends("luckperms"),
		plugin.LoadBefore("worldguard", "plotsquared"),
	)
}

// Enable initializes FastAsyncWorldEdit storage, services, listeners, CUI, and commands.
func (p *WorldEdit) Enable(s plugin.Server) error {
	we, err := core.Enable(s)
	if err != nil {
		return err
	}
	p.we = we
	commands.Register(we)
	return nil
}

// Disable shuts down background tasks, rollback databases, and logs.
func (p *WorldEdit) Disable() {
	if p.we != nil {
		p.we.Disable()
	}
}
