# FastAsyncWorldEdit for golem

An unofficial port of [FastAsyncWorldEdit](https://github.com/IntellectualSites/FastAsyncWorldEdit)
(FAWE) to [golem](https://github.com/golem-mc/golem), the Minecraft: Java Edition
server written in Go. Commands, permission nodes, configuration files, messages
and schematic formats work as in FAWE; see the
[FAWE documentation](https://intellectualsites.gitbook.io/fastasyncworldedit)
for how to use them.

```bash
golem-launcher plugin add github.com/golem-mc/worldedit
```

## What's included

- **Selections** — `//pos1`, `//pos2`, `//wand`, `//sel` with all selection modes, `//expand`, `//contract`, `//size`, and WorldEdit CUI.
- **Region operations** — `//set`, `//replace`, `//walls`, `//overlay`, `//move`, `//stack`, `//hollow`, `//naturalize`, `//smooth`.
- **Patterns, masks and transforms** — the full FAWE syntax, including noise patterns, block tags and `=expr` masks.
- **Expressions** — `//calc`, `//generate`, `//deform`.
- **Clipboard** — `//copy`, `//cut`, `//paste`, `//rotate`, `//flip`, and several clipboards at once.
- **Schematics** — `//schem` load and save in Sponge v1–v3, MCEdit, Structure `.nbt` and PNG.
- **History** — `/undo`, `/redo`, `//history` with rollback.
- **Generation** — `//sphere`, `//cyl`, `//pyramid`, `//cone`, `//generate`, `//caves`, `//ore`, `//img`.
- **Utility** — `/butcher`, `/remove`, `/fixwater`, `/snow`, `/thaw`, `/green`, `/unstuck`, `/ascend`, `/jumpto`, `//drain`.
- **Brushes and tools** — `/br`, `/mask`, `/material`, `/size`, `/tree`, `/repl`, `/sp`.
- **Snapshots and Anvil tools** — `/snap`, `/restore`, `/anvil`.
- **Scripting** — `//macro`, `/cs`.
- **Plugin API** — other golem plugins can use `worldedit.API` and protect regions.

## Not yet

- `//setbiome`, `//generatebiome` and `//regen`: golem cannot yet change biomes or regenerate chunks in a loaded world.

## Configuration

On first start the plugin writes `config.yml` and `worldedit-config.yml` to
`plugins/worldedit/`, as FAWE does. Run `/we reload` after editing them.

## License

GPL-3.0, see [LICENSE](LICENSE). Derived from
[FastAsyncWorldEdit](https://github.com/IntellectualSites/FastAsyncWorldEdit)
and [WorldEdit](https://github.com/EngineHub/WorldEdit). Not affiliated with or
endorsed by IntellectualSites or EngineHub.
