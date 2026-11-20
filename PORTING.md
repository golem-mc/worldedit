# Porting notes

Decisions in this port of FastAsyncWorldEdit. The reference is FAWE's behaviour
on Paper.

## Differences from Java FAWE

- **Biomes and chunk regeneration:** golem's `plugin.World` reads biomes and sets
  them during chunk generation (`ChunkWriter.SetBiome`), but has no live chunk
  biome mutation or chunk regeneration. `//setbiome`, `//generatebiome` and
  `//regen` validate input and tell the sender; restore backups with `/snap`
  and `/restore`.
- **CraftScripts:** `/cs` and `/.s` run line-based command scripts with `$1`, `$2`
  substitutions instead of a Rhino/Nashorn JavaScript engine.
- **No Vault/WorldGuard/PlotSquared bridges:** other golem plugins integrate
  through `plugin.Service[worldedit.API](s)` and `RegisterRegionManager`.

## Implementation

- Selection modes: `cuboid`, `extend`, `poly`, `ellipsoid`, `sphere`, `cyl`,
  `convex`, `fuzzy`. Legacy numeric IDs (`35:14`) use the bundled `legacy.json`.
- Expressions: a recursive-descent compiler and evaluator with timeout and step
  guards, used by `//calc`, `//generate`, `//deform`, `=expr` masks,
  `#expression` patterns and `/br deform`.
- Schematic formats: Sponge v3 (`fast`, `fawe`, `schem`, `sponge.3`) read and
  write; Sponge v2 (`fast.2`, `sponge.2`) read and write; Sponge v1 (`sponge.1`)
  and MCEdit (`mcedit`, `mce`, `schematic`) read only; Structure `.nbt`
  (`structure`) read and write; isometric `.png` (`png`) export.
- History: LZ4-compressed disk history at `history/<world>/<uuid>/<id>.bd`, and a
  pure-Go SQLite (`modernc.org/sqlite`, no CGO) rollback database `summary.db`
  with FAWE's `_edits` schema.
- Anvil: a pure-Go `.mca` reader backs `/snap` and `/anvil`.
- Files in `plugins/worldedit/`: `config.yml` (FAWE), `worldedit-config.yml`,
  `schematics/`, `history/`, `clipboard/`, `brushes/`, `heightmap/`,
  `craftscripts/`. Optional LZ4 disk clipboards.
- Needs golem at commit `146fccc` or newer. `mise run dev` starts a development
  server in `dev/` built through `golem-launcher`.

## Plugin API

The plugin calls `plugin.SoftDepends("luckperms")` and
`plugin.LoadBefore("worldguard", "plotsquared")`, and provides `worldedit.API`
via `plugin.Provide`:

```go
if we, ok := plugin.Service[worldedit.API](server); ok {
    sel, err := we.Selection(player)
    es := we.NewEditSession(world, player, "myplugin")
    es.SetRegion(sel, pat)
    _ = es.Commit()
}
```

Protection plugins register with `we.RegisterRegionManager(...)`. Any plugin can
listen to `core.EditSessionEvent`, `core.SchematicSaveEvent`,
`core.SchematicLoadEvent`, `core.ClipboardPasteEvent` and
`core.RegionChangeEvent`.
