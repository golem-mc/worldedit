// Package core implements the FastAsyncWorldEdit engine, configuration,
// translations, permissions, player sessions, edit sessions, CUI protocol,
// event listeners, and public Go API service for golem.
package core

import (
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/golem-mc/golem/plugin"
	"github.com/golem-mc/golem/registry"
	"go.yaml.in/yaml/v4"
)

//go:embed data/worldedit-config.yml
var defaultWorldEditConfigYAML []byte

// Config holds both FAWE's config.yml and WorldEdit's worldedit-config.yml.
type Config struct {
	FAWE      FAWEConfig
	WorldEdit WorldEditConfig
}

// FAWEConfig mirrors FastAsyncWorldEdit's upstream config.yml (Settings.java).
type FAWEConfig struct {
	Issues              string                   `yaml:"issues"`
	Wiki                string                   `yaml:"wiki"`
	RegionRestrictions  bool                     `yaml:"region-restrictions"`
	MaxMemoryPercent    int                      `yaml:"max-memory-percent"`
	SlowerMemoryPercent int                      `yaml:"slower-memory-percent"`
	EnabledComponents   EnabledComponentsConfig  `yaml:"enabled-components"`
	Paths               PathsConfig              `yaml:"paths"`
	RegionOptions       RegionRestrictionsConfig `yaml:"region-restrictions-options"`
	Limits              map[string]*LimitConfig  `yaml:"limits"`
	History             HistoryConfig            `yaml:"history"`
	Queue               QueueConfig              `yaml:"queue"`
	Experimental        ExperimentalConfig       `yaml:"experimental"`
	Web                 WebConfig                `yaml:"web"`
	Extent              ExtentConfig             `yaml:"extent"`
	TickLimiter         TickLimiterConfig        `yaml:"tick-limiter"`
	Clipboard           ClipboardConfig          `yaml:"clipboard"`
	Lighting            LightingConfig           `yaml:"lighting"`
	General             GeneralConfig            `yaml:"general"`
}

type EnabledComponentsConfig struct {
	Commands                    bool `yaml:"commands"`
	Debug                       bool `yaml:"debug"`
	SnapshotUpdateNotifications bool `yaml:"snapshot-update-notifications"`
	ReleaseUpdateNotifications  bool `yaml:"release-update-notifications"`
	NotifyUpdateIngame          bool `yaml:"notify-update-ingame"`
}

type PathsConfig struct {
	Textures           string `yaml:"textures"`
	Heightmap          string `yaml:"heightmap"`
	History            string `yaml:"history"`
	Clipboard          string `yaml:"clipboard"`
	PerPlayerSchematic bool   `yaml:"per-player-schematics"`
}

type RegionRestrictionsConfig struct {
	Mode                      string   `yaml:"mode"`
	AllowBlacklists           bool     `yaml:"allow-blacklists"`
	ExclusiveManagers         []string `yaml:"exclusive-managers"`
	WorldGuardRegionBlacklist bool     `yaml:"worldguard-region-blacklist"`
	RestrictToSafeRange       bool     `yaml:"restrict-to-safe-range"`
}

type LimitConfig struct {
	MaxActions               int      `yaml:"max-actions"`
	MaxChanges               int64    `yaml:"max-changes"`
	MaxChecks                int64    `yaml:"max-checks"`
	MaxFails                 int      `yaml:"max-fails"`
	MaxIterations            int      `yaml:"max-iterations"`
	MaxEntities              int      `yaml:"max-entities"`
	MaxRadius                int      `yaml:"max-radius"`
	MaxSuperPickaxeSize      int      `yaml:"max-super-pickaxe-size"`
	MaxBrushRadius           int      `yaml:"max-brush-radius"`
	MaxButcherRadius         int      `yaml:"max-butcher-radius"`
	MaxBlockstates           int      `yaml:"max-blockstates"`
	MaxHistoryMB             int      `yaml:"max-history-mb"`
	SchemFileSizeLimit       int      `yaml:"schem-file-size-limit"`
	SchemFileNumLimit        int      `yaml:"schem-file-num-limit"`
	MaxExpressionMS          int      `yaml:"max-expression-ms"`
	SpeedReduction           int      `yaml:"speed-reduction"`
	FastPlacement            bool     `yaml:"fast-placement"`
	InventoryMode            int      `yaml:"inventory-mode"`
	ConfirmLarge             bool     `yaml:"confirm-large"`
	RestrictHistoryToRegions bool     `yaml:"restrict-history-to-regions"`
	StripNBT                 []string `yaml:"strip-nbt"`
	UniversalDisallowed      bool     `yaml:"universal-disallowed-blocks"`
	AllowLegacy              bool     `yaml:"allow-legacy"`
	SkipCreatureSpawnEvents  bool     `yaml:"skip-creature-spawn-events"`
	DisallowedBlocks         []string `yaml:"disallowed-blocks"`
	RemapProperties          []string `yaml:"remap-properties"`
}

