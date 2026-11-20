// Package commands registers and executes all WorldEdit and FastAsyncWorldEdit
// commands, both single-slash Brigadier commands and double-slash (//) commands.
package commands

import (
	"fmt"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/golem-mc/golem/plugin"
	"github.com/golem-mc/golem/registry"
	"github.com/golem-mc/worldedit/block"
	"github.com/golem-mc/worldedit/core"
	"github.com/golem-mc/worldedit/tools"
)

// CmdContext is passed to every WorldEdit command handler.
type CmdContext struct {
	WE      *core.WorldEdit
	Sender  plugin.CommandSender
	Player  plugin.Player // nil for console
	Session *core.Session
	World   plugin.World
	Label   string
	RawArgs []string
	Args    []string        // positional args (flags stripped)
	Flags   map[rune]bool   // boolean flags (-a, -e, -h, etc.)
	FlagVal map[rune]string // value flags (e.g. -m <mask>, -t <time>)
	RawLine string
}

// RequirePlayer sends an error and returns false if Sender is not an online player.
func (c *CmdContext) RequirePlayer() bool {
	if c.Player == nil {
		c.WE.Msgs.Error(c.Sender, "This command can only be run by a player.")
		return false
	}
	return true
}

// ParseContext builds a block.ParseContext for this command invocation.
func (c *CmdContext) ParseContext() block.ParseContext {
	return c.WE.ParseContextFor(c.Sender, c.World, c.Session)
}

// CmdSpec defines a WorldEdit/FAWE command.
type CmdSpec struct {
	Name        string   // canonical name, e.g. "/set" (invoked as //set) or "undo" (invoked as /undo)
	Aliases     []string // additional names, with or without leading "/"
	Permission  string
	Description string
	Usage       string
	ValueFlags  string // flag runes that consume the next argument (e.g. "mt")
	Run         func(ctx *CmdContext) error
	Suggest     func(ctx *plugin.CommandContext, partial string) []string
}

// Router holds all registered WorldEdit/FAWE commands and handles both Brigadier and double-slash dispatch.
type Router struct {
	WE         *core.WorldEdit
	byName     map[string]*CmdSpec // key: exact command token as typed after first "/" (e.g. "/set", "undo")
	allSpecs   []*CmdSpec
	registered map[string]bool
}

// Register installs tool handlers and registers all WorldEdit and FAWE commands on we.
func Register(we *core.WorldEdit) *Router {
	tools.InstallHandlers(we)
	r := &Router{
		WE:         we,
		byName:     make(map[string]*CmdSpec),
		registered: make(map[string]bool),
	}

	if !we.Config().FAWE.EnabledComponents.Commands {
		return r
	}

	r.registerGeneral()
	r.registerSelection()
	r.registerRegion()
	r.registerClipboard()
	r.registerSchematic()
	r.registerHistory()
	r.registerGeneration()
	r.registerNavigation()
	r.registerUtility()
	r.registerBiomeAndChunk()
	r.registerToolsAndBrushes()
	r.registerSnapshots()
	r.registerAnvil()

	r.installCommandInterceptors()
	return r
}

