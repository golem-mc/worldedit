package core

import (
	"context"
	"errors"
	"fmt"
	"math"
	"slices"
	"time"

	"github.com/golem-mc/golem/nbt"
	"github.com/golem-mc/golem/plugin"
	"github.com/golem-mc/golem/registry"
	"github.com/golem-mc/worldedit/block"
	"github.com/golem-mc/worldedit/region"
	"github.com/golem-mc/worldedit/storage"
)

// ErrLimitExceeded is returned when an edit exceeds the actor's FAWE block/check/fail limit.
var ErrLimitExceeded = errors.New("maximum block change limit reached")

// ErrCancelled is returned when an edit is cancelled via //cancel or an EditSessionEvent listener.
var ErrCancelled = errors.New("edit cancelled")

// EditSession manages a transactional batch of world edits with FAWE limits,
// masks, transforms, undo/redo recording, block entity preservation, and rollback logging.
type EditSession struct {
	WE      *WorldEdit
	World   plugin.World
	Actor   plugin.CommandSender
	Player  plugin.Player // nil for console
	Session *Session
	Command string
	Limit   *EffectiveLimit

	AllowedRegions []region.Region
	DestMask       block.Mask
	SourceMask     block.Mask
	Transform      block.Transform
	Origin         plugin.BlockPos
	FastMode       bool
	RecordHistory  bool
	Ctx            context.Context

	changeSet    *storage.ChangeSet
	changedCount int64
	checkedCount int64
	failedCount  int
	limitHit     bool
	aborted      bool
}

// NewEditSession constructs a new EditSession for world w and actor.
func (we *WorldEdit) NewEditSession(w plugin.World, actor plugin.CommandSender, command string) *EditSession {
	cfg := we.Config()
	var sess *Session
	var p plugin.Player
	if pl, ok := actor.(plugin.Player); ok {
		p = pl
		sess = we.Session(pl)
	} else {
		sess = we.ConsoleSession()
	}

	lim := cfg.LimitFor(actor)
	if sess.BlockLimit() >= 0 && sess.BlockLimit() < lim.MaxChanges {
		cp := *lim
		cp.MaxChanges = sess.BlockLimit()
		cp.Unlimited = false
		lim = &cp
	}

	var allowed []region.Region
	if p != nil && cfg.FAWE.RegionRestrictions && !sess.BypassRegions() && !p.HasPermission("fawe.bypass.regions") && !p.HasPermission("fawe.bypass") {
		allowed = we.AllowedRegions(p, w)
	}

	fastMode := sess.FastMode() || !sess.PerfSideEffects()
	recordHistory := !sess.FastMode()
	if p == nil && !cfg.FAWE.History.EnableForConsole {
		recordHistory = false
	}

	var cs *storage.ChangeSet
	if recordHistory {
		editID := sess.NextEditID(we.HistoryDir(), w.Key().String(), cfg.FAWE.History.UseDisk)
		diskPath := ""
		if cfg.FAWE.History.UseDisk {
			diskPath = storage.HistoryFilePath(we.HistoryDir(), w.Key().String(), sess.PlayerUUID, editID)
		}
		cs = storage.NewChangeSet(editID, sess.PlayerUUID, w.Key().String(), command, cfg.FAWE.History.StoreRedo, diskPath)
	}

	origin := plugin.BlockPos{}
	if p != nil {
		origin = p.Pos().Block()
	}

	return &EditSession{
		WE:             we,
		World:          w,
		Actor:          actor,
		Player:         p,
		Session:        sess,
		Command:        command,
		Limit:          lim,
		AllowedRegions: allowed,
		DestMask:       sess.GlobalMask(),
		SourceMask:     sess.GlobalSourceMask(),
		Transform:      sess.GlobalTransform(),
		Origin:         origin,
		FastMode:       fastMode,
		RecordHistory:  recordHistory,
		Ctx:            context.Background(),
		changeSet:      cs,
	}
}

