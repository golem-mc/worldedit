# FastAsyncWorldEdit for golem

An unofficial port of [FastAsyncWorldEdit](https://github.com/IntellectualSites/FastAsyncWorldEdit)
(FAWE) to [golem](https://github.com/golem-mc/golem), the Minecraft: Java Edition
server written in Go. It keeps FAWE and WorldEdit's commands, permission nodes,
configuration files, messages, schematic formats, disk history, and SQLite
rollback database schema, so builders and server administrators coming from
Bukkit or Paper get the same workflow here.

## What's included

- **Selections & WECUI** — All 8 selection modes (`cuboid`, `extend`, `poly`,
  `ellipsoid`, `sphere`, `cyl`, `convex`, `fuzzy`), `//pos1`/`//pos2`,
  `//hpos1`/`//hpos2`, `//chunk`, `//wand`, `//expand`, `//contract`, `//shift`,
  `//outset`, `//inset`, `//trim`, `//size`, `//count`, `//distr`, `//sel`, and
  WorldEdit CUI protocol support over the `worldedit:cui` plugin channel.
- **Region operations** — `//set`, `//air`, `//replace`, `//walls`, `//faces`,
  `//overlay`, `//lay`, `//center`, `//naturalize`, `//smooth`, `//snowsmooth`,
  `//move`, `//stack`, `//fall`, `//hollow`, `//line`, `//curve`, `//deform`,
  `//forest`, `//flora`, `//fixblocks`, `//wea`, `//wer`, `//nbtinfo`.
- **Rich patterns, masks & transforms** — Weighted random patterns, block tags
  (`##planks`), random states (`*oak_stairs`), state-preserving (`^stone`),
  legacy numeric IDs (`35:14` via bundled `legacy.json`), `#simplex`, `#perlin`,
  `#voronoi`, `#ridgedmulti`, `#linear`/`#linear2d`/`#linear3d`, `#clipboard`,
  `#mask`, `#offset`, `#randomoffset`, `#relative`, `#nox`/`#noy`/`#noz`,
  `#buffered`, `#expression`, `#color`/`#darken`/`#lighten`/`#saturate`/`#desaturate`,
  `#typeswap`; masks (`!`, `>`, `<`, `~`, `%`, `=expr`, `$biome`, `/[min][max]`,
  `#existing`, `#air`, `#solid`, `#liquid`, `#surface`, `#wall`, `#region`,
  `#hand`, `#hotbar`, `#adjacent`, `#simplex`, `#radius`, `#offset`); and global
  & brush transforms (`#offset`, `#rotate`, `#scale`, `#spread`, `#pattern`,
  `#random`, `#linear`, `#linear3d`).
- **Mathematical expression engine** — Full recursive-descent compiler &
  evaluator with timeout/step guards powering `//calc`, `//generate`,
  `//deform`, `=expr` masks, `#expression` patterns, and `/br deform`.
- **Clipboards & Schematics** — `//copy`, `//lazycopy`, `//cut`, `//lazycut`,
  `//paste`, `//place`, `//rotate`, `//flip`, `/clearclipboard`, `/download`,
  multi-clipboard (`loadall`/`unload`), optional LZ4 disk clipboards, and
  native pure-Go schematic I/O for:
  - Sponge Schematic v3 (`.schem` — `fast` / `fawe` / `schem` / `sponge.3`)
  - Sponge Schematic v2 (`.schem` — `fast.2` / `sponge.2`)
  - Sponge Schematic v1 reader (`sponge.1`)
  - Legacy MCEdit `.schematic` reader (`mcedit` / `mce` / `schematic`)
  - Vanilla Minecraft Structure `.nbt` reader & writer (`structure`)
  - Isometric `.png` export (`png`)
- **History & SQLite Rollback** — `/undo`, `/redo`, `/clearhistory`,
  LZ4-compressed disk history (`history/<world>/<uuid>/<id>.bd`), pure-Go
  SQLite (`modernc.org/sqlite`, zero CGO) rollback summary database using
  FAWE's upstream `_edits` schema (`summary.db`), and `//history` (`//frb`)
  `list`, `find`, `info`, `distr`, `rollback`, `restore`, `clear`, `import`.
- **Generation, Navigation & Utility** — `//sphere`, `//hsphere`, `//cyl`,
  `//hcyl`, `//pyramid`, `//hpyramid`, `//cone`, `//generate`, `//blob`,
  `//caves`, `//ore`, `//ores`, `//img`, `/forestgen`, `/pumpkins`, `/unstuck`,
  `/ascend`, `/descend`, `/ceil`, `/thru`, `/jumpto`, `/up`, `//fill`,
  `//fillr`, `//drain`, `/fixwater`, `/fixlava`, `/removeabove`, `/removebelow`,
  `/removenear`, `/replacenear`, `/snow`, `/thaw`, `/green`, `/extinguish`,
  `/butcher`, `/remove`, `//macro`, `/cs`, `/.s`, `//confirm`, `//cancel`.
- **Tools, Brushes & Super-Pickaxe** — Selection/navigation wands, `/info`,
  `/inspect`, `/tree`, `/repl`, `/cycler`, `/floodfill`, `/deltree`, `/farwand`,
  `/lrbuild`, `/stacker`, `/sp` (`single`/`area`/`recursive`), and full `/br`
  brush suite (`sphere`, `cyl`, `circle`, `clipboard`, `smooth`, `snowsmooth`,
  `blendball`, `erode`, `pull`, `recursive`, `line`, `spline`, `catenary`,
  `sweep`, `rock`, `shatter`, `layer`, `scatter`, `splatter`, `height`, `cliff`,
  `flatten`, `copypaste`, `command`, `deform`, `forest`, `raise`, `lower`,
  `snow`, `ex`, `gravity`, `butcher`, plus `/mask`, `/smask`, `/material`,
  `/size`, `/range`, `/transform`, `/target`, `/targetoffset`, `/scroll`,
  `/br save`, `/br load`, `/br list`).
- **Snapshots & Anvil tools** — Pure-Go `.mca` Anvil region file reader for
  `/snap` (`list`, `use`, `sel`, `before`, `after`), `/restore`, and `/anvil`
  (`count`, `countall`, `distr`, `set`, `replace`, `clear`, `deletealloldregions`).

## Installing

### With the golem launcher

Add the plugin with
[`golem-launcher`](https://github.com/golem-mc/golem#installing-plugins):

```bash
./golem-launcher plugin add github.com/golem-mc/worldedit
./golem-launcher
```

The launcher writes the plugin to `launcher.json`:

```json
{
  "server": "<golem version>",
  "plugins": [
    "github.com/golem-mc/worldedit"
  ]
}
```

The plugin needs golem at commit `146fccc` or newer.

### Development server

`mise run dev` starts a server in `dev/` with this checkout built in through
`golem-launcher` (the golem version comes from `go.mod`). Its runtime files
are git-ignored.

## Configuration

On first start, FastAsyncWorldEdit writes its files to `plugins/worldedit/`:

- `config.yml` — FAWE configuration (`limits`, `history`, `clipboard`, `paths`,
  `region-restrictions`, `queue`, `web`, `tick-limiter`, `lighting`, `general`).
- `worldedit-config.yml` — WorldEdit configuration (`wand-item`,
  `navigation-wand`, `saving.dir`, `snapshots.directory`, `super-pickaxe`,
  `logging`, `no-double-slash`, `no-op-permissions`, etc.).
- `schematics/` — Saved and loadable `.schem`, `.schematic`, `.nbt`, and `.png`
  files.
- `history/<world>/` — LZ4-compressed `.bd` change logs and SQLite `summary.db`.
- `clipboard/`, `brushes/`, `heightmap/`, `craftscripts/` — Disk clipboards,
  saved brush presets, heightmap/pixel-art images, and command scripts.

Run `/we reload` after editing `config.yml`, `worldedit-config.yml`, or custom
`lang/strings.json` overrides.

## Plugin API

FastAsyncWorldEdit registers itself with `plugin.SoftDepends("luckperms")` and
`plugin.LoadBefore("worldguard", "plotsquared")`, providing `worldedit.API` via
`plugin.Provide`:

```go
if we, ok := plugin.Service[worldedit.API](server); ok {
    sel, err := we.Selection(player)
    es := we.NewEditSession(world, player, "myplugin")
    es.SetRegion(sel, pat)
    _ = es.Commit()
}
```

Protection plugins can hook into region restrictions with
`we.RegisterRegionManager(...)`, and any plugin can listen to
`core.EditSessionEvent`, `core.SchematicSaveEvent`, `core.SchematicLoadEvent`,
`core.ClipboardPasteEvent`, and `core.RegionChangeEvent`.

## Differences from Java FAWE

- **Live biome & chunk regeneration:** golem's `plugin.World` API reads biomes
  and sets biomes during chunk generation (`ChunkWriter.SetBiome`), but does
  not yet expose live chunk biome mutation or live chunk regeneration.
  `//setbiome`, `//generatebiome`, and `//regen` validate inputs and inform the
  sender; world backups can be restored with `/snap` and `/restore`.
- **CraftScripts:** `/cs` and `/.s` execute line-based WorldEdit/server command
  scripts with `$1`, `$2`, ... positional substitutions rather than embedding a
  Rhino/Nashorn JVM JavaScript engine.
- **No Vault/WorldGuard/PlotSquared Java bridges:** Other golem plugins integrate
  directly via `plugin.Service[worldedit.API](s)` and `RegisterRegionManager`.

## Development

```bash
mise exec -- gofmt -l .
mise exec -- go vet ./...
mise exec -- go mod tidy -diff
mise exec -- go test -race ./... -count=1
mise run build
```

See [CONTRIBUTING.md](CONTRIBUTING.md) for commit conventions and the release
process.

## License

GPL-3.0 — see [LICENSE](LICENSE). Derived from
[FastAsyncWorldEdit](https://github.com/IntellectualSites/FastAsyncWorldEdit)
and [WorldEdit](https://github.com/EngineHub/WorldEdit), including the bundled
`worldedit-config.yml`, `strings.json`, and `legacy.json`. This project is not
affiliated with or endorsed by IntellectualSites or EngineHub.