func (r *Router) Add(spec CmdSpec) {
	sp := spec
	r.allSpecs = append(r.allSpecs, &sp)

	names := append([]string{sp.Name}, sp.Aliases...)
	var singleNames []string
	for _, n := range names {
		ln := strings.ToLower(strings.TrimSpace(n))
		if ln == "" {
			continue
		}
		r.byName[ln] = &sp
		// If no-double-slash is configured, also map "/foo" -> "foo"
		if strings.HasPrefix(ln, "/") && len(ln) > 1 && r.WE.Config().WorldEdit.NoDoubleSlash {
			bare := strings.TrimPrefix(ln, "/")
			if _, exists := r.byName[bare]; !exists {
				r.byName[bare] = &sp
			}
		}
		if !strings.ContainsAny(ln, " :/\\\t\n\r") {
			singleNames = append(singleNames, ln)
		}
	}

	// Register single-slash names natively with golem's Brigadier tree.
	if len(singleNames) == 0 {
		return
	}
	primary := ""
	var aliases []string
	for _, n := range singleNames {
		if r.registered[n] {
			continue
		}
		if primary == "" {
			primary = n
		} else {
			aliases = append(aliases, n)
		}
	}
	if primary == "" {
		return
	}

	run := func(pctx *plugin.CommandContext) (int, error) {
		rest := plugin.Arg[string](pctx, "args")
		rawLine := primary
		if rest != "" {
			rawLine += " " + rest
		}
		if err := r.Execute(pctx.Sender, primary, tokenizeArgs(rest), rawLine, &sp); err != nil {
			return 0, plugin.ErrHandled
		}
		return 1, nil
	}

	cmd := plugin.Command{
		Name:        primary,
		Aliases:     aliases,
		Description: sp.Description,
		Usage:       sp.Usage,
		Permission:  sp.Permission,
		Node: plugin.Node{
			Run: run,
			Children: []plugin.Node{
				{
					Arg:     plugin.GreedyArg("args"),
					Run:     run,
					Suggest: sp.Suggest,
				},
			},
		},
	}
	if err := r.WE.Server.Commands().Register(cmd); err != nil {
		// Retry without colliding aliases so core command still works alongside other plugins.
		cmd.Aliases = nil
		if err2 := r.WE.Server.Commands().Register(cmd); err2 != nil {
			r.WE.Log.Debug("skipping colliding single-slash command", "cmd", primary, "err", err2)
			return
		}
		r.registered[primary] = true
		return
	}
	r.registered[primary] = true
	for _, a := range aliases {
		r.registered[a] = true
	}
}

func (r *Router) installCommandInterceptors() {
	// Intercept player double-slash commands (e.g. "//set stone" arrives as "/set stone")
	plugin.Listen(r.WE.Server, func(e *plugin.PlayerCommandEvent) {
		if r.tryIntercept(e.Player, e.Command) {
			e.SetCancelled(true)
		}
	}, plugin.ListenOptions{Priority: plugin.Low})

	// Intercept console commands starting with "/" (e.g. "/set", "//set", "/we reload")
	plugin.Listen(r.WE.Server, func(e *plugin.ServerCommandEvent) {
		cmd := strings.TrimSpace(e.Command)
		if strings.HasPrefix(cmd, "//") {
			cmd = cmd[1:] // normalize "//set" -> "/set"
		}
		if r.tryIntercept(r.WE.Server.Console(), cmd) {
			e.SetCancelled(true)
		}
	}, plugin.ListenOptions{Priority: plugin.Low})
}

func (r *Router) tryIntercept(sender plugin.CommandSender, line string) bool {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		return false
	}
	// Strip namespace prefix "worldedit:" or "fastasyncworldedit:" if present
	lower := strings.ToLower(trimmed)
	for _, ns := range []string{"worldedit:", "fastasyncworldedit:", "fawe:"} {
		if strings.HasPrefix(lower, ns) {
			trimmed = trimmed[len(ns):]
			break
		}
	}

	fields := tokenizeArgs(trimmed)
	if len(fields) == 0 {
		return false
	}
	cmdName := strings.ToLower(fields[0])
	// Only intercept commands starting with "/" (double-slash commands) or bare no-double-slash commands not in Brigadier
	if !strings.HasPrefix(cmdName, "/") && r.registered[cmdName] {
		return false
	}
	sp, ok := r.byName[cmdName]
	if !ok {
		return false
	}
	_ = r.Execute(sender, cmdName, fields[1:], trimmed, sp)
	return true
}