type HistoryConfig struct {
	UseDisk           bool `yaml:"use-disk"`
	UseDatabase       bool `yaml:"use-database"`
	CombineStages     bool `yaml:"combine-stages"`
	SendBeforeHistory bool `yaml:"send-before-history"`
	CompressionLevel  int  `yaml:"compression-level"`
	BufferSize        int  `yaml:"buffer-size"`
	DeleteAfterDays   int  `yaml:"delete-after-days"`
	DeleteOnLogout    bool `yaml:"delete-on-logout"`
	DeleteDiskLogout  bool `yaml:"delete-disk-on-logout"`
	EnableForConsole  bool `yaml:"enable-for-console"`
	StoreRedo         bool `yaml:"store-redo"`
	SmallEdits        bool `yaml:"small-edits"`
}

type QueueConfig struct {
	ParallelThreads     int                 `yaml:"parallel-threads"`
	TargetSize          int                 `yaml:"target-size"`
	ExtraTimeMS         int                 `yaml:"extra-time-ms"`
	PreloadChunkCount   int                 `yaml:"preload-chunk-count"`
	Pool                bool                `yaml:"pool"`
	AsyncChunkLoadWrite bool                `yaml:"async-chunk-load-write"`
	ThreadTargetSizePct int                 `yaml:"thread-target-size-percent"`
	Progress            QueueProgressConfig `yaml:"progress"`
}

type QueueProgressConfig struct {
	Display  string `yaml:"display"`
	Interval int    `yaml:"interval"`
	Delay    int    `yaml:"delay"`
}

type ExperimentalConfig struct {
	UndoBatchSize         int  `yaml:"undo-batch-size"`
	AnvilQueueMode        bool `yaml:"anvil-queue-mode"`
	DynamicChunkRendering int  `yaml:"dynamic-chunk-rendering"`
	PersistentBrushes     bool `yaml:"persistent-brushes"`
	KeepEntitiesInBlocks  bool `yaml:"keep-entities-in-blocks"`
	RemoveEntityOnFail    bool `yaml:"remove-entity-from-world-on-chunk-fail"`
	ImprovedEntityEdits   bool `yaml:"improved-entity-edits"`
	Other                 bool `yaml:"other"`
	AllowTickFluids       bool `yaml:"allow-tick-fluids"`
	UseVectorAPI          bool `yaml:"use-vector-api"`
}

type WebConfig struct {
	URL                      string   `yaml:"url"`
	ArkitektonikaBackendURL  string   `yaml:"arkitektonika-backend-url"`
	ArkitektonikaDownloadURL string   `yaml:"arkitektonika-download-url"`
	ArkitektonikaDeleteURL   string   `yaml:"arkitektonika-delete-url"`
	MaxImageLoadTime         int      `yaml:"max-image-load-time"`
	MaxImageSize             int      `yaml:"max-image-size"`
	AllowedImageHosts        []string `yaml:"allowed-image-hosts"`
}

type ExtentConfig struct {
	AllowedPlugins []string `yaml:"allowed-plugins"`
	Debug          bool     `yaml:"debug"`
}

type TickLimiterConfig struct {
	Enabled   bool `yaml:"enabled"`
	Interval  int  `yaml:"interval"`
	Falling   int  `yaml:"falling"`
	PhysicsMS int  `yaml:"physics-ms"`
	Items     int  `yaml:"items"`
}

