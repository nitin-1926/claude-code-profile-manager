package cmd

import (
	"github.com/nitin-1926/ccpm/internal/manifest"
	"github.com/nitin-1926/ccpm/internal/share"
)

func init() {
	rootCmd.AddCommand(NewAssetCmd(AssetSpec{
		Name:      "rule",
		Kind:      manifest.KindRule,
		SharedDir: share.RulesDir,
	}))
}