// BlockState implements block.Extent.
func (es *EditSession) BlockState(pos plugin.BlockPos) plugin.BlockState {
	es.checkedCount++
	if es.Limit != nil && !es.Limit.Unlimited && es.Limit.MaxChecks > 0 && es.checkedCount > es.Limit.MaxChecks {
		es.limitHit = true
	}
	return es.World.BlockState(pos)
}

// Biome implements block.Extent.
func (es *EditSession) Biome(pos plugin.BlockPos) registry.Biome {
	return es.World.Biome(pos)
}

// Changed returns the number of blocks modified so far.
func (es *EditSession) Changed() int64 { return es.changedCount }

// Failed returns the number of blocks skipped due to region restrictions or disallowed blocks.
func (es *EditSession) Failed() int { return es.failedCount }

// LimitHit reports whether the change limit was reached.
func (es *EditSession) LimitHit() bool { return es.limitHit }

// ChangeSet returns the underlying ChangeSet (may be nil if history is disabled).
func (es *EditSession) ChangeSet() *storage.ChangeSet { return es.changeSet }

func (es *EditSession) canEdit(pos plugin.BlockPos) bool {
	if es.Ctx != nil && es.Ctx.Err() != nil {
		es.aborted = true
		return false
	}
	if es.WE.Config().FAWE.RegionOptions.RestrictToSafeRange {
		if pos.X < -30000000 || pos.X > 30000000 || pos.Z < -30000000 || pos.Z > 30000000 {
			es.failedCount++
			return false
		}
	}
	if pos.Y < es.World.MinY() || pos.Y >= es.World.MinY()+es.World.Height() {
		return false
	}
	if len(es.AllowedRegions) > 0 {
		ok := false
		for _, r := range es.AllowedRegions {
			if r.Contains(pos) {
				ok = true
				break
			}
		}
		if !ok {
			es.failedCount++
			return false
		}
	}
	return true
}

// SetBlock sets a single block at pos, enforcing limits, masks, transforms, and recording history.
func (es *EditSession) SetBlock(pos plugin.BlockPos, st plugin.BlockState) bool {
	return es.SetBlockWithNBT(pos, st, nil)
}

// SetBlockWithNBT sets a block and applies optional block entity data (sign text, container contents, etc.).
func (es *EditSession) SetBlockWithNBT(pos plugin.BlockPos, st plugin.BlockState, beNBT nbt.Compound) bool {
	if es.aborted || es.limitHit {
		return false
	}
	if es.Transform != nil {
		pos, st = es.Transform.Apply(pos, st, es.Origin)
	}
	if !es.canEdit(pos) {
		return false
	}
	if es.DestMask != nil && !es.DestMask.Test(es, pos) {
		return false
	}
	if es.Limit != nil {
		if !es.Limit.Unlimited && es.changedCount >= es.Limit.MaxChanges {
			es.limitHit = true
			return false
		}
		if !es.Limit.Unlimited && es.Limit.MaxFails > 0 && es.failedCount >= es.Limit.MaxFails {
			es.limitHit = true
			return false
		}
		if es.Limit.IsDisallowed(st) && (es.Actor == nil || !es.Actor.HasPermission("worldedit.anyblock")) {
			es.failedCount++
			return false
		}
		st = es.Limit.RemapState(st)
		if beNBT != nil && len(es.Limit.StripNBT) > 0 {
			cp := nbt.Copy(beNBT).(nbt.Compound)
			for k := range es.Limit.StripNBT {
				delete(cp, k)
			}
			beNBT = cp
		}
	}

	oldState := es.World.BlockState(pos)
	if oldState == st && beNBT == nil {
		return false
	}

	// Enforce FAWE inventory-mode (1 = freebuild remove+place, 2 = survival place)
	if es.Player != nil && es.Limit != nil && es.Limit.InventoryMode > 0 && !es.Limit.Unlimited {
		if es.Player.GameMode() != plugin.Creative || es.WE.Config().WorldEdit.UseInventory.CreativeModeOverrides {
			if !block.IsAir(st) {
				if itemID, ok := registry.ItemByName(block.BlockName(st)); ok {
					missing := es.Player.Inventory().Remove(plugin.NewStack(itemID, 1))
					if len(missing) > 0 {
						es.failedCount++
						return false
					}
				}
			}
			if es.Limit.InventoryMode == 1 && !block.IsAir(oldState) {
				if itemID, ok := registry.ItemByName(block.BlockName(oldState)); ok {
					es.Player.Inventory().Add(plugin.NewStack(itemID, 1))
				}
			}
		}
	}
	var oldNBT nbt.Compound
	if es.RecordHistory {
		oldNBT = CaptureBlockEntityNBT(es.World, pos)
	}

	flags := plugin.UpdateAll
	if es.FastMode || (es.Limit != nil && es.Limit.FastPlacement) {
		flags = plugin.UpdateClients | plugin.UpdateKnownShape | plugin.UpdateSkipOnPlace | plugin.UpdateSuppressDrops
	}
	if !es.World.SetBlockState(pos, st, flags) {
		return false
	}
	if beNBT != nil {
		ApplyBlockEntityNBT(es.World, pos, beNBT)
	}
	if es.changeSet != nil {
		es.changeSet.Add(storage.BlockChange{
			Pos:     pos,
			From:    oldState,
			To:      st,
			FromNBT: oldNBT,
			ToNBT:   beNBT,
		})
	}
	es.changedCount++
	return true
}