type ClipboardConfig struct {
	UseDisk           bool `yaml:"use-disk"`
	CompressionLevel  int  `yaml:"compression-level"`
	DeleteAfterDays   int  `yaml:"delete-after-days"`
	DeleteOnLogout    bool `yaml:"delete-on-logout"`
	SaveClipboardNBT  bool `yaml:"save-clipboard-nbt-to-disk"`
	LockClipboardFile bool `yaml:"lock-clipboard-file"`
}

type LightingConfig struct {
	DelayPacketSending bool `yaml:"delay-packet-sending"`
	Async              bool `yaml:"async"`
	Mode               int  `yaml:"mode"`
	RemoveFirst        bool `yaml:"remove-first"`
}

type GeneralConfig struct {
	UnstuckOnGenerate              bool `yaml:"unstuck-on-generate"`
	LimitUnlimitedConfirms         bool `yaml:"limit-unlimited-confirms"`
	LimitUnlimitedSkipsSpawnEvents bool `yaml:"limit-unlimited-skips-creature-spawn-events"`
}

// WorldEditConfig mirrors upstream worldedit-config.yml.
type WorldEditConfig struct {
	Limits struct {
		MaxBlocksChanged struct {
			Default int64 `yaml:"default"`
			Maximum int64 `yaml:"maximum"`
		} `yaml:"max-blocks-changed"`
		MaxPolygonalPoints struct {
			Default int `yaml:"default"`
			Maximum int `yaml:"maximum"`
		} `yaml:"max-polygonal-points"`
		MaxPolyhedronPoints struct {
			Default int `yaml:"default"`
			Maximum int `yaml:"maximum"`
		} `yaml:"max-polyhedron-points"`
		VerticalHeight struct {
			Default int `yaml:"default"`
		} `yaml:"vertical-height"`
		MaxRadius           int `yaml:"max-radius"`
		MaxSuperPickaxeSize int `yaml:"max-super-pickaxe-size"`
		MaxBrushRadius      int `yaml:"max-brush-radius"`
		ButcherRadius       struct {
			Default int `yaml:"default"`
			Maximum int `yaml:"maximum"`
		} `yaml:"butcher-radius"`
		DisallowedBlocks []string `yaml:"disallowed-blocks"`
	} `yaml:"limits"`
	UseInventory struct {
		Enable                bool `yaml:"enable"`
		AllowOverride         bool `yaml:"allow-override"`
		CreativeModeOverrides bool `yaml:"creative-mode-overrides"`
	} `yaml:"use-inventory"`
	Logging struct {
		LogCommands bool   `yaml:"log-commands"`
		File        string `yaml:"file"`
		Format      string `yaml:"format"`
	} `yaml:"logging"`
	SuperPickaxe struct {
		DropItems     bool `yaml:"drop-items"`
		ManyDropItems bool `yaml:"many-drop-items"`
	} `yaml:"super-pickaxe"`
	Snapshots struct {
		Directory string `yaml:"directory"`
	} `yaml:"snapshots"`
	NavigationWand struct {
		Item        string `yaml:"item"`
		MaxDistance int    `yaml:"max-distance"`
	} `yaml:"navigation-wand"`
	Scripting struct {
		Timeout int    `yaml:"timeout"`
		Dir     string `yaml:"dir"`
	} `yaml:"scripting"`
	Saving struct {
		Dir string `yaml:"dir"`
	} `yaml:"saving"`
	Files struct {
		AllowSymbolicLinks bool `yaml:"allow-symbolic-links"`
	} `yaml:"files"`
	History struct {
		Size       int `yaml:"size"`
		Expiration int `yaml:"expiration"`
	} `yaml:"history"`
	Calculation struct {
		Timeout    int `yaml:"timeout"`
		MaxTimeout int `yaml:"max-timeout"`
	} `yaml:"calculation"`
	WandItem            string `yaml:"wand-item"`
	ShellSaveType       string `yaml:"shell-save-type"`
	NoDoubleSlash       bool   `yaml:"no-double-slash"`
	NoOpPermissions     bool   `yaml:"no-op-permissions"`
	Debug               bool   `yaml:"debug"`
	ShowHelpOnFirstUse  bool   `yaml:"show-help-on-first-use"`
	ServerSideCUI       bool   `yaml:"server-side-cui"`
	CommandBlockSupport bool   `yaml:"command-block-support"`
}

