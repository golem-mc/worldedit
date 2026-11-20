package core

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/golem-mc/golem/plugin"
	"github.com/golem-mc/worldedit/block"
)

type fakeSender struct {
	name  string
	perms map[string]bool
}

func (f fakeSender) Name() string                { return f.name }
func (f fakeSender) SendMessage(plugin.Text)     {}
func (f fakeSender) OpLevel() int                { return 0 }
func (f fakeSender) HasPermission(n string) bool { return f.perms[n] }
func (f fakeSender) PermissionSet(n string) bool { _, ok := f.perms[n]; return ok }

func TestConfigLoadAndLimitMerging(t *testing.T) {
	dir := t.TempDir()
	cfg, err := LoadConfig(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "config.yml")); err != nil {
		t.Fatalf("config.yml not written: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "worldedit-config.yml")); err != nil {
		t.Fatalf("worldedit-config.yml not written: %v", err)
	}
	if cfg.WorldEdit.WandItem != "minecraft:wooden_axe" {
		t.Fatalf("wand item = %q", cfg.WorldEdit.WandItem)
	}

	// Add a custom VIP limit group with property remap and disallowed block rule.
	cfg.FAWE.Limits["vip"] = &LimitConfig{
		MaxChanges:       100000,
		MaxBrushRadius:   50,
		AllowLegacy:      true,
		DisallowedBlocks: []string{"minecraft:bedrock", "minecraft:oak_stairs[half=top]"},
		RemapProperties:  []string{"waterlogged[true:false]"},
	}

	vip := fakeSender{name: "VIP", perms: map[string]bool{"fawe.limit.vip": true}}
	lim := cfg.LimitFor(vip)
	if lim.Unlimited || lim.MaxChanges != 100000 || lim.MaxBrushRadius != 50 {
		t.Fatalf("unexpected VIP limit: %+v", lim)
	}
	bedrock, _ := block.ParseState("bedrock", false)
	topStairs, _ := block.ParseState("oak_stairs[facing=north,half=top]", false)
	botStairs, _ := block.ParseState("oak_stairs[facing=north,half=bottom]", false)
	if !lim.IsDisallowed(bedrock) || !lim.IsDisallowed(topStairs) || lim.IsDisallowed(botStairs) {
		t.Fatal("disallowed block rules failed")
	}

	wlStairs, _ := block.ParseState("oak_stairs[facing=north,waterlogged=true]", false)
	remapped := lim.RemapState(wlStairs)
	if !strings.Contains(block.FormatState(remapped), "waterlogged=false") {
		t.Fatalf("expected waterlogged=false after remap, got %s", block.FormatState(remapped))
	}

	admin := fakeSender{name: "Admin", perms: map[string]bool{"fawe.limit.unlimited": true}}
	if !cfg.LimitFor(admin).Unlimited {
		t.Fatal("expected unlimited limit for fawe.limit.unlimited")
	}
}

func TestMessagesAndLegacyColors(t *testing.T) {
	msgs := LoadMessages("")
	got := msgs.Raw("fawe.worldedit.copy.command.copy", 42)
	if got != "42 blocks were copied." {
		t.Fatalf("Raw = %q", got)
	}
	prefixed := msgs.Prefixed("fawe.worldedit.clipboard.clipboard.cleared")
	if !strings.Contains(prefixed.PlainString(), "(FAWE)") || !strings.Contains(prefixed.PlainString(), "Clipboard cleared") {
		t.Fatalf("Prefixed = %q", prefixed.PlainString())
	}
}
