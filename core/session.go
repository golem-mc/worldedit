package core

import (
	"context"
	"sync"
	"time"

	"github.com/golem-mc/golem/plugin"
	"github.com/golem-mc/golem/registry"
	"github.com/golem-mc/worldedit/block"
	"github.com/golem-mc/worldedit/region"
	"github.com/golem-mc/worldedit/storage"
	"github.com/google/uuid"
)

// ToolContext carries interaction details when a player uses a bound tool or brush.
type ToolContext struct {
	WE       *WorldEdit
	Session  *Session
	Player   plugin.Player
	World    plugin.World
	Clicked  *plugin.Block
	Face     plugin.BlockFace
	Item     plugin.ItemStack
	Sneaking bool
}

// Tool is any WorldEdit/FAWE tool or brush bound to an item in a player's session.
type Tool interface {
	Name() string
	Primary(ctx ToolContext) bool   // left-click
	Secondary(ctx ToolContext) bool // right-click
}

// SuperPickaxeMode selects the super-pickaxe behavior.
type SuperPickaxeMode string

const (
	SuperPickSingle    SuperPickaxeMode = "single"
	SuperPickArea      SuperPickaxeMode = "area"
	SuperPickRecursive SuperPickaxeMode = "recursive"
)

// Session holds all per-player (or console) WorldEdit/FAWE editing state.
type Session struct {
	mu         sync.RWMutex
	PlayerUUID uuid.UUID
	PlayerName string

	selectors map[plugin.Key]region.Selector
	selType   region.SelectorType

	clipboard *storage.ClipboardHolder

	history    []*storage.ChangeSet
	historyIdx int // points to the next undo slot (len(history) == nothing undone yet)
	nextEditID int

	globalMask      block.Mask
	globalSrcMask   block.Mask
	globalTransform block.Transform
	globalTexture   string

	fastMode        bool
	bypassRegions   bool
	placeAtPos1     bool
	drawSel         bool
	tipsDisabled    bool
	blockLimit      int64 // -1 = use config limit
	timeoutMS       int   // -1 = use config timeout
	reorderMode     string
	perfSideEffects bool
	worldOverride   plugin.World
	wandDisabled    bool

	superPickEnabled bool
	superPickMode    SuperPickaxeMode
	superPickRadius  int

	tools map[registry.Item]Tool

	pendingConfirm     func()
	pendingConfirmDesc string

	cancelMu     sync.Mutex
	activeCancel context.CancelFunc

	timezone *time.Location
	snapshot *storage.Snapshot

	cuiReady bool
	cuiVer   int

	lastScript     string
	lastScriptArgs []string
}

// NewSession constructs a new editing session for uid/name.
func NewSession(uid uuid.UUID, name string) *Session {
	return &Session{
		PlayerUUID:      uid,
		PlayerName:      name,
		selectors:       make(map[plugin.Key]region.Selector),
		selType:         region.SelectorCuboid,
		blockLimit:      -1,
		timeoutMS:       -1,
		reorderMode:     "fast",
		perfSideEffects: true,
		drawSel:         true,
		superPickMode:   SuperPickSingle,
		superPickRadius: 1,
		tools:           make(map[registry.Item]Tool),
		timezone:        time.Local,
	}
}

// Selector returns the active region selector for w, creating one of the session's type if needed.
func (s *Session) Selector(w plugin.World) region.Selector {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := plugin.MinecraftKey("overworld")
	if w != nil {
		key = w.Key()
	}
	sel, ok := s.selectors[key]
	if !ok {
		sel = region.NewSelector(s.selType)
		s.selectors[key] = sel
	}
	return sel
}

// SetSelectorType switches the selector type for w and clears the previous selection.
func (s *Session) SetSelectorType(w plugin.World, t region.SelectorType) region.Selector {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.selType = t
	key := plugin.MinecraftKey("overworld")
	if w != nil {
		key = w.Key()
	}
	sel := region.NewSelector(t)
	s.selectors[key] = sel
	return sel
}

// Selection returns the currently defined Region in w, or region.ErrIncomplete.
func (s *Session) Selection(w plugin.World) (region.Region, error) {
	return s.Selector(w).Region()
}

// SetClipboard sets the player's active ClipboardHolder.
func (s *Session) SetClipboard(h *storage.ClipboardHolder) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.clipboard = h
}