// DefaultFAWEConfig builds upstream FAWE default config.yml values.
func DefaultFAWEConfig() FAWEConfig {
	cpus := max(1, runtime.NumCPU())
	return FAWEConfig{
		Issues:              "https://github.com/golem-mc/worldedit/issues",
		Wiki:                "https://intellectualsites.github.io/fastasyncworldedit-documentation/",
		RegionRestrictions:  true,
		MaxMemoryPercent:    95,
		SlowerMemoryPercent: 80,
		EnabledComponents: EnabledComponentsConfig{
			Commands:                    true,
			Debug:                       false,
			SnapshotUpdateNotifications: false,
			ReleaseUpdateNotifications:  false,
			NotifyUpdateIngame:          false,
		},
		Paths: PathsConfig{
			Textures:           "textures",
			Heightmap:          "heightmap",
			History:            "history",
			Clipboard:          "clipboard",
			PerPlayerSchematic: false,
		},
		RegionOptions: RegionRestrictionsConfig{
			Mode:                "MEMBER",
			AllowBlacklists:     false,
			ExclusiveManagers:   []string{"ExamplePlugin"},
			RestrictToSafeRange: true,
		},
		Limits: map[string]*LimitConfig{
			"default": DefaultLimitConfig(),
		},
		History: HistoryConfig{
			UseDisk:           true,
			UseDatabase:       true,
			CombineStages:     true,
			SendBeforeHistory: true,
			CompressionLevel:  3,
			BufferSize:        531441,
			DeleteAfterDays:   7,
			DeleteOnLogout:    true,
			DeleteDiskLogout:  false,
			EnableForConsole:  true,
			StoreRedo:         true,
			SmallEdits:        false,
		},
		Queue: QueueConfig{
			ParallelThreads:     cpus,
			TargetSize:          8 * cpus,
			ExtraTimeMS:         0,
			PreloadChunkCount:   512,
			Pool:                true,
			AsyncChunkLoadWrite: true,
			ThreadTargetSizePct: min(100, 200/cpus),
			Progress: QueueProgressConfig{
				Display:  "false",
				Interval: 1,
				Delay:    5000,
			},
		},
		Experimental: ExperimentalConfig{
			UndoBatchSize:         128,
			AnvilQueueMode:        false,
			DynamicChunkRendering: -1,
			PersistentBrushes:     true,
			KeepEntitiesInBlocks:  true,
			RemoveEntityOnFail:    true,
			ImprovedEntityEdits:   true,
			AllowTickFluids:       false,
		},
		Web: WebConfig{
			URL:                      "https://schem.intellectualsites.com/fawe/",
			ArkitektonikaBackendURL:  "https://api.schematic.cloud/",
			ArkitektonikaDownloadURL: "https://schematic.cloud/download/{key}",
			ArkitektonikaDeleteURL:   "https://schematic.cloud/delete/{key}",
			MaxImageLoadTime:         5,
			MaxImageSize:             8294400,
			AllowedImageHosts:        []string{"i.imgur.com"},
		},
		Extent: ExtentConfig{
			AllowedPlugins: []string{"com.example.ExamplePlugin"},
			Debug:          true,
		},
		TickLimiter: TickLimiterConfig{
			Enabled:   false,
			Interval:  20,
			Falling:   64,
			PhysicsMS: 10,
			Items:     256,
		},
		Clipboard: ClipboardConfig{
			UseDisk:           true,
			CompressionLevel:  1,
			DeleteAfterDays:   1,
			DeleteOnLogout:    false,
			SaveClipboardNBT:  true,
			LockClipboardFile: false,
		},
		Lighting: LightingConfig{
			DelayPacketSending: true,
			Async:              true,
			Mode:               1,
			RemoveFirst:        true,
		},
		General: GeneralConfig{
			UnstuckOnGenerate:              true,
			LimitUnlimitedConfirms:         true,
			LimitUnlimitedSkipsSpawnEvents: true,
		},
	}
}

