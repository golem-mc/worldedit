package core

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/golem-mc/golem/plugin"
	"github.com/golem-mc/golem/registry"
	"github.com/golem-mc/worldedit/block"
	"github.com/golem-mc/worldedit/region"
	"github.com/golem-mc/worldedit/storage"
	"github.com/google/uuid"
)

// Version is the FastAsyncWorldEdit for golem plugin version string.
const Version = "2.12.0-golem"

// CUIChannel is the WorldEdit CUI plugin channel key (`worldedit:cui`).
var CUIChannel = plugin.Key{Namespace: "worldedit", Path: "cui"}

// WorldEdit is the central runtime state of the plugin.
type WorldEdit struct {
	Server    plugin.Server
	Log       *plugin.Logger
	StartedAt time.Time
	dataDir   string

	cfgMu sync.RWMutex
	cfg   *Config
	Msgs  *Messages

	sessMu     sync.RWMutex
	sessions   map[uuid.UUID]*Session
	consoleSes *Session

	dbMu sync.Mutex
	dbs  map[string]*storage.RollbackDB

	rmMu           sync.RWMutex
	regionManagers []RegionManager

	// Optional default tool handlers registered by the tools package.
	WandHandler      func(ctx ToolContext, leftClick bool) bool
	NavWandHandler   func(ctx ToolContext, leftClick bool) bool
	SuperPickHandler func(we *WorldEdit, sess *Session, e *plugin.BlockBreakEvent) bool

	// Tick limiter counters reset periodically.
	tickMu       sync.Mutex
	tickFalling  map[plugin.ChunkPos]int
	tickItems    map[plugin.ChunkPos]int
	tickResetTsk plugin.Task

	// Command log file handle (when worldedit-config.yml logging.log-commands is true).
	logMu   sync.Mutex
	cmdLogF *os.File

	TraceEnabled atomic.Bool
	stopping     atomic.Bool
}

var (
	instMu   sync.Mutex
	instance *WorldEdit
)

// Get returns the active WorldEdit instance, if enabled.
func Get() (*WorldEdit, bool) {
	instMu.Lock()
	defer instMu.Unlock()
	return instance, instance != nil
}

// Enable initializes FastAsyncWorldEdit on server s, resolves DataDir to an
// absolute path, loads config/translations, opens rollback databases, registers
// permissions, events, listeners, CUI channel, and provides core.API.
func Enable(s plugin.Server) (*WorldEdit, error) {
	dataDir := s.DataDir()
	if abs, err := filepath.Abs(dataDir); err == nil {
		dataDir = abs
	}
	cfg, err := LoadConfig(dataDir)
	if err != nil {
		return nil, fmt.Errorf("worldedit: load config: %w", err)
	}
	msgs := LoadMessages(dataDir)

	we := &WorldEdit{
		Server:      s,
		Log:         s.Logger(),
		StartedAt:   time.Now(),
		dataDir:     dataDir,
		cfg:         cfg,
		Msgs:        msgs,
		sessions:    make(map[uuid.UUID]*Session),
		consoleSes:  NewSession(uuid.Nil, "CONSOLE"),
		dbs:         make(map[string]*storage.RollbackDB),
		tickFalling: make(map[plugin.ChunkPos]int),
		tickItems:   make(map[plugin.ChunkPos]int),
	}

	// Ensure standard FAWE/WorldEdit subdirectories exist.
	for _, dir := range []string{
		we.SchematicsDir(),
		we.HistoryDir(),
		we.ClipboardDir(),
		we.HeightmapDir(),
		we.BrushesDir(),
		we.ScriptsDir(),
	} {
		_ = os.MkdirAll(dir, 0o755)
	}

	// Register permissions and custom events.
	RegisterPermissions(s, cfg.WorldEdit.NoOpPermissions)
	defineEvents(s)

	// Open SQLite rollback database for loaded worlds when enabled.
	if cfg.FAWE.History.UseDatabase {
		for _, w := range s.Worlds() {
			if _, err := we.openRollbackDB(w.Key().String()); err != nil {
				we.Log.Warn("failed to open rollback database", "world", w.Key().String(), "err", err)
			}
		}
	}

	// Purge expired disk history files in background if configured.
	if cfg.FAWE.History.UseDisk && cfg.FAWE.History.DeleteAfterDays > 0 {
		hDir := we.HistoryDir()
		days := cfg.FAWE.History.DeleteAfterDays
		s.Scheduler().RunAsync(func() {
			storage.PurgeOldHistory(hDir, time.Duration(days)*24*time.Hour)
		})
	}

	// Open command log file if configured.
	we.reopenCommandLog()

	// Register WorldEdit CUI plugin messaging channel.
	s.Messenger().Listen(CUIChannel, func(from plugin.Player, data []byte) {
		msg := strings.TrimSpace(string(data))
		if after, ok := strings.CutPrefix(msg, "v|"); ok {
			ver, _ := strconv.Atoi(after)
			sess := we.Session(from)
			sess.SetCUI(ver)
			we.DispatchCUI(from, sess)
		}
	})

	// Register server/player/block listeners.
	we.registerListeners()

	// Register API service via plugin.Provide.
	plugin.Provide[API](s, we, 0)

	instMu.Lock()
	instance = we
	instMu.Unlock()

	we.Log.Info("FastAsyncWorldEdit enabled", "version", Version, "dir", dataDir)
	return we, nil
}