// Clipboard returns the player's ClipboardHolder (may be nil).
func (s *Session) Clipboard() *storage.ClipboardHolder {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.clipboard
}

// PushHistory records a committed ChangeSet into the undo stack (truncating any redo tail).
func (s *Session) PushHistory(cs *storage.ChangeSet, maxSize int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.historyIdx < len(s.history) {
		s.history = s.history[:s.historyIdx]
	}
	s.history = append(s.history, cs)
	if maxSize > 0 && len(s.history) > maxSize {
		excess := len(s.history) - maxSize
		s.history = s.history[excess:]
	}
	s.historyIdx = len(s.history)
}

// PopUndo returns the next ChangeSet to undo, or nil if at the bottom of history.
func (s *Session) PopUndo() *storage.ChangeSet {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.historyIdx <= 0 {
		return nil
	}
	s.historyIdx--
	return s.history[s.historyIdx]
}

// PopRedo returns the next ChangeSet to redo, or nil if at the top of history.
func (s *Session) PopRedo() *storage.ChangeSet {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.historyIdx >= len(s.history) {
		return nil
	}
	cs := s.history[s.historyIdx]
	s.historyIdx++
	return cs
}

// ClearHistory empties the in-memory undo/redo stack.
func (s *Session) ClearHistory() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.history = nil
	s.historyIdx = 0
}

// HistoryList returns a snapshot of the session's undo history and current pointer.
func (s *Session) HistoryList() ([]*storage.ChangeSet, int) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*storage.ChangeSet, len(s.history))
	copy(out, s.history)
	return out, s.historyIdx
}

func (s *Session) NextEditID(historyDir, worldName string, useDisk bool) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.nextEditID == 0 && useDisk && historyDir != "" {
		s.nextEditID = storage.NextHistoryID(historyDir, worldName, s.PlayerUUID)
	}
	id := s.nextEditID
	s.nextEditID++
	return id
}

func (s *Session) GlobalMask() block.Mask     { s.mu.RLock(); defer s.mu.RUnlock(); return s.globalMask }
func (s *Session) SetGlobalMask(m block.Mask) { s.mu.Lock(); defer s.mu.Unlock(); s.globalMask = m }
func (s *Session) GlobalSourceMask() block.Mask {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.globalSrcMask
}
func (s *Session) SetGlobalSourceMask(m block.Mask) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.globalSrcMask = m
}
func (s *Session) GlobalTransform() block.Transform {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.globalTransform
}
func (s *Session) SetGlobalTransform(t block.Transform) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.globalTransform = t
}
func (s *Session) GlobalTexture() string     { s.mu.RLock(); defer s.mu.RUnlock(); return s.globalTexture }
func (s *Session) SetGlobalTexture(t string) { s.mu.Lock(); defer s.mu.Unlock(); s.globalTexture = t }