// DefaultLimitConfig returns FAWE's upstream default limit group.
func DefaultLimitConfig() *LimitConfig {
	return &LimitConfig{
		MaxActions:               1,
		MaxChanges:               50000000,
		MaxChecks:                50000000,
		MaxFails:                 50000000,
		MaxIterations:            1000,
		MaxEntities:              1337,
		MaxRadius:                -1,
		MaxSuperPickaxeSize:      5,
		MaxBrushRadius:           100,
		MaxButcherRadius:         -1,
		MaxBlockstates:           1337,
		MaxHistoryMB:             -1,
		SchemFileSizeLimit:       -1,
		SchemFileNumLimit:        -1,
		MaxExpressionMS:          50,
		SpeedReduction:           0,
		FastPlacement:            true,
		InventoryMode:            0,
		ConfirmLarge:             true,
		RestrictHistoryToRegions: true,
		UniversalDisallowed:      true,
		AllowLegacy:              true,
		SkipCreatureSpawnEvents:  false,
		DisallowedBlocks:         []string{"minecraft:wheat", "minecraft:fire", "minecraft:redstone_wire"},
		RemapProperties:          []string{},
	}
}

// LoadConfig ensures config.yml and worldedit-config.yml exist in dataDir and loads both.
func LoadConfig(dataDir string) (*Config, error) {
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return nil, err
	}

	fawePath := filepath.Join(dataDir, "config.yml")
	faweCfg := DefaultFAWEConfig()
	if data, err := os.ReadFile(fawePath); err == nil {
		if err := yaml.Unmarshal(data, &faweCfg); err != nil {
			return nil, fmt.Errorf("parse config.yml: %w", err)
		}
	} else if os.IsNotExist(err) {
		out, err := yaml.Marshal(&faweCfg)
		if err != nil {
			return nil, err
		}
		header := "# FastAsyncWorldEdit configuration for golem\n"
		if err := os.WriteFile(fawePath, append([]byte(header), out...), 0o644); err != nil {
			return nil, err
		}
	} else {
		return nil, err
	}
	if faweCfg.Limits == nil {
		faweCfg.Limits = map[string]*LimitConfig{"default": DefaultLimitConfig()}
	} else if faweCfg.Limits["default"] == nil {
		faweCfg.Limits["default"] = DefaultLimitConfig()
	}

	wePath := filepath.Join(dataDir, "worldedit-config.yml")
	var weCfg WorldEditConfig
	if _, err := os.Stat(wePath); os.IsNotExist(err) {
		if err := os.WriteFile(wePath, defaultWorldEditConfigYAML, 0o644); err != nil {
			return nil, err
		}
	}
	weBytes, err := os.ReadFile(wePath)
	if err != nil {
		return nil, err
	}
	if err := yaml.Unmarshal(weBytes, &weCfg); err != nil {
		return nil, fmt.Errorf("parse worldedit-config.yml: %w", err)
	}
	if weCfg.WandItem == "" {
		weCfg.WandItem = "minecraft:wooden_axe"
	}
	if weCfg.NavigationWand.Item == "" {
		weCfg.NavigationWand.Item = "minecraft:compass"
	}
	if weCfg.NavigationWand.MaxDistance <= 0 {
		weCfg.NavigationWand.MaxDistance = 100
	}
	if weCfg.Saving.Dir == "" {
		weCfg.Saving.Dir = "schematics"
	}
	if weCfg.Scripting.Dir == "" {
		weCfg.Scripting.Dir = "craftscripts"
	}
	if weCfg.History.Size <= 0 {
		weCfg.History.Size = 15
	}
	if weCfg.Calculation.Timeout <= 0 {
		weCfg.Calculation.Timeout = 100
	}
	if weCfg.Limits.MaxPolygonalPoints.Maximum <= 0 {
		weCfg.Limits.MaxPolygonalPoints.Maximum = 20
	}
	if weCfg.Limits.MaxPolyhedronPoints.Maximum <= 0 {
		weCfg.Limits.MaxPolyhedronPoints.Maximum = 20
	}

	return &Config{FAWE: faweCfg, WorldEdit: weCfg}, nil
}

