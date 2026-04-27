package cmd

import (
	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/manifest"
	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/share"
)

func init() {
	rootCmd.AddCommand(NewAssetCmd(AssetSpec{
		Name:      "command",
		Kind:      manifest.KindCommand,
		SharedDir: share.CommandsDir,
	}))
}