// Execute checks permissions, parses flags, logs the command, and invokes sp.Run.
func (r *Router) Execute(sender plugin.CommandSender, label string, rawArgs []string, rawLine string, sp *CmdSpec) error {
	if sp.Permission != "" && !sender.HasPermission(sp.Permission) {
		r.WE.Msgs.ErrorKey(sender, "fawe.error.no-perm", sp.Permission)
		return fmt.Errorf("missing permission %s", sp.Permission)
	}
	r.WE.LogCommand(sender, rawLine)

	var p plugin.Player
	var sess *core.Session
	if pl, ok := sender.(plugin.Player); ok {
		p = pl
		sess = r.WE.Session(pl)
	} else {
		sess = r.WE.ConsoleSession()
	}
	w := r.WE.ResolveWorld(sender, sess)

	posArgs, flags, flagVals := parseFlags(rawArgs, sp.ValueFlags)
	ctx := &CmdContext{
		WE:      r.WE,
		Sender:  sender,
		Player:  p,
		Session: sess,
		World:   w,
		Label:   label,
		RawArgs: rawArgs,
		Args:    posArgs,
		Flags:   flags,
		FlagVal: flagVals,
		RawLine: rawLine,
	}
	if err := sp.Run(ctx); err != nil {
		r.WE.Msgs.Error(sender, err.Error())
		return err
	}
	return nil
}

// tokenizeArgs splits a command line honoring single/double quotes.
func tokenizeArgs(s string) []string {
	var out []string
	var cur strings.Builder
	var quote byte
	inTok := false
	for i := 0; i < len(s); i++ {
		c := s[i]
		if quote != 0 {
			if c == quote {
				quote = 0
				continue
			}
			cur.WriteByte(c)
			continue
		}
		if c == '"' || c == '\'' {
			quote = c
			inTok = true
			continue
		}
		if c == ' ' || c == '\t' {
			if inTok {
				out = append(out, cur.String())
				cur.Reset()
				inTok = false
			}
			continue
		}
		cur.WriteByte(c)
		inTok = true
	}
	if inTok {
		out = append(out, cur.String())
	}
	return out
}

// parseFlags separates `-abc` flags and `-m value` value flags from positional arguments.
// Negative numbers (e.g. `-5`, `-10.5`) are preserved as positional arguments.
func parseFlags(raw []string, valueFlags string) (pos []string, flags map[rune]bool, vals map[rune]string) {
	flags = make(map[rune]bool)
	vals = make(map[rune]string)
	for i := 0; i < len(raw); i++ {
		tok := raw[i]
		if tok == "--" {
			pos = append(pos, raw[i+1:]...)
			break
		}
		if len(tok) >= 2 && tok[0] == '-' && isFlagStart(tok[1]) {
			runes := []rune(tok[1:])
			for j, ch := range runes {
				flags[ch] = true
				if strings.ContainsRune(valueFlags, ch) {
					if j+1 < len(runes) {
						vals[ch] = string(runes[j+1:])
					} else if i+1 < len(raw) {
						i++
						vals[ch] = raw[i]
					}
					break
				}
			}
			continue
		}
		pos = append(pos, tok)
	}
	return pos, flags, vals
}

func isFlagStart(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z')
}

// --- General & /worldedit (/we /fawe) commands ---