// SetPattern sets a block at pos using pat (respecting SourceMask if set).
func (es *EditSession) SetPattern(pos plugin.BlockPos, pat block.Pattern) bool {
	if es.SourceMask != nil && !es.SourceMask.Test(es, pos) {
		return false
	}
	return es.SetBlock(pos, pat.Apply(es, pos))
}

// SetRegion fills every block in r with pat.
func (es *EditSession) SetRegion(r region.Region, pat block.Pattern) int64 {
	start := es.changedCount
	r.ForEach(func(pos plugin.BlockPos) bool {
		if es.aborted || es.limitHit {
			return false
		}
		es.SetPattern(pos, pat)
		return true
	})
	return es.changedCount - start
}

// ReplaceRegion replaces every block in r matching mask with pat.
func (es *EditSession) ReplaceRegion(r region.Region, mask block.Mask, pat block.Pattern) int64 {
	start := es.changedCount
	r.ForEach(func(pos plugin.BlockPos) bool {
		if es.aborted || es.limitHit {
			return false
		}
		if mask == nil || mask.Test(es, pos) {
			es.SetPattern(pos, pat)
		}
		return true
	})
	return es.changedCount - start
}

// Commit finalizes the EditSession, flushes disk history, logs to RollbackDB, and fires EditSessionEvent.
func (es *EditSession) Commit() error {
	if es.aborted {
		return ErrCancelled
	}
	preEv := &EditSessionEvent{
		Stage:   EditStageBefore,
		Actor:   es.Actor,
		World:   es.World,
		Command: es.Command,
		Changed: es.changedCount,
	}
	es.WE.Server.Events().Fire(preEv)
	if preEv.Cancelled() {
		return ErrCancelled
	}

	if es.changeSet != nil && es.changeSet.Size() > 0 {
		cfg := es.WE.Config()
		if cfg.FAWE.History.UseDisk {
			if err := es.changeSet.Flush(); err != nil {
				es.WE.Log.Warn("failed to flush disk history", "err", err)
			}
		}
		es.Session.PushHistory(es.changeSet, cfg.WorldEdit.History.Size)
		if cfg.FAWE.History.UseDatabase {
			if lo, hi, ok := es.changeSet.Bounds(); ok {
				if rdb := es.WE.RollbackDB(es.World.Key().String()); rdb != nil {
					_ = rdb.LogEdit(storage.EditSummary{
						Player:  es.changeSet.PlayerUUID,
						ID:      es.changeSet.ID,
						Time:    es.changeSet.Timestamp,
						Min:     lo,
						Max:     hi,
						Size:    es.changeSet.Size(),
						Command: es.changeSet.Command,
						World:   es.changeSet.WorldName,
					})
				}
			}
		}
	}

	postEv := &EditSessionEvent{
		Stage:   EditStageAfter,
		Actor:   es.Actor,
		World:   es.World,
		Command: es.Command,
		Changed: es.changedCount,
	}
	es.WE.Server.Events().Fire(postEv)
	return nil
}