// EffectiveLimit is the resolved per-actor FAWE limit with compiled disallowed-block & property-remap rules.
type EffectiveLimit struct {
	Unlimited                bool
	MaxActions               int
	MaxChanges               int64
	MaxChecks                int64
	MaxFails                 int
	MaxIterations            int
	MaxEntities              int
	MaxRadius                int
	MaxSuperPickaxeSize      int
	MaxBrushRadius           int
	MaxButcherRadius         int
	MaxBlockstates           int
	MaxHistoryMB             int
	SchemFileSizeLimit       int
	SchemFileNumLimit        int
	MaxExpressionMS          int
	FastPlacement            bool
	InventoryMode            int
	ConfirmLarge             bool
	RestrictHistoryToRegions bool
	AllowLegacy              bool
	DisallowedBlocks         []disallowedRule
	RemapProperties          map[string]map[string]string
	StripNBT                 map[string]bool
}

type disallowedRule struct {
	blockName string
	props     map[string]string
}

// IsDisallowed reports whether st violates any disallowed-block rule in l.
func (l *EffectiveLimit) IsDisallowed(st plugin.BlockState) bool {
	if l == nil || l.Unlimited || len(l.DisallowedBlocks) == 0 {
		return false
	}
	name, props, ok := registry.StateNameProps(uint16(st))
	if !ok {
		return false
	}
	for _, r := range l.DisallowedBlocks {
		if r.blockName != name {
			continue
		}
		if len(r.props) == 0 {
			return true
		}
		allMatch := true
		for k, v := range r.props {
			if props[k] != v {
				allMatch = false
				break
			}
		}
		if allMatch {
			return true
		}
	}
	return false
}

// RemapState applies any configured property remappings to st.
func (l *EffectiveLimit) RemapState(st plugin.BlockState) plugin.BlockState {
	if l == nil || len(l.RemapProperties) == 0 {
		return st
	}
	name, props, ok := registry.StateNameProps(uint16(st))
	if !ok || len(props) == 0 {
		return st
	}
	changed := false
	next := make(map[string]string, len(props))
	for k, v := range props {
		if mp, ok := l.RemapProperties[k]; ok {
			if nv, ok := mp[v]; ok {
				next[k] = nv
				changed = true
				continue
			}
		}
		next[k] = v
	}
	if !changed {
		return st
	}
	if id, ok := registry.FindState(name, next); ok {
		return plugin.BlockState(id)
	}
	return st
}

