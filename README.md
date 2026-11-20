# FastAsyncWorldEdit for golem

An unofficial port of [FastAsyncWorldEdit](https://github.com/IntellectualSites/FastAsyncWorldEdit)
(FAWE) to [golem](https://github.com/golem-mc/golem), the Minecraft: Java Edition
server written in Go. Commands, permission nodes, config files, messages and
schematic formats follow FAWE, so FAWE's guides apply.

## Install

```bash
golem-launcher plugin add github.com/golem-mc/worldedit
```

This is the only module. [LuckPerms](https://github.com/golem-mc/perms) is used
for permissions when it is installed (optional).

## Getting started

Operators can use every command at once. Hold a wooden axe (the wand): left-click
a block for position 1 and right-click for position 2, or stand somewhere and run
`//pos1` and `//pos2`. Then edit the selection:

```
//set stone
//replace stone dirt,grass_block
//copy
//paste
/undo
```

Commands start with `//`; with `no-double-slash: true` in `worldedit-config.yml`
a single `/` works too. `//help` lists commands, `/we version` shows the
version. Anything that would change a very large number of blocks asks you to
confirm with `//confirm`.

## Commands

Arguments in `<>` are required and `[]` optional. Common flags are noted; the
[FAWE documentation](https://intellectualsites.gitbook.io/fastasyncworldedit)
covers the full syntax of patterns (`stone,dirt`, `50%stone,50%dirt`, `#clipboard`,
`#simplex`, `=expression`), masks (`>stone`, `#existing`, `!air`, `=expr`) and
transforms.

### Selection

| Command | What it does | Permission |
| --- | --- | --- |
| `//pos1 [x,y,z]`, `//pos2` (`//1`, `//2`) | Set a selection point | `worldedit.selection.pos` |
| `//hpos1`, `//hpos2` | Set a point at the block you look at | `worldedit.selection.hpos` |
| `//wand`, `/toggleeditwand` | Get the wand, or toggle it | `worldedit.wand`, `worldedit.wand.toggle` |
| `//sel [mode]` (`;`) | Clear the selection or pick `cuboid`, `extend`, `poly`, `ellipsoid`, `sphere`, `cyl`, `convex`, `fuzzy` | `worldedit.analysis.sel` |
| `//chunk` | Select your chunk | `worldedit.selection.chunk` |
| `//expand`, `//contract`, `//shift`, `//outset`, `//inset`, `//trim` | Change the selection | `worldedit.selection.*` |
| `//size`, `//count <mask>`, `//distr` | Inspect the selection | `worldedit.selection.size`, `worldedit.analysis.*` |

The WorldEdit CUI client mod shows selections; so does `//drawsel` without it.

### Region

| Command | What it does | Permission |
| --- | --- | --- |
| `//set <pattern>`, `//air` | Fill the selection | `worldedit.region.set` |
| `//replace [from] <to>` (`//re`) | Replace blocks | `worldedit.region.replace` |
| `//walls`, `//faces`, `//hollow`, `//overlay`, `//center`, `//line`, `//curve` | Build from the selection | `worldedit.region.*` |
| `//move`, `//stack`, `//fall` | Move or repeat the selection (`-s` shifts it, `-a` skips air) | `worldedit.region.move`, `.stack`, `.fall` |
| `//naturalize`, `//smooth`, `//snowsmooth`, `//deform`, `//flora`, `//forest`, `//fixblocks` | Terrain and plants | `worldedit.region.*` |
| `//fixlighting`, `//removelighting`, `//setblocklight` | Lighting | `worldedit.light.*` |
| `//nbtinfo` | NBT of the block you look at | `worldedit.nbtinfo` |

### Clipboard and schematics

| Command | What it does | Permission |
| --- | --- | --- |
| `//copy`, `//cut` | Copy or cut the selection (`-e` entities, `-b` biomes, `-m <mask>`) | `worldedit.clipboard.copy`, `.cut` |
| `//paste` (`//p`) | Paste at your position (`-a` skip air, `-o` original position, `-s` select) | `worldedit.clipboard.paste` |
| `//place`, `//rotate`, `//flip`, `/clearclipboard` | Place, transform or clear the clipboard | `worldedit.clipboard.*` |
| `//schem <load\|save\|list\|delete\|move\|formats\|clear\|unload\|share> [name]` (`/schem`) | Schematic files in Sponge v1–v3, MCEdit and Structure `.nbt`; PNG export | `worldedit.schematic.*` |
| `//macro`, `/cs`, `/.s` | Command macros and line-based scripts (`$1`, `$2` arguments) | `worldedit.macro`, `worldedit.scripting.execute` |

Several clipboards can be loaded at once with `//schem loadall` and `#clipboard`.

### Generation

| Command | What it does | Permission |
| --- | --- | --- |
| `//sphere`, `//hsphere`, `//cyl`, `//hcyl`, `//pyramid`, `//hpyramid`, `//cone` | Shapes (`-r` raised) | `worldedit.generation.*` |
| `//generate <pattern> <expression>` | Shape from an expression | `worldedit.generation.shape` |
| `//caves`, `//ore`, `//blob`, `//feature`, `//structure`, `//img` | Caves, ores, blobs, features, structures, heightmap images | `worldedit.generation.*` |
| `/forestgen`, `/pumpkins` | Trees and pumpkin patches | `worldedit.generation.forest`, `.pumpkins` |
| `//calc <expression>` (`//calculate`) | Evaluate an expression | `worldedit.calc` |

### History

| Command | What it does | Permission |
| --- | --- | --- |
| `/undo [times] [player]`, `/redo` | Undo or redo edits | `worldedit.history.undo`, `.redo` |
| `/clearhistory` | Forget your history | `worldedit.history.clear` |
| `//history <list\|find\|info\|distr\|rollback\|restore\|clear>` (`//frb`) | Look through and roll back other players' edits | `worldedit.history.*` |
| `//cancel`, `//confirm` | Cancel a running edit or confirm a large one | `fawe.cancel`, `fawe.confirm` |

### Tools and brushes

| Command | What it does | Permission |
| --- | --- | --- |
| `/br <sphere\|cyl\|smooth\|erode\|height\|clipboard\|line\|spline\|scatter\|splatter\|layer\|gravity\|...> [args]` (`/brush`) | Bind a brush to the held item | `worldedit.brush.*` |
| `/mask`, `/material`, `/size`, `/tree`, `/repl`, `/none` | Brush mask, material and size, and other tools | `worldedit.brush.options.*`, `worldedit.tool.*` |
| `//` or `/sp` (super pickaxe) | Break blocks instantly | `worldedit.superpickaxe` |
| `/gmask`, `/gtransform`, `/gtexture` | Global mask, transform and texture for every edit | `worldedit.global-*` |
| `/info`, `/inspect` | Block information and edit history inspection tools | `worldedit.tool.info`, `.inspect` |

### Utility and navigation

| Command | What it does | Permission |
| --- | --- | --- |
| `/butcher [radius]`, `/remove <type> <radius>` | Remove mobs and entities | `worldedit.butcher`, `worldedit.remove` |
| `/fixwater`, `/fixlava`, `/drain`, `/extinguish` (`/ex`) | Fluids and fire | `worldedit.*` |
| `/snow`, `/thaw`, `/green`, `/removenear`, `/replacenear` | Area clean-up | `worldedit.*` |
| `/unstuck` (`!`), `/ascend`, `/descend`, `/ceil`, `/thru`, `/jumpto` (`/j`), `//up` | Move yourself | `worldedit.navigation.*` |
| `/snap`, `/restore` | World snapshots (Anvil backups) | `worldedit.snapshots.*` |
| `/anvil <count\|distr\|set\|replace\|clear\|deletealloldregions>` | Region file tools | `worldedit.anvil.*` |
| `/biomelist`, `/biomeinfo`, `/chunkinfo`, `/listchunks`, `/delchunks` | Biome and chunk information | `worldedit.biome.*`, `worldedit.*chunks` |
| `//setbiome [-p] <biome>` | Set the biome of the selection (`-p`: the column you stand in) | `worldedit.biome.set` |
| `//generatebiome <biome> <expression>`, `//genbiome`, `//gb` | Set a biome where the formula is positive (`-r`, `-o`, `-c` pick the coordinates) | `worldedit.generation.shape.biome` |
| `//regen` | Regenerate the selection from the world seed | `worldedit.regen` |
| `/we <version\|reload\|threads\|cui\|tz\|help>` (`/fawe`) | Plugin administration | `worldedit.reload`, `fawe.reload` |
| `//limit`, `//timeout`, `//fast`, `//perf`, `//reorder`, `//world`, `//wea` | Per-player settings | `worldedit.*`, `fawe.admin` |

## Permissions

Nodes are FAWE's. Everything defaults to operators (level 2); with
`no-op-permissions: true` nobody has them by default and you grant them with a
permissions plugin.

| Node | Meaning |
| --- | --- |
| `fawe.permpack.basic` | Every ordinary WorldEdit and FAWE command |
| `fawe.bypass` | Bypass region restrictions (`fawe.bypass.regions`) and limits (`fawe.limit.unlimited`) |
| `fawe.admin` | `//wea` and administration commands |
| `fawe.confirm`, `fawe.cancel` | Confirm and cancel edits |
| `fawe.plotsquared`, `fawe.tips` | PlotSquared integration (everyone); edit tips |
| `worldedit.<command group>.<command>` | A single command, for example `worldedit.region.set` or `worldedit.clipboard.paste` |
| `worldedit.history.undo.self` | Undo only your own edits |

## Configuration

Both files are in `plugins/worldedit/` and written on first start. Run
`/we reload` after editing.

**`config.yml`** (FAWE)

| Option | Default | Meaning |
| --- | --- | --- |
| `region-restrictions` | `true` | Honour region plugins (see `region-restrictions-options.mode`, default `MEMBER`) |
| `limits.default.max-changes` | `50000000` | Blocks one edit may change |
| `limits.default.max-brush-radius` | `100` | Largest brush radius |
| `limits.default.max-radius` | `-1` | Largest radius for shapes (`-1` unlimited) |
| `limits.default.confirm-large` | `true` | Ask for `//confirm` on big edits |
| `limits.default.disallowed-blocks` | wheat, fire, redstone wire | Blocks players cannot place |
| `limits.default.max-history-mb` | `-1` | Size cap of a player's history |
| `history.use-disk` | `true` | Keep history on disk |
| `history.delete-after-days` | `7` | Remove old history |
| `history.use-database` | `true` | Keep the rollback database for `//history` |
| `paths.per-player-schematics` | `false` | One schematic folder per player |
| `queue.parallel-threads` | CPU count | Worker threads for edits |

**`worldedit-config.yml`** (WorldEdit)

| Option | Default | Meaning |
| --- | --- | --- |
| `wand-item` | `minecraft:wooden_axe` | Selection wand |
| `navigation-wand.item` / `.max-distance` | `minecraft:compass` / `100` | Navigation wand |
| `history.size` / `.expiration` | `15` / `10` | Undo steps and minutes kept in memory |
| `super-pickaxe.drop-items` | `true` | Super pickaxe drops items |
| `saving.dir` | `schematics` | Schematics folder |
| `scripting.timeout` | `3000` | CraftScript time limit in ms |
| `no-double-slash` | `false` | Allow `/set` instead of `//set` |
| `no-op-permissions` | `false` | Do not give operators every permission |
| `server-side-cui` | `true` | Show selections without a client mod |
| `logging.log-commands` | `false` | Log commands to `worldedit.log` |

## Migrating from Bukkit or Paper

Copy `config.yml`, `worldedit-config.yml`, `schematics/` and `history/` from
FAWE's `plugins/FastAsyncWorldEdit/` into `plugins/worldedit/`. Schematics,
brushes and permission nodes work unchanged. Permission plugins that manage FAWE
nodes work as they do on Paper, including
[LuckPerms](https://github.com/golem-mc/perms) for golem. Region plugins use
`RegisterRegionManager` (below) instead of Bukkit's WorldGuard hooks.

## Differences from FAWE

- `/cs` scripts are plain command lists with `$1`, `$2` arguments, not JavaScript.
- No built-in WorldGuard, PlotSquared or Vault bridges; those plugins connect through the API below.

## For plugin developers

```bash
go get github.com/golem-mc/worldedit
```

Use WorldEdit from another plugin through the `worldedit.API` service:

```go
we, ok := plugin.Service[worldedit.API](server)
if !ok {
    return nil // WorldEdit is not installed
}
sel, err := we.Selection(player) // the player's current selection
if err != nil {
    return err
}
es := we.NewEditSession(world, player, "myplugin")
es.SetRegion(sel, block.SinglePattern(world.BlockState(plugin.BlockPos{Y: 64})))
return es.Commit() // recorded in the player's /undo history
```

Other methods: `Session`, `ConsoleSession`, `SaveSchematic`, `LoadSchematic`,
`AllowedRegions` and `Config`.

A protection plugin limits where players may edit by registering a
`core.RegionManager`, and any plugin can listen to `core.EditSessionEvent`
(cancel it in the `EditStageBefore` stage to refuse an edit),
`core.SchematicSaveEvent`, `core.SchematicLoadEvent`, `core.ClipboardPasteEvent`
and `core.RegionChangeEvent`:

```go
type protection struct{}

func (protection) Name() string { return "myprotection" }

func (protection) AllowedRegions(p plugin.Player, w plugin.World) []region.Region {
    return []region.Region{
        region.NewCuboid(plugin.BlockPos{X: 0, Y: 0, Z: 0}, plugin.BlockPos{X: 99, Y: 255, Z: 99}),
    }
}

func enable(server plugin.Server) {
    if we, ok := plugin.Service[worldedit.API](server); ok {
        we.RegisterRegionManager(protection{})
    }
    plugin.Listen(server, func(e *core.EditSessionEvent) {
        if e.Stage == core.EditStageBefore {
            e.SetCancelled(true) // refuse the edit
        }
    })
}
```

## License

GPL-3.0, see [LICENSE](LICENSE). Derived from
[FastAsyncWorldEdit](https://github.com/IntellectualSites/FastAsyncWorldEdit)
and [WorldEdit](https://github.com/EngineHub/WorldEdit). Not affiliated with or
endorsed by IntellectualSites or EngineHub.