// UndoChangeSet reverts all block changes recorded in cs in reverse order.
func (we *WorldEdit) UndoChangeSet(w plugin.World, cs *storage.ChangeSet, allowed []region.Region) (int64, error) {
	chs, err := cs.Changes()
	if err != nil {
		return 0, err
	}
	flags := plugin.UpdateClients | plugin.UpdateKnownShape | plugin.UpdateSkipOnPlace | plugin.UpdateSuppressDrops
	var count int64
	for _, ch := range slices.Backward(chs) {

		if len(allowed) > 0 && !posInRegions(ch.Pos, allowed) {
			continue
		}
		if w.SetBlockState(ch.Pos, ch.From, flags) {
			if ch.FromNBT != nil {
				ApplyBlockEntityNBT(w, ch.Pos, ch.FromNBT)
			}
			count++
		}
	}
	return count, nil
}

// RedoChangeSet re-applies all block changes recorded in cs in forward order.
func (we *WorldEdit) RedoChangeSet(w plugin.World, cs *storage.ChangeSet, allowed []region.Region) (int64, error) {
	chs, err := cs.Changes()
	if err != nil {
		return 0, err
	}
	flags := plugin.UpdateClients | plugin.UpdateKnownShape | plugin.UpdateSkipOnPlace | plugin.UpdateSuppressDrops
	var count int64
	for _, ch := range chs {
		if len(allowed) > 0 && !posInRegions(ch.Pos, allowed) {
			continue
		}
		if w.SetBlockState(ch.Pos, ch.To, flags) {
			if ch.ToNBT != nil {
				ApplyBlockEntityNBT(w, ch.Pos, ch.ToNBT)
			}
			count++
		}
	}
	return count, nil
}

func posInRegions(p plugin.BlockPos, regs []region.Region) bool {
	for _, r := range regs {
		if r.Contains(p) {
			return true
		}
	}
	return false
}

// CaptureBlockEntityNBT reads supported block entity state (Sign, CommandBlock, Spawner, Beacon, Container name)
// into a portable nbt.Compound for clipboards, schematics, and undo history.
func CaptureBlockEntityNBT(w plugin.World, pos plugin.BlockPos) nbt.Compound {
	be, ok := w.BlockEntity(pos)
	if !ok || be == nil || !be.IsValid() {
		return nil
	}
	comp := nbt.Compound{
		"id": nbt.String(be.Type().String()),
	}
	if n, ok := be.(plugin.Nameable); ok {
		if cn := n.CustomName(); cn != nil {
			comp["CustomName"] = nbt.String(cn.PlainString())
		}
	}
	switch v := be.(type) {
	case plugin.Sign:
		front := v.Lines(true)
		back := v.Lines(false)
		comp["front_lines"] = nbt.List{
			Elem: nbt.TypeString,
			Items: []nbt.Tag{
				nbt.String(front[0].PlainString()), nbt.String(front[1].PlainString()),
				nbt.String(front[2].PlainString()), nbt.String(front[3].PlainString()),
			},
		}
		comp["back_lines"] = nbt.List{
			Elem: nbt.TypeString,
			Items: []nbt.Tag{
				nbt.String(back[0].PlainString()), nbt.String(back[1].PlainString()),
				nbt.String(back[2].PlainString()), nbt.String(back[3].PlainString()),
			},
		}
		comp["front_glowing"] = nbt.Byte(boolByte(v.Glowing(true)))
		comp["back_glowing"] = nbt.Byte(boolByte(v.Glowing(false)))
		comp["waxed"] = nbt.Byte(boolByte(v.Waxed()))
	case plugin.CommandBlock:
		comp["Command"] = nbt.String(v.Command())
	case plugin.Spawner:
		comp["Delay"] = nbt.Short(v.Delay())
		comp["MinSpawnDelay"] = nbt.Short(v.MinDelay())
		comp["MaxSpawnDelay"] = nbt.Short(v.MaxDelay())
		comp["SpawnCount"] = nbt.Short(v.SpawnCount())
		comp["RequiredPlayerRange"] = nbt.Short(v.RequiredPlayerRange())
		comp["EntityType"] = nbt.Int(v.EntityType())
	case plugin.Beacon:
		p, s := v.Effects()
		comp["PrimaryEffect"] = nbt.Int(p)
		comp["SecondaryEffect"] = nbt.Int(s)
	case plugin.Container:
		if tbl, seed := v.LootTable(); tbl.Path != "" {
			comp["LootTable"] = nbt.String(tbl.String())
			comp["LootTableSeed"] = nbt.Long(seed)
		}
		if lock := v.Lock(); lock != nil {
			comp["lock"] = nbt.String(string(*lock))
		}
	}
	return comp
}