func (s *Session) FastMode() bool          { s.mu.RLock(); defer s.mu.RUnlock(); return s.fastMode }
func (s *Session) SetFastMode(v bool)      { s.mu.Lock(); defer s.mu.Unlock(); s.fastMode = v }
func (s *Session) BypassRegions() bool     { s.mu.RLock(); defer s.mu.RUnlock(); return s.bypassRegions }
func (s *Session) SetBypassRegions(v bool) { s.mu.Lock(); defer s.mu.Unlock(); s.bypassRegions = v }
func (s *Session) PlaceAtPos1() bool       { s.mu.RLock(); defer s.mu.RUnlock(); return s.placeAtPos1 }
func (s *Session) SetPlaceAtPos1(v bool)   { s.mu.Lock(); defer s.mu.Unlock(); s.placeAtPos1 = v }
func (s *Session) DrawSel() bool           { s.mu.RLock(); defer s.mu.RUnlock(); return s.drawSel }
func (s *Session) SetDrawSel(v bool)       { s.mu.Lock(); defer s.mu.Unlock(); s.drawSel = v }
func (s *Session) TipsDisabled() bool      { s.mu.RLock(); defer s.mu.RUnlock(); return s.tipsDisabled }
func (s *Session) SetTipsDisabled(v bool)  { s.mu.Lock(); defer s.mu.Unlock(); s.tipsDisabled = v }
func (s *Session) BlockLimit() int64       { s.mu.RLock(); defer s.mu.RUnlock(); return s.blockLimit }
func (s *Session) SetBlockLimit(v int64)   { s.mu.Lock(); defer s.mu.Unlock(); s.blockLimit = v }
func (s *Session) TimeoutMS() int          { s.mu.RLock(); defer s.mu.RUnlock(); return s.timeoutMS }
func (s *Session) SetTimeoutMS(v int)      { s.mu.Lock(); defer s.mu.Unlock(); s.timeoutMS = v }
func (s *Session) ReorderMode() string     { s.mu.RLock(); defer s.mu.RUnlock(); return s.reorderMode }
func (s *Session) SetReorderMode(v string) { s.mu.Lock(); defer s.mu.Unlock(); s.reorderMode = v }
func (s *Session) PerfSideEffects() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.perfSideEffects
}
func (s *Session) SetPerfSideEffects(v bool) { s.mu.Lock(); defer s.mu.Unlock(); s.perfSideEffects = v }
func (s *Session) WorldOverride() plugin.World {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.worldOverride
}
func (s *Session) SetWorldOverride(w plugin.World) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.worldOverride = w
}
func (s *Session) WandDisabled() bool     { s.mu.RLock(); defer s.mu.RUnlock(); return s.wandDisabled }
func (s *Session) SetWandDisabled(v bool) { s.mu.Lock(); defer s.mu.Unlock(); s.wandDisabled = v }

func (s *Session) SuperPickaxe() (bool, SuperPickaxeMode, int) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.superPickEnabled, s.superPickMode, s.superPickRadius
}

func (s *Session) SetSuperPickaxe(enabled bool, mode SuperPickaxeMode, radius int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.superPickEnabled = enabled
	if mode != "" {
		s.superPickMode = mode
	}
	if radius > 0 {
		s.superPickRadius = radius
	}
}

func (s *Session) Tool(item registry.Item) (Tool, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	t, ok := s.tools[item]
	return t, ok
}

func (s *Session) SetTool(item registry.Item, t Tool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if t == nil {
		delete(s.tools, item)
		return
	}
	s.tools[item] = t
}

func (s *Session) SetPendingConfirm(desc string, fn func()) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pendingConfirmDesc = desc
	s.pendingConfirm = fn
}

func (s *Session) TakePendingConfirm() (string, func()) {
	s.mu.Lock()
	defer s.mu.Unlock()
	desc, fn := s.pendingConfirmDesc, s.pendingConfirm
	s.pendingConfirmDesc, s.pendingConfirm = "", nil
	return desc, fn
}

func (s *Session) StartCancellable() (context.Context, func()) {
	ctx, cancel := context.WithCancel(context.Background())
	s.cancelMu.Lock()
	s.activeCancel = cancel
	s.cancelMu.Unlock()
	return ctx, func() {
		cancel()
		s.cancelMu.Lock()
		s.activeCancel = nil
		s.cancelMu.Unlock()
	}
}

func (s *Session) CancelActive() bool {
	s.cancelMu.Lock()
	cancel := s.activeCancel
	s.activeCancel = nil
	s.cancelMu.Unlock()
	if cancel != nil {
		cancel()
		return true
	}
	return false
}

func (s *Session) Timezone() *time.Location       { s.mu.RLock(); defer s.mu.RUnlock(); return s.timezone }
func (s *Session) SetTimezone(loc *time.Location) { s.mu.Lock(); defer s.mu.Unlock(); s.timezone = loc }
func (s *Session) Snapshot() *storage.Snapshot    { s.mu.RLock(); defer s.mu.RUnlock(); return s.snapshot }
func (s *Session) SetSnapshot(sn *storage.Snapshot) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.snapshot = sn
}
func (s *Session) SetCUI(ver int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cuiReady = true
	s.cuiVer = ver
}
func (s *Session) CUIReady() bool { s.mu.RLock(); defer s.mu.RUnlock(); return s.cuiReady }

func (s *Session) SetLastScript(name string, args []string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lastScript = name
	s.lastScriptArgs = append([]string(nil), args...)
}

func (s *Session) LastScript() (string, []string) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.lastScript, append([]string(nil), s.lastScriptArgs...)
}