func (r *Router) registerGeneral() {
	r.Add(CmdSpec{
		Name:        "worldedit",
		Aliases:     []string{"we", "fawe", "fastasyncworldedit", "/worldedit", "/we", "/fawe"},
		Description: "WorldEdit / FAWE administration commands",
		Usage:       "/we <version|reload|threads|trace|cui|tz|help|debugpaste>",
		Run: func(ctx *CmdContext) error {
			if len(ctx.Args) == 0 {
				ctx.WE.Msgs.Info(ctx.Sender, fmt.Sprintf("FastAsyncWorldEdit v%s for golem. Use /we help for commands.", core.Version))
				return nil
			}
			switch strings.ToLower(ctx.Args[0]) {
			case "version", "ver":
				mcVer, proto := ctx.WE.Server.Version()
				ctx.WE.Msgs.Info(ctx.Sender, fmt.Sprintf("FastAsyncWorldEdit v%s (golem Minecraft %s, protocol %d)", core.Version, mcVer, proto))
			case "reload":
				if !ctx.Sender.HasPermission("worldedit.reload") && !ctx.Sender.HasPermission("fawe.reload") {
					ctx.WE.Msgs.ErrorKey(ctx.Sender, "fawe.error.no-perm", "worldedit.reload")
					return nil
				}
				if err := ctx.WE.Reload(); err != nil {
					return err
				}
				ctx.WE.Msgs.Info(ctx.Sender, "Configuration and translations reloaded!")
			case "threads":
				if !ctx.Sender.HasPermission("worldedit.threads") {
					ctx.WE.Msgs.ErrorKey(ctx.Sender, "fawe.error.no-perm", "worldedit.threads")
					return nil
				}
				ctx.WE.Msgs.Info(ctx.Sender, fmt.Sprintf("Active goroutines: %d, CPUs: %d", runtime.NumGoroutine(), runtime.NumCPU()))
			case "trace":
				if !ctx.Sender.HasPermission("worldedit.trace") {
					ctx.WE.Msgs.ErrorKey(ctx.Sender, "fawe.error.no-perm", "worldedit.trace")
					return nil
				}
				next := !ctx.WE.TraceEnabled.Load()
				ctx.WE.TraceEnabled.Store(next)
				ctx.WE.Msgs.Info(ctx.Sender, fmt.Sprintf("Trace hook set to %v.", next))
			case "cui":
				if !ctx.RequirePlayer() {
					return nil
				}
				ctx.Session.SetCUI(4)
				ctx.WE.DispatchCUI(ctx.Player, ctx.Session)
				ctx.WE.Msgs.Info(ctx.Sender, "WorldEdit CUI handshake completed.")
			case "tz":
				if len(ctx.Args) < 2 {
					ctx.WE.Msgs.Info(ctx.Sender, "Current timezone: "+ctx.Session.Timezone().String())
					return nil
				}
				loc, err := time.LoadLocation(ctx.Args[1])
				if err != nil {
					return fmt.Errorf("unknown timezone %q", ctx.Args[1])
				}
				ctx.Session.SetTimezone(loc)
				ctx.WE.Msgs.Info(ctx.Sender, "Timezone set to "+loc.String())
			case "debugpaste":
				if !ctx.Sender.HasPermission("worldedit.debugpaste") && !ctx.Sender.HasPermission("worldedit.report") {
					ctx.WE.Msgs.ErrorKey(ctx.Sender, "fawe.error.no-perm", "worldedit.debugpaste")
					return nil
				}
				mcVer, _ := ctx.WE.Server.Version()
				ctx.WE.Msgs.Info(ctx.Sender, fmt.Sprintf("FAWE v%s | MC %s | Go %s | DataDir: %s",
					core.Version, mcVer, runtime.Version(), ctx.WE.DataDir()))
			case "help":
				r.printHelp(ctx, ctx.Args[1:])
			default:
				ctx.WE.Msgs.Error(ctx.Sender, "Unknown subcommand. Usage: /we <version|reload|threads|trace|cui|tz|help|debugpaste>")
			}
			return nil
		},
	})

	r.Add(CmdSpec{
		Name:        "/limit",
		Permission:  "worldedit.limit",
		Description: "Modify block change limit",
		Run: func(ctx *CmdContext) error {
			if len(ctx.Args) == 0 {
				ctx.WE.Msgs.Info(ctx.Sender, fmt.Sprintf("Current block change limit: %d (-1 = default).", ctx.Session.BlockLimit()))
				return nil
			}
			v, err := strconv.ParseInt(ctx.Args[0], 10, 64)
			if err != nil {
				return fmt.Errorf("invalid limit %q", ctx.Args[0])
			}
			ctx.Session.SetBlockLimit(v)
			ctx.WE.Msgs.Info(ctx.Sender, fmt.Sprintf("Block change limit set to %d.", v))
			return nil
		},
	})

	r.Add(CmdSpec{
		Name:        "/timeout",
		Permission:  "worldedit.timeout",
		Description: "Modify expression evaluation timeout in milliseconds",
		Run: func(ctx *CmdContext) error {
			if len(ctx.Args) == 0 {
				ctx.WE.Msgs.Info(ctx.Sender, fmt.Sprintf("Current expression timeout: %dms.", ctx.Session.TimeoutMS()))
				return nil
			}
			v, err := strconv.Atoi(ctx.Args[0])
			if err != nil {
				return fmt.Errorf("invalid timeout %q", ctx.Args[0])
			}
			ctx.Session.SetTimeoutMS(v)
			ctx.WE.Msgs.Info(ctx.Sender, fmt.Sprintf("Expression timeout set to %dms.", v))
			return nil
		},
	})

	r.Add(CmdSpec{
		Name:        "/fast",
		Permission:  "worldedit.fast",
		Description: "Toggle fast mode (disables undo and neighbor updates)",
		Run: func(ctx *CmdContext) error {
			next := !ctx.Session.FastMode()
			if len(ctx.Args) > 0 {
				next = strings.EqualFold(ctx.Args[0], "on") || strings.EqualFold(ctx.Args[0], "true")
			}
			ctx.Session.SetFastMode(next)
			if next {
				ctx.WE.Msgs.Info(ctx.Sender, "Fast mode enabled. History and block updates will be skipped.")
			} else {
				ctx.WE.Msgs.Info(ctx.Sender, "Fast mode disabled.")
			}
			return nil
		},
	})

	r.Add(CmdSpec{
		Name:        "/perf",
		Permission:  "worldedit.perf",
		Description: "Toggle side effects for performance",
		Run: func(ctx *CmdContext) error {
			next := !ctx.Session.PerfSideEffects()
			if len(ctx.Args) > 0 {
				next = strings.EqualFold(ctx.Args[0], "on") || strings.EqualFold(ctx.Args[0], "true")
			}
			ctx.Session.SetPerfSideEffects(next)
			ctx.WE.Msgs.Info(ctx.Sender, fmt.Sprintf("Side effects set to %v.", next))
			return nil
		},
	})

	r.Add(CmdSpec{
		Name:        "/reorder",
		Permission:  "worldedit.reorder",
		Description: "Sets the reorder mode of WorldEdit",
		Run: func(ctx *CmdContext) error {
			if len(ctx.Args) == 0 {
				ctx.WE.Msgs.Info(ctx.Sender, "Current reorder mode: "+ctx.Session.ReorderMode())
				return nil
			}
			ctx.Session.SetReorderMode(strings.ToLower(ctx.Args[0]))
			ctx.WE.Msgs.Info(ctx.Sender, "Reorder mode set to "+ctx.Session.ReorderMode())
			return nil
		},
	})

	r.Add(CmdSpec{
		Name:        "/drawsel",
		Permission:  "worldedit.drawsel",
		Description: "Toggle drawing the current selection (CUI)",
		Run: func(ctx *CmdContext) error {
			next := !ctx.Session.DrawSel()
			ctx.Session.SetDrawSel(next)
			ctx.WE.Msgs.Info(ctx.Sender, fmt.Sprintf("Selection drawing set to %v.", next))
			return nil
		},
	})

	r.Add(CmdSpec{
		Name:        "/world",
		Permission:  "worldedit.world",
		Description: "Sets the world override",
		Run: func(ctx *CmdContext) error {
			if len(ctx.Args) == 0 {
				ctx.Session.SetWorldOverride(nil)
				ctx.WE.Msgs.Info(ctx.Sender, "World override cleared.")
				return nil
			}
			k, err := plugin.ParseKey(ctx.Args[0])
			if err != nil {
				return err
			}
			w, ok := ctx.WE.Server.World(k)
			if !ok {
				return fmt.Errorf("world %q not found", ctx.Args[0])
			}
			ctx.Session.SetWorldOverride(w)
			ctx.WE.Msgs.Info(ctx.Sender, "World override set to "+w.Key().String())
			return nil
		},
	})

	r.Add(CmdSpec{
		Name:        "gmask",
		Aliases:     []string{"/gmask"},
		Permission:  "worldedit.global-mask",
		Description: "Set the global destination mask",
		Run: func(ctx *CmdContext) error {
			if len(ctx.Args) == 0 {
				ctx.Session.SetGlobalMask(nil)
				ctx.WE.Msgs.Info(ctx.Sender, "Global mask disabled.")
				return nil
			}
			m, err := block.ParseMask(strings.Join(ctx.Args, " "), ctx.ParseContext())
			if err != nil {
				return err
			}
			ctx.Session.SetGlobalMask(m)
			ctx.WE.Msgs.Info(ctx.Sender, "Global mask set to: "+m.String())
			return nil
		},
	})

	r.Add(CmdSpec{
		Name:        "gsmask",
		Aliases:     []string{"/gsmask", "globalsourcemask", "/globalsourcemask"},
		Permission:  "worldedit.global-mask",
		Description: "Set the global source mask",
		Run: func(ctx *CmdContext) error {
			if len(ctx.Args) == 0 {
				ctx.Session.SetGlobalSourceMask(nil)
				ctx.WE.Msgs.InfoKey(ctx.Sender, "fawe.worldedit.general.source.mask.disabled")
				return nil
			}
			m, err := block.ParseMask(strings.Join(ctx.Args, " "), ctx.ParseContext())
			if err != nil {
				return err
			}
			ctx.Session.SetGlobalSourceMask(m)
			ctx.WE.Msgs.InfoKey(ctx.Sender, "fawe.worldedit.general.source.mask")
			return nil
		},
	})

	r.Add(CmdSpec{
		Name:        "gtransform",
		Aliases:     []string{"/gtransform"},
		Permission:  "worldedit.global-transform",
		Description: "Set the global transform",
		Run: func(ctx *CmdContext) error {
			if len(ctx.Args) == 0 {
				ctx.Session.SetGlobalTransform(nil)
				ctx.WE.Msgs.InfoKey(ctx.Sender, "fawe.worldedit.general.transform.disabled")
				return nil
			}
			tr, err := block.ParseTransform(strings.Join(ctx.Args, " "), ctx.ParseContext())
			if err != nil {
				return err
			}
			ctx.Session.SetGlobalTransform(tr)
			ctx.WE.Msgs.InfoKey(ctx.Sender, "fawe.worldedit.general.transform")
			return nil
		},
	})

	r.Add(CmdSpec{
		Name:        "gtexture",
		Aliases:     []string{"/gtexture"},
		Permission:  "worldedit.global-texture",
		Description: "Set the global texture",
		Run: func(ctx *CmdContext) error {
			if len(ctx.Args) == 0 {
				ctx.Session.SetGlobalTexture("")
				ctx.WE.Msgs.InfoKey(ctx.Sender, "fawe.worldedit.general.texture.disabled")
				return nil
			}
			ctx.Session.SetGlobalTexture(ctx.Args[0])
			ctx.WE.Msgs.InfoKey(ctx.Sender, "fawe.worldedit.general.texture.set", "", ctx.Args[0])
			return nil
		},
	})

	r.Add(CmdSpec{
		Name:        "toggleplace",
		Aliases:     []string{"/toggleplace"},
		Permission:  "worldedit.toggleplace",
		Description: "Switch between your position and pos1 for placement",
		Run: func(ctx *CmdContext) error {
			next := !ctx.Session.PlaceAtPos1()
			ctx.Session.SetPlaceAtPos1(next)
			if next {
				ctx.WE.Msgs.Info(ctx.Sender, "Now placing at pos #1.")
			} else {
				ctx.WE.Msgs.Info(ctx.Sender, "Now placing at the block you stand in.")
			}
			return nil
		},
	})

	r.Add(CmdSpec{
		Name:        "tips",
		Aliases:     []string{"/tips"},
		Permission:  "fawe.tips",
		Description: "Toggle FAWE tips",
		Run: func(ctx *CmdContext) error {
			next := !ctx.Session.TipsDisabled()
			ctx.Session.SetTipsDisabled(next)
			if next {
				ctx.WE.Msgs.InfoKey(ctx.Sender, "fawe.info.worldedit.toggle.tips.on")
			} else {
				ctx.WE.Msgs.InfoKey(ctx.Sender, "fawe.info.worldedit.toggle.tips.off")
			}
			return nil
		},
	})

	r.Add(CmdSpec{
		Name:        "searchitem",
		Aliases:     []string{"/searchitem", "/l", "/search"},
		Permission:  "worldedit.searchitem",
		Description: "Search for a block or item by name",
		Run: func(ctx *CmdContext) error {
			if len(ctx.Args) == 0 {
				return fmt.Errorf("usage: /searchitem <query>")
			}
			q := strings.ToLower(ctx.Args[0])
			var matches []string
			for _, name := range registry.BlockNames {
				if strings.Contains(name, q) {
					matches = append(matches, name)
					if len(matches) >= 20 {
						break
					}
				}
			}
			if len(matches) == 0 {
				ctx.WE.Msgs.Info(ctx.Sender, "No blocks matched "+q)
				return nil
			}
			ctx.WE.Msgs.Info(ctx.Sender, fmt.Sprintf("Matches (%d): %s", len(matches), strings.Join(matches, ", ")))
			return nil
		},
	})

	r.Add(CmdSpec{
		Name:        "/registry",
		Permission:  "worldedit.registry",
		Description: "Search through Minecraft registries (block, biome, tag)",
		Run: func(ctx *CmdContext) error {
			kind := "block"
			q := ""
			if len(ctx.Args) >= 1 {
				kind = strings.ToLower(ctx.Args[0])
			}
			if len(ctx.Args) >= 2 {
				q = strings.ToLower(ctx.Args[1])
			}
			var items []string
			switch kind {
			case "block", "blocks":
				items = registry.BlockNames
			case "tag", "tags", "blocktag":
				items = registry.BlockTagNames()
			case "biome", "biomes":
				for i := registry.Biome(0); i < registry.BiomeCount; i++ {
					if n := i.Name(); n != "" {
						items = append(items, n)
					}
				}
			default:
				return fmt.Errorf("unknown registry %q (use block, biome, tag)", kind)
			}
			var matches []string
			for _, it := range items {
				if q == "" || strings.Contains(it, q) {
					matches = append(matches, it)
					if len(matches) >= 25 {
						break
					}
				}
			}
			ctx.WE.Msgs.Info(ctx.Sender, fmt.Sprintf("Registry %s (%d shown): %s", kind, len(matches), strings.Join(matches, ", ")))
			return nil
		},
	})
}

func (r *Router) printHelp(ctx *CmdContext, args []string) {
	page := 1
	query := ""
	if len(args) > 0 {
		if n, err := strconv.Atoi(args[0]); err == nil && n > 0 {
			page = n
		} else {
			query = strings.ToLower(args[0])
		}
	}
	var list []*CmdSpec
	for _, sp := range r.allSpecs {
		if query != "" && !strings.Contains(strings.ToLower(sp.Name), query) && !strings.Contains(strings.ToLower(sp.Description), query) {
			continue
		}
		list = append(list, sp)
	}
	sort.Slice(list, func(i, j int) bool { return list[i].Name < list[j].Name })
	const perPage = 10
	pages := max(1, (len(list)+perPage-1)/perPage)
	if page > pages {
		page = pages
	}
	start := (page - 1) * perPage
	end := min(len(list), start+perPage)
	ctx.WE.Msgs.Info(ctx.Sender, fmt.Sprintf("=== WorldEdit Help (Page %d/%d, %d commands) ===", page, pages, len(list)))
	for _, sp := range list[start:end] {
		ctx.WE.Msgs.Info(ctx.Sender, fmt.Sprintf("/%s - %s", sp.Name, sp.Description))
	}
}