// ApplyBlockEntityNBT restores captured NBT fields onto the live block entity at pos.
func ApplyBlockEntityNBT(w plugin.World, pos plugin.BlockPos, comp nbt.Compound) {
	if comp == nil {
		return
	}
	be, ok := w.BlockEntity(pos)
	if !ok || be == nil || !be.IsValid() {
		return
	}
	if n, ok := be.(plugin.Nameable); ok {
		if cn, ok := nbt.Get[nbt.String](comp, "CustomName"); ok && cn != "" {
			t := plugin.Plain(string(cn))
			n.SetCustomName(&t)
		}
	}
	switch v := be.(type) {
	case plugin.Sign:
		if lst, ok := nbt.Get[nbt.List](comp, "front_lines"); ok && len(lst.Items) == 4 {
			var lines [4]plugin.Text
			for i := range 4 {
				if s, ok := lst.Items[i].(nbt.String); ok {
					lines[i] = plugin.Plain(string(s))
				}
			}
			v.SetLines(true, lines)
		}
		if lst, ok := nbt.Get[nbt.List](comp, "back_lines"); ok && len(lst.Items) == 4 {
			var lines [4]plugin.Text
			for i := range 4 {
				if s, ok := lst.Items[i].(nbt.String); ok {
					lines[i] = plugin.Plain(string(s))
				}
			}
			v.SetLines(false, lines)
		}
		v.SetGlowing(true, nbt.BoolField(comp, "front_glowing"))
		v.SetGlowing(false, nbt.BoolField(comp, "back_glowing"))
		v.SetWaxed(nbt.BoolField(comp, "waxed"))
	case plugin.CommandBlock:
		if cmd, ok := nbt.Get[nbt.String](comp, "Command"); ok {
			v.SetCommand(string(cmd))
		}
	case plugin.Spawner:
		if et, ok := nbt.IntField(comp, "EntityType"); ok {
			v.SetEntityType(registry.EntityType(et))
		}
		if d, ok := nbt.ShortField(comp, "Delay"); ok {
			v.SetDelay(int(d))
		}
		if lo, okLo := nbt.ShortField(comp, "MinSpawnDelay"); okLo {
			hi, _ := nbt.ShortField(comp, "MaxSpawnDelay")
			v.SetDelays(int(lo), int(hi))
		}
		if sc, ok := nbt.ShortField(comp, "SpawnCount"); ok {
			v.SetSpawnCount(int(sc))
		}
		if rpr, ok := nbt.ShortField(comp, "RequiredPlayerRange"); ok {
			v.SetRequiredPlayerRange(int(rpr))
		}
	case plugin.Beacon:
		p, okP := nbt.IntField(comp, "PrimaryEffect")
		s, okS := nbt.IntField(comp, "SecondaryEffect")
		if okP || okS {
			v.SetEffects(plugin.Effect(p), plugin.Effect(s))
		}
	case plugin.Container:
		if tbl, ok := nbt.Get[nbt.String](comp, "LootTable"); ok {
			if k, err := plugin.ParseKey(string(tbl)); err == nil {
				seed, _ := nbt.LongValue(comp["LootTableSeed"])
				v.SetLootTable(k, seed)
			}
		}
		if lk, ok := nbt.Get[nbt.String](comp, "lock"); ok {
			p := plugin.LockPredicate(lk)
			v.SetLock(&p)
		}
	}
}

