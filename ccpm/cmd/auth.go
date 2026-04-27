package cmd

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
	"golang.org/x/term"

	claudepkg "github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/claude"
	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/config"
	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/credentials"
	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/keystore"
	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/vault"
)

var authCmd = &cobra.Command{
	Use:   "auth",
	Short: "Manage authentication for profiles",
}

var authStatusCmd = &cobra.Command{
	Use:   "status [name]",
	Short: "Show auth health across profiles",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runAuthStatus,
}

var authRefreshCmd = &cobra.Command{
	Use:   "refresh <name>",
	Short: "Re-authenticate a profile",
	Args:  cobra.ExactArgs(1),
	RunE:  runAuthRefresh,
}

var authBackupCmd = &cobra.Command{
	Use:   "backup <name>",
	Short: "Save encrypted credential backup",
	Args:  cobra.ExactArgs(1),
	RunE:  runAuthBackup,
}

var authRestoreCmd = &cobra.Command{
	Use:   "restore <name>",
	Short: "Restore credentials from backup",
	Args:  cobra.ExactArgs(1),
	RunE:  runAuthRestore,
}

func init() {
	authCmd.AddCommand(authStatusCmd)
	authCmd.AddCommand(authRefreshCmd)
	authCmd.AddCommand(authBackupCmd)
	authCmd.AddCommand(authRestoreCmd)
	rootCmd.AddCommand(authCmd)
}

func runAuthStatus(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	store := keystore.New()
	checker := credentials.NewChecker(store)

	green := color.New(color.FgGreen)
	red := color.New(color.FgRed)
	yellow := color.New(color.FgYellow)

	profiles := make([]config.ProfileConfig, 0)
	if len(args) == 1 {
		p, exists := cfg.Profiles[args[0]]
		if !exists {
			return fmt.Errorf("profile %q not found", args[0])
		}
		profiles = append(profiles, p)
	} else {
		names := make([]string, 0, len(cfg.Profiles))
		for n := range cfg.Profiles {
			names = append(names, n)
		}
		sort.Strings(names)
		for _, n := range names {
			profiles = append(profiles, cfg.Profiles[n])
		}
	}

	if len(profiles) == 0 {
		fmt.Println("No profiles found.")
		return nil
	}

	v := vault.New(store)
	for _, p := range profiles {
		status := checker.Check(p.Dir, p.Name, p.AuthMethod)
		c := red
		icon := "✗"
		if status.Valid {
			icon = "✓"
			c = green
			if strings.Contains(status.Detail, "expires") {
				c = yellow
				icon = "⚠"
			}
		}
		vaultStatus := "no backup"
		if v.Exists(p.Name) {
			vaultStatus = "backed up"
		}
		c.Printf("  %s %s (%s) — %s [vault: %s]\n", icon, p.Name, p.AuthMethod, status.Detail, vaultStatus)
	}

	return nil
}

func runAuthRefresh(cmd *cobra.Command, args []string) error {
	name := args[0]

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	p, exists := cfg.Profiles[name]
	if !exists {
		return fmt.Errorf("profile %q not found", name)
	}

	green := color.New(color.FgGreen, color.Bold)

	switch p.AuthMethod {
	case "oauth":
		fmt.Println("Launching Claude Code for re-authentication...")
		fmt.Println("Run /login inside to re-authenticate, then /exit to return.")
		fmt.Println()

		if err := claudepkg.Spawn(p.Dir); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: claude exited with error: %v\n", err)
		}
		green.Printf("✓ Profile %q re-authenticated\n", name)

	case "api_key":
		fmt.Print("Enter new API key: ")
		var key string
		if term.IsTerminal(int(os.Stdin.Fd())) {
			keyBytes, err := term.ReadPassword(int(os.Stdin.Fd()))
			fmt.Println()
			if err != nil {
				return fmt.Errorf("reading API key: %w", err)
			}
			key = strings.TrimSpace(string(keyBytes))
		} else {
			reader := bufio.NewReader(os.Stdin)
			line, err := reader.ReadString('\n')
			if err != nil {
				return fmt.Errorf("reading API key: %w", err)
			}
			key = strings.TrimSpace(line)
		}
		if key == "" {
			return fmt.Errorf("API key cannot be empty")
		}

		store := keystore.New()
		if err := store.SetAPIKey(name, key); err != nil {
			return fmt.Errorf("storing API key: %w", err)
		}
		green.Printf("✓ API key updated for profile %q\n", name)
	}

	return nil
}

