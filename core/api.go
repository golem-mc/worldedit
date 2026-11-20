package core

import (
	"github.com/golem-mc/golem/plugin"
	"github.com/golem-mc/worldedit/region"
	"github.com/golem-mc/worldedit/storage"
)

// RegionManager is an extension hook that protection plugins (e.g. WorldGuard,
// PlotSquared) can register with WorldEdit to restrict where a player may edit.
type RegionManager interface {
	Name() string
	AllowedRegions(p plugin.Player, w plugin.World) []region.Region
}

// API is the public Go service exposed via plugin.Provide[core.API](s, ...) so
// other golem plugins can interact with WorldEdit sessions, edits, schematics,
// and region restrictions.
type API interface {
	Session(p plugin.Player) *Session
	ConsoleSession() *Session
	Selection(p plugin.Player) (region.Region, error)
	NewEditSession(w plugin.World, actor plugin.CommandSender, command string) *EditSession
	SaveSchematic(name string, cb *storage.Clipboard, format storage.ClipboardFormat) (string, error)
	LoadSchematic(name string, format storage.ClipboardFormat) (*storage.Clipboard, error)
	RegisterRegionManager(m RegionManager)
	AllowedRegions(p plugin.Player, w plugin.World) []region.Region
	Config() *Config
	Engine() *WorldEdit
}

// EditSessionEvent fires before and after an EditSession commits changes.
// Cancelling a Pre-stage event aborts the edit.
type EditSessionEvent struct {
	plugin.Cancellable
	Stage   EditStage
	Actor   plugin.CommandSender
	World   plugin.World
	Command string
	Changed int64
}

// EditStage identifies when EditSessionEvent fires.
type EditStage uint8

const (
	EditStageBefore EditStage = iota
	EditStageAfter
)

// SchematicSaveEvent fires when a schematic is saved to disk.
type SchematicSaveEvent struct {
	plugin.Cancellable
	Actor  plugin.CommandSender
	Path   string
	Format storage.ClipboardFormat
}

// SchematicLoadEvent fires when a schematic is loaded into a clipboard.
type SchematicLoadEvent struct {
	plugin.Cancellable
	Actor  plugin.CommandSender
	Path   string
	Format storage.ClipboardFormat
}

// ClipboardPasteEvent fires before a clipboard is pasted into a world.
type ClipboardPasteEvent struct {
	plugin.Cancellable
	Actor  plugin.CommandSender
	World  plugin.World
	Target plugin.BlockPos
}

// RegionChangeEvent fires when a player's selection changes.
type RegionChangeEvent struct {
	plugin.Base
	Player   plugin.Player
	Selector region.Selector
}

func defineEvents(s plugin.Server) {
	plugin.DefineEvent[EditSessionEvent](s)
	plugin.DefineEvent[SchematicSaveEvent](s)
	plugin.DefineEvent[SchematicLoadEvent](s)
	plugin.DefineEvent[ClipboardPasteEvent](s)
	plugin.DefineEvent[RegionChangeEvent](s)
}