func boolByte(b bool) int8 {
	if b {
		return 1
	}
	return 0
}

// ParseContextFor constructs a block.ParseContext populated from actor/session/world.
func (we *WorldEdit) ParseContextFor(actor plugin.CommandSender, w plugin.World, sess *Session) block.ParseContext {
	cfg := we.Config()
	lim := cfg.LimitFor(actor)
	ctx := block.ParseContext{
		Extent:      worldExtent{w},
		AllowLegacy: lim.AllowLegacy,
		ExprTimeout: time.Duration(max(50, lim.MaxExpressionMS)) * time.Millisecond,
	}
	if sess != nil {
		if sel, err := sess.Selection(w); err == nil {
			ctx.Selection = sel
		}
		ctx.DynamicRegion = func() region.Region {
			r, _ := sess.Selection(w)
			return r
		}
		if holder := sess.Clipboard(); holder != nil {
			if cb := holder.Primary(); cb != nil {
				ctx.Clipboard = cb
			}
		}
		if sess.TimeoutMS() > 0 {
			ctx.ExprTimeout = time.Duration(sess.TimeoutMS()) * time.Millisecond
		}
	}
	if p, ok := actor.(plugin.Player); ok {
		ctx.Origin = p.Pos().Block()
		inv := p.Inventory()
		held := inv.Get(plugin.SlotMainHand)
		if !held.Empty() {
			if k, ok := we.Server.Registries().KeyOf(held.Item); ok {
				if bname := registry.BlockOfItem(k.String()); bname != "" {
					if id, ok := registry.FindState(bname, nil); ok {
						ctx.HandBlock = plugin.BlockState(id)
						ctx.HasHandBlock = true
					}
				}
			}
		}
		for slot := range 9 {
			st := inv.Item(slot)
			if st.Empty() {
				continue
			}
			if k, ok := we.Server.Registries().KeyOf(st.Item); ok {
				if bname := registry.BlockOfItem(k.String()); bname != "" {
					if id, ok := registry.FindState(bname, nil); ok {
						ctx.HotbarBlocks = append(ctx.HotbarBlocks, plugin.BlockState(id))
					}
				}
			}
		}
	}
	return ctx
}

type worldExtent struct{ w plugin.World }

func (e worldExtent) BlockState(pos plugin.BlockPos) plugin.BlockState { return e.w.BlockState(pos) }
func (e worldExtent) Biome(pos plugin.BlockPos) registry.Biome         { return e.w.Biome(pos) }

// ResolveWorld returns the session's world override, or the player's world, or the overworld.
func (we *WorldEdit) ResolveWorld(actor plugin.CommandSender, sess *Session) plugin.World {
	if sess != nil && sess.WorldOverride() != nil {
		return sess.WorldOverride()
	}
	if p, ok := actor.(plugin.Player); ok && p.World() != nil {
		return p.World()
	}
	return we.Server.Overworld()
}

// PlacementPos returns either pos1 (if toggleplace is active) or the player's block position.
func (we *WorldEdit) PlacementPos(p plugin.Player, sess *Session) plugin.BlockPos {
	if sess != nil && sess.PlaceAtPos1() {
		if pos1, ok := sess.Selector(p.World()).PrimaryPos(); ok {
			return pos1
		}
	}
	return p.Pos().Block()
}