func runAuthBackup(cmd *cobra.Command, args []string) error {
	name := args[0]

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	p, exists := cfg.Profiles[name]
	if !exists {
		return fmt.Errorf("profile %q not found", name)
	}

	store := keystore.New()
	v := vault.New(store)

	var data []byte
	switch p.AuthMethod {
	case "oauth":
		data, err = readOAuthCredentialsForBackup(p.Dir)
		if err != nil {
			return err
		}
	case "api_key":
		key, err := store.GetAPIKey(name)
		if err != nil {
			return fmt.Errorf("reading API key: %w", err)
		}
		data = []byte(key)
	}

	if err := v.Backup(name, data); err != nil {
		return fmt.Errorf("backing up credentials: %w", err)
	}

	color.New(color.FgGreen, color.Bold).Printf("✓ Credentials backed up for profile %q\n", name)
	return nil
}

func runAuthRestore(cmd *cobra.Command, args []string) error {
	name := args[0]

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	p, exists := cfg.Profiles[name]
	if !exists {
		return fmt.Errorf("profile %q not found", name)
	}

	store := keystore.New()
	v := vault.New(store)

	data, err := v.Restore(name)
	if err != nil {
		return fmt.Errorf("restoring credentials: %w", err)
	}

	switch p.AuthMethod {
	case "oauth":
		if err := writeOAuthCredentialsForRestore(p.Dir, data); err != nil {
			return err
		}
	case "api_key":
		if err := store.SetAPIKey(name, string(data)); err != nil {
			return fmt.Errorf("restoring API key to keychain: %w", err)
		}
	}

	color.New(color.FgGreen, color.Bold).Printf("✓ Credentials restored for profile %q\n", name)
	return nil
}

// readOAuthCredentialsForBackup returns the raw bytes to stuff in the vault
// for an OAuth profile. On macOS we pull from the namespaced keychain entry
// Claude Code writes; on Linux/Windows we read the legacy .credentials.json.
func readOAuthCredentialsForBackup(profileDir string) ([]byte, error) {
	if runtime.GOOS == "darwin" {
		kc, err := credentials.ReadMacKeychainOAuth(profileDir)
		if err != nil {
			return nil, fmt.Errorf("reading macOS keychain OAuth entry: %w", err)
		}
		if kc == nil || kc.Raw == "" {
			return nil, fmt.Errorf("no OAuth entry found in macOS keychain for this profile; log in with `ccpm auth refresh` first")
		}
		return []byte(kc.Raw), nil
	}
	credFile := filepath.Join(profileDir, ".credentials.json")
	data, err := os.ReadFile(credFile)
	if err != nil {
		return nil, fmt.Errorf("reading credentials file: %w", err)
	}
	return data, nil
}

// writeOAuthCredentialsForRestore is the inverse of readOAuthCredentialsForBackup.
func writeOAuthCredentialsForRestore(profileDir string, data []byte) error {
	if runtime.GOOS == "darwin" {
		if err := credentials.WriteMacKeychainOAuth(profileDir, string(data)); err != nil {
			return fmt.Errorf("writing macOS keychain OAuth entry: %w", err)
		}
		return nil
	}
	credFile := filepath.Join(profileDir, ".credentials.json")
	if err := os.WriteFile(credFile, data, 0600); err != nil {
		return fmt.Errorf("writing credentials file: %w", err)
	}
	return nil
}