// LimitFor computes the effective FAWE limit for sender by merging all matching
// `fawe.limit.<group>` sections (or unlimited if `fawe.limit.unlimited` is granted).
func (c *Config) LimitFor(sender plugin.CommandSender) *EffectiveLimit {
	if sender == nil || sender.HasPermission("fawe.limit.unlimited") || sender.HasPermission("fawe.bypass") {
		return &EffectiveLimit{
			Unlimited:           true,
			MaxActions:          1 << 30,
			MaxChanges:          1 << 60,
			MaxChecks:           1 << 60,
			MaxFails:            1 << 30,
			MaxIterations:       1 << 20,
			MaxEntities:         1 << 20,
			MaxRadius:           -1,
			MaxSuperPickaxeSize: 100,
			MaxBrushRadius:      1000,
			MaxButcherRadius:    -1,
			MaxBlockstates:      1 << 20,
			MaxHistoryMB:        -1,
			MaxExpressionMS:     max(50, c.WorldEdit.Calculation.MaxTimeout),
			FastPlacement:       true,
			ConfirmLarge:        c.FAWE.General.LimitUnlimitedConfirms,
			AllowLegacy:         true,
		}
	}
	eff := &EffectiveLimit{
		InventoryMode:            2,
		ConfirmLarge:             true,
		RestrictHistoryToRegions: true,
		AllowLegacy:              true,
		RemapProperties:          make(map[string]map[string]string),
		StripNBT:                 make(map[string]bool),
	}
	var matched []*LimitConfig
	for name, grp := range c.FAWE.Limits {
		if name == "default" || grp == nil {
			continue
		}
		if sender.HasPermission("fawe.limit." + name) {
			matched = append(matched, grp)
		}
	}
	if len(matched) == 0 {
		if def := c.FAWE.Limits["default"]; def != nil {
			matched = append(matched, def)
		} else {
			matched = append(matched, DefaultLimitConfig())
		}
	}
	for _, g := range matched {
		eff.MaxActions = max(eff.MaxActions, unlimitedInt(g.MaxActions))
		eff.MaxChanges = max(eff.MaxChanges, unlimitedInt64(g.MaxChanges))
		eff.MaxChecks = max(eff.MaxChecks, unlimitedInt64(g.MaxChecks))
		eff.MaxFails = max(eff.MaxFails, unlimitedInt(g.MaxFails))
		eff.MaxIterations = max(eff.MaxIterations, unlimitedInt(g.MaxIterations))
		eff.MaxEntities = max(eff.MaxEntities, unlimitedInt(g.MaxEntities))
		if g.MaxRadius == -1 {
			eff.MaxRadius = -1
		} else if eff.MaxRadius != -1 {
			eff.MaxRadius = max(eff.MaxRadius, g.MaxRadius)
		}
		eff.MaxSuperPickaxeSize = max(eff.MaxSuperPickaxeSize, unlimitedInt(g.MaxSuperPickaxeSize))
		eff.MaxBrushRadius = max(eff.MaxBrushRadius, unlimitedInt(g.MaxBrushRadius))
		if g.MaxButcherRadius == -1 {
			eff.MaxButcherRadius = -1
		} else if eff.MaxButcherRadius != -1 {
			eff.MaxButcherRadius = max(eff.MaxButcherRadius, g.MaxButcherRadius)
		}
		eff.MaxBlockstates = max(eff.MaxBlockstates, unlimitedInt(g.MaxBlockstates))
		eff.MaxHistoryMB = max(eff.MaxHistoryMB, g.MaxHistoryMB)
		eff.SchemFileSizeLimit = max(eff.SchemFileSizeLimit, g.SchemFileSizeLimit)
		eff.SchemFileNumLimit = max(eff.SchemFileNumLimit, g.SchemFileNumLimit)
		eff.MaxExpressionMS = max(eff.MaxExpressionMS, unlimitedInt(g.MaxExpressionMS))
		eff.FastPlacement = eff.FastPlacement || g.FastPlacement
		eff.InventoryMode = min(eff.InventoryMode, g.InventoryMode)
		eff.ConfirmLarge = eff.ConfirmLarge && g.ConfirmLarge
		eff.RestrictHistoryToRegions = eff.RestrictHistoryToRegions && g.RestrictHistoryToRegions
		eff.AllowLegacy = eff.AllowLegacy && g.AllowLegacy
		for _, k := range g.StripNBT {
			eff.StripNBT[k] = true
		}
		for _, raw := range g.DisallowedBlocks {
			eff.DisallowedBlocks = append(eff.DisallowedBlocks, parseDisallowedRule(raw))
		}
		for _, raw := range g.RemapProperties {
			parseRemapRule(raw, eff.RemapProperties)
		}
	}
	return eff
}

func unlimitedInt(v int) int {
	if v < 0 {
		return 1 << 30
	}
	return v
}

func unlimitedInt64(v int64) int64 {
	if v < 0 {
		return 1 << 60
	}
	return v
}

func parseDisallowedRule(raw string) disallowedRule {
	s := strings.TrimSpace(strings.ToLower(raw))
	if idx := strings.IndexByte(s, '['); idx >= 0 && strings.HasSuffix(s, "]") {
		name := registry.Qualify(s[:idx])
		props := registry.ParseStateProps(s[idx+1 : len(s)-1])
		return disallowedRule{blockName: name, props: props}
	}
	return disallowedRule{blockName: registry.Qualify(s)}
}

func parseRemapRule(raw string, out map[string]map[string]string) {
	idx := strings.IndexByte(raw, '[')
	if idx <= 0 || !strings.HasSuffix(raw, "]") {
		return
	}
	prop := strings.ToLower(strings.TrimSpace(raw[:idx]))
	body := raw[idx+1 : len(raw)-1]
	if out[prop] == nil {
		out[prop] = make(map[string]string)
	}
	for pair := range strings.SplitSeq(body, ",") {
		from, to, ok := strings.Cut(pair, ":")
		if !ok {
			continue
		}
		out[prop][strings.TrimSpace(strings.ToLower(from))] = strings.TrimSpace(strings.ToLower(to))
	}
}