// FloodFill performs a 6-connected BFS starting at origin matching mask up to maxBlocks.
func FloodFill(w plugin.World, origin plugin.BlockPos, match func(pos plugin.BlockPos, st plugin.BlockState) bool, maxRadius, maxBlocks int) []plugin.BlockPos {
	if maxBlocks <= 0 {
		maxBlocks = 4096
	}
	visited := map[plugin.BlockPos]struct{}{origin: {}}
	queue := []plugin.BlockPos{origin}
	var out []plugin.BlockPos
	dirs := [6]plugin.BlockPos{{X: 1}, {X: -1}, {Y: 1}, {Y: -1}, {Z: 1}, {Z: -1}}
	r2 := float64(maxRadius * maxRadius)

	for len(queue) > 0 && len(out) < maxBlocks {
		cur := queue[0]
		queue = queue[1:]
		st := w.BlockState(cur)
		if !match(cur, st) {
			continue
		}
		out = append(out, cur)
		for _, d := range dirs {
			nxt := plugin.BlockPos{X: cur.X + d.X, Y: cur.Y + d.Y, Z: cur.Z + d.Z}
			if _, seen := visited[nxt]; seen {
				continue
			}
			if maxRadius > 0 {
				dx, dy, dz := float64(nxt.X-origin.X), float64(nxt.Y-origin.Y), float64(nxt.Z-origin.Z)
				if dx*dx+dy*dy+dz*dz > r2 {
					continue
				}
			}
			if nxt.Y < w.MinY() || nxt.Y >= w.MinY()+w.Height() {
				continue
			}
			visited[nxt] = struct{}{}
			queue = append(queue, nxt)
		}
	}
	return out
}

// DirectionFromString resolves a WorldEdit direction argument ("up", "down", "north",
// "south", "east", "west", "me"/"forward", "back", "left", "right") relative to a player's facing.
func DirectionFromString(dir string, yaw, pitch float32) (plugin.BlockPos, error) {
	switch dir {
	case "", "me", "forward", "f":
		if pitch < -45 {
			return plugin.BlockPos{Y: 1}, nil
		}
		if pitch > 45 {
			return plugin.BlockPos{Y: -1}, nil
		}
		return horizontalFacing(yaw), nil
	case "back", "b":
		if pitch < -45 {
			return plugin.BlockPos{Y: -1}, nil
		}
		if pitch > 45 {
			return plugin.BlockPos{Y: 1}, nil
		}
		d := horizontalFacing(yaw)
		return plugin.BlockPos{X: -d.X, Y: -d.Y, Z: -d.Z}, nil
	case "left", "l":
		return horizontalFacing(yaw - 90), nil
	case "right", "r":
		return horizontalFacing(yaw + 90), nil
	case "up", "u":
		return plugin.BlockPos{Y: 1}, nil
	case "down", "d":
		return plugin.BlockPos{Y: -1}, nil
	case "north", "n":
		return plugin.BlockPos{Z: -1}, nil
	case "south", "s":
		return plugin.BlockPos{Z: 1}, nil
	case "west", "w":
		return plugin.BlockPos{X: -1}, nil
	case "east", "e":
		return plugin.BlockPos{X: 1}, nil
	default:
		return plugin.BlockPos{}, fmt.Errorf("unknown direction %q", dir)
	}
}

func horizontalFacing(yaw float32) plugin.BlockPos {
	y := math.Mod(float64(yaw), 360)
	if y < 0 {
		y += 360
	}
	switch {
	case y >= 315 || y < 45:
		return plugin.BlockPos{Z: 1} // south
	case y >= 45 && y < 135:
		return plugin.BlockPos{X: -1} // west
	case y >= 135 && y < 225:
		return plugin.BlockPos{Z: -1} // north
	default:
		return plugin.BlockPos{X: 1} // east
	}
}