// Disable shuts down background tasks, closes SQLite rollback databases and logs.
func (we *WorldEdit) Disable() {
	if !we.stopping.CompareAndSwap(false, true) {
		return
	}
	if we.tickResetTsk != nil {
		we.tickResetTsk.Cancel()
	}
	we.dbMu.Lock()
	for k, db := range we.dbs {
		_ = db.Close()
		delete(we.dbs, k)
	}
	we.dbMu.Unlock()

	we.logMu.Lock()
	if we.cmdLogF != nil {
		_ = we.cmdLogF.Close()
		we.cmdLogF = nil
	}
	we.logMu.Unlock()

	instMu.Lock()
	if instance == we {
		instance = nil
	}
	instMu.Unlock()
}

// Reload reloads config.yml, worldedit-config.yml, and lang/strings.json.
func (we *WorldEdit) Reload() error {
	cfg, err := LoadConfig(we.dataDir)
	if err != nil {
		return err
	}
	msgs := LoadMessages(we.dataDir)
	we.cfgMu.Lock()
	we.cfg = cfg
	we.Msgs = msgs
	we.cfgMu.Unlock()
	we.reopenCommandLog()
	return nil
}

func (we *WorldEdit) reopenCommandLog() {
	we.logMu.Lock()
	defer we.logMu.Unlock()
	if we.cmdLogF != nil {
		_ = we.cmdLogF.Close()
		we.cmdLogF = nil
	}
	cfg := we.Config()
	if !cfg.WorldEdit.Logging.LogCommands || cfg.WorldEdit.Logging.File == "" {
		return
	}
	p := cfg.WorldEdit.Logging.File
	if !filepath.IsAbs(p) {
		p = filepath.Join(we.dataDir, p)
	}
	f, err := os.OpenFile(p, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err == nil {
		we.cmdLogF = f
	}
}

// LogCommand appends an entry to worldedit.log when logging.log-commands is enabled.
func (we *WorldEdit) LogCommand(sender plugin.CommandSender, line string) {
	we.logMu.Lock()
	defer we.logMu.Unlock()
	if we.cmdLogF == nil {
		return
	}
	_, _ = fmt.Fprintf(we.cmdLogF, "[%s] %s: %s\n", time.Now().Format("2006-01-02 15:04:05"), sender.Name(), line)
}

// DataDir returns the resolved absolute data directory.
func (we *WorldEdit) DataDir() string { return we.dataDir }

func (we *WorldEdit) resolvePath(rel string) string {
	if filepath.IsAbs(rel) {
		return rel
	}
	return filepath.Join(we.dataDir, rel)
}

func (we *WorldEdit) SchematicsDir() string { return we.resolvePath(we.Config().WorldEdit.Saving.Dir) }
func (we *WorldEdit) HistoryDir() string    { return we.resolvePath(we.Config().FAWE.Paths.History) }
func (we *WorldEdit) ClipboardDir() string  { return we.resolvePath(we.Config().FAWE.Paths.Clipboard) }
func (we *WorldEdit) HeightmapDir() string  { return we.resolvePath(we.Config().FAWE.Paths.Heightmap) }
func (we *WorldEdit) BrushesDir() string    { return we.resolvePath("brushes") }
func (we *WorldEdit) ScriptsDir() string    { return we.resolvePath(we.Config().WorldEdit.Scripting.Dir) }
func (we *WorldEdit) SnapshotsDir() string {
	d := we.Config().WorldEdit.Snapshots.Directory
	if d == "" {
		return ""
	}
	return we.resolvePath(d)
}

// Config implements API.
func (we *WorldEdit) Config() *Config {
	we.cfgMu.RLock()
	defer we.cfgMu.RUnlock()
	return we.cfg
}

// Engine implements API.
func (we *WorldEdit) Engine() *WorldEdit { return we }

// Session implements API.
func (we *WorldEdit) Session(p plugin.Player) *Session {
	if p == nil {
		return we.consoleSes
	}
	uid := p.UUID()
	we.sessMu.RLock()
	s, ok := we.sessions[uid]
	we.sessMu.RUnlock()
	if ok {
		return s
	}
	we.sessMu.Lock()
	defer we.sessMu.Unlock()
	if s, ok = we.sessions[uid]; ok {
		return s
	}
	s = NewSession(uid, p.Name())
	we.sessions[uid] = s
	return s
}

// SessionByUUID looks up an existing session by UUID.
func (we *WorldEdit) SessionByUUID(uid uuid.UUID) (*Session, bool) {
	we.sessMu.RLock()
	defer we.sessMu.RUnlock()
	s, ok := we.sessions[uid]
	return s, ok
}

// ConsoleSession implements API.
func (we *WorldEdit) ConsoleSession() *Session { return we.consoleSes }

// Selection implements API.
func (we *WorldEdit) Selection(p plugin.Player) (region.Region, error) {
	if p == nil {
		return nil, region.ErrIncomplete
	}
	sess := we.Session(p)
	return sess.Selection(we.ResolveWorld(p, sess))
}

// RegisterRegionManager implements API.
func (we *WorldEdit) RegisterRegionManager(m RegionManager) {
	if m == nil {
		return
	}
	we.rmMu.Lock()
	defer we.rmMu.Unlock()
	we.regionManagers = append(we.regionManagers, m)
}

// AllowedRegions implements API.
func (we *WorldEdit) AllowedRegions(p plugin.Player, w plugin.World) []region.Region {
	we.rmMu.RLock()
	rms := append([]RegionManager(nil), we.regionManagers...)
	we.rmMu.RUnlock()
	var out []region.Region
	for _, rm := range rms {
		out = append(out, rm.AllowedRegions(p, w)...)
	}
	return out
}

// RollbackDB returns (opening on demand) the SQLite rollback DB for worldName.
func (we *WorldEdit) RollbackDB(worldName string) *storage.RollbackDB {
	if !we.Config().FAWE.History.UseDatabase {
		return nil
	}
	db, err := we.openRollbackDB(worldName)
	if err != nil {
		we.Log.Warn("open rollback db failed", "world", worldName, "err", err)
		return nil
	}
	return db
}

func (we *WorldEdit) openRollbackDB(worldName string) (*storage.RollbackDB, error) {
	key := storage.SanitizeWorldDir(worldName)
	we.dbMu.Lock()
	defer we.dbMu.Unlock()
	if db, ok := we.dbs[key]; ok {
		return db, nil
	}
	db, err := storage.OpenRollbackDB(we.HistoryDir(), worldName, we.Config().FAWE.History.DeleteAfterDays)
	if err != nil {
		return nil, err
	}
	we.dbs[key] = db
	return db, nil
}

// SaveSchematic implements API.
func (we *WorldEdit) SaveSchematic(name string, cb *storage.Clipboard, format storage.ClipboardFormat) (string, error) {
	if format == "" {
		format = storage.FormatFastV3
	}
	ext := "schem"
	for _, f := range storage.AllFormats {
		if f.Format == format {
			ext = f.Extension
			break
		}
	}
	clean := filepath.Clean(name)
	if strings.Contains(clean, "..") {
		return "", fmt.Errorf("invalid schematic path %q", name)
	}
	if !strings.HasSuffix(strings.ToLower(clean), "."+ext) {
		clean += "." + ext
	}
	full := filepath.Join(we.SchematicsDir(), clean)
	if err := storage.SaveSchematicFile(full, cb, format); err != nil {
		return "", err
	}
	return full, nil
}

// LoadSchematic implements API.
func (we *WorldEdit) LoadSchematic(name string, format storage.ClipboardFormat) (*storage.Clipboard, error) {
	clean := filepath.Clean(name)
	if strings.Contains(clean, "..") {
		return nil, fmt.Errorf("invalid schematic path %q", name)
	}
	dir := we.SchematicsDir()
	candidates := []string{
		filepath.Join(dir, clean),
		filepath.Join(dir, clean+".schem"),
		filepath.Join(dir, clean+".schematic"),
		filepath.Join(dir, clean+".nbt"),
	}
	for _, p := range candidates {
		if fi, err := os.Stat(p); err == nil && !fi.IsDir() {
			return storage.ReadSchematicFile(p, format)
		}
	}
	return nil, os.ErrNotExist
}

// DispatchCUI sends selection packets over the `worldedit:cui` plugin channel to a CUI-capable client.
func (we *WorldEdit) DispatchCUI(p plugin.Player, sess *Session) {
	if p == nil || sess == nil || !sess.CUIReady() || !sess.DrawSel() {
		return
	}
	sel := sess.Selector(we.ResolveWorld(p, sess))
	for _, ev := range sel.CUIEvents() {
		we.Server.Messenger().Send(p, CUIChannel, []byte(ev))
	}
}

// WandItem resolves the configured selection wand item ID.
func (we *WorldEdit) WandItem() (registry.Item, bool) {
	k, err := plugin.ParseKey(we.Config().WorldEdit.WandItem)
	if err != nil {
		return 0, false
	}
	return we.Server.Registries().Item(k)
}

// NavWandItem resolves the configured navigation wand item ID.
func (we *WorldEdit) NavWandItem() (registry.Item, bool) {
	k, err := plugin.ParseKey(we.Config().WorldEdit.NavigationWand.Item)
	if err != nil {
		return 0, false
	}
	return we.Server.Registries().Item(k)
}

func (we *WorldEdit) registerListeners() {
	// WorldLoadEvent -> open rollback DB
	plugin.Listen(we.Server, func(e *plugin.WorldLoadEvent) {
		if we.Config().FAWE.History.UseDatabase {
			_, _ = we.openRollbackDB(e.World.Key().String())
		}
	})

	// PlayerQuitEvent -> optional history/clipboard cleanup on logout
	plugin.Listen(we.Server, func(e *plugin.PlayerQuitEvent) {
		cfg := we.Config()
		if sess, ok := we.SessionByUUID(e.Player.UUID()); ok {
			sess.CancelActive()
			if cfg.FAWE.History.DeleteOnLogout {
				sess.ClearHistory()
			}
			if cfg.FAWE.Clipboard.DeleteOnLogout {
				sess.SetClipboard(nil)
			}
		}
	})

	// PlayerInteractEvent -> tools, brushes, selection wand, navigation wand
	plugin.Listen(we.Server, func(e *plugin.PlayerInteractEvent) {
		if e.Hand != plugin.MainHand || e.Action == plugin.Physical {
			return
		}
		if e.Item.Empty() {
			return
		}
		p := e.Player
		sess := we.Session(p)
		isLeft := e.Action == plugin.LeftClickBlock || e.Action == plugin.LeftClickAir
		ctx := ToolContext{
			WE:       we,
			Session:  sess,
			Player:   p,
			World:    p.World(),
			Clicked:  e.Block,
			Face:     e.Face,
			Item:     e.Item,
			Sneaking: p.Sneaking(),
		}

		// 1. Bound tool or brush on this item
		if tool, ok := sess.Tool(e.Item.Item); ok {
			var handled bool
			if isLeft {
				handled = tool.Primary(ctx)
			} else {
				handled = tool.Secondary(ctx)
			}
			if handled {
				e.SetCancelled(true)
			}
			return
		}

		// 2. Default selection wand (wooden_axe)
		if !sess.WandDisabled() {
			if wandItem, ok := we.WandItem(); ok && e.Item.Item == wandItem && p.HasPermission("worldedit.selection.pos") {
				if e.Block != nil && we.WandHandler != nil {
					if we.WandHandler(ctx, isLeft) {
						e.SetCancelled(true)
					}
					return
				}
			}
		}

		// 3. Default navigation wand (compass)
		if navItem, ok := we.NavWandItem(); ok && e.Item.Item == navItem {
			if (isLeft && p.HasPermission("worldedit.navigation.jumpto.tool")) ||
				(!isLeft && p.HasPermission("worldedit.navigation.thru.tool")) {
				if we.NavWandHandler != nil && we.NavWandHandler(ctx, isLeft) {
					e.SetCancelled(true)
				}
			}
		}
	})

	// BlockBreakEvent -> Super-Pickaxe & creative wand left-click guard
	plugin.Listen(we.Server, func(e *plugin.BlockBreakEvent) {
		p := e.Player
		sess := we.Session(p)
		held := p.Inventory().Get(plugin.SlotMainHand)
		if !held.Empty() {
			// Guard against breaking blocks in creative mode while holding the selection wand or a bound tool.
			if _, ok := sess.Tool(held.Item); ok {
				e.SetCancelled(true)
				return
			}
			if !sess.WandDisabled() {
				if wandItem, ok := we.WandItem(); ok && held.Item == wandItem && p.HasPermission("worldedit.selection.pos") {
					if we.WandHandler != nil {
						blk := e.Block
						ctx := ToolContext{
							WE:      we,
							Session: sess,
							Player:  p,
							World:   p.World(),
							Clicked: &blk,
							Item:    held,
						}
						we.WandHandler(ctx, true)
					}
					e.SetCancelled(true)
					return
				}
			}
		}
		if enabled, _, _ := sess.SuperPickaxe(); enabled && we.SuperPickHandler != nil {
			if we.SuperPickHandler(we, sess, e) {
				e.SetCancelled(true)
			}
		}
	})

	// Optional FAWE Tick Limiter
	if tl := we.Config().FAWE.TickLimiter; tl.Enabled && tl.Interval > 0 {
		we.tickResetTsk = we.Server.Scheduler().RunTimer(tl.Interval, tl.Interval, func() {
			we.tickMu.Lock()
			clear(we.tickFalling)
			clear(we.tickItems)
			we.tickMu.Unlock()
		})
		plugin.Listen(we.Server, func(e *plugin.EntitySpawnEvent) {
			cfg := we.Config().FAWE.TickLimiter
			if !cfg.Enabled {
				return
			}
			cp := e.Entity.Pos().Block().Chunk()
			we.tickMu.Lock()
			defer we.tickMu.Unlock()
			switch e.Entity.(type) {
			case plugin.FallingBlock:
				we.tickFalling[cp]++
				if cfg.Falling > 0 && we.tickFalling[cp] > cfg.Falling {
					e.SetCancelled(true)
				}
			case plugin.ItemEntity:
				we.tickItems[cp]++
				if cfg.Items > 0 && we.tickItems[cp] > cfg.Items {
					e.SetCancelled(true)
				}
			}
		})
	}
}

// Ensure block package is referenced for API consumers.
var _ block.Mask
