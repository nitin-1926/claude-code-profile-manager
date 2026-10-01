package cmd

import (
	"bufio"
	"os"
	"strings"

	"golang.org/x/term"

	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/credentials"
	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/keystore"
	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/picker"
)

// Side-effect seams for the profile lifecycle commands (add, remove, rename,
// clone, set-default, uninstall). Tests replace them so they never touch the
// real OS keychain, launchd, or a terminal.
var (
	newKeystore         = keystore.New
	readOAuthKeychain   = credentials.ReadMacKeychainOAuth
	writeOAuthKeychain  = credentials.WriteMacKeychainOAuth
	deleteOAuthKeychain = credentials.DeleteMacKeychainOAuth
	setSystemDefault    = setSystemDefaultConfigDir
	clearSystemDefault  = clearSystemDefaultConfigDir
	selectOption        = picker.Select
	stdinIsTerminal     = func() bool { return term.IsTerminal(int(os.Stdin.Fd())) }
	checkCredentials    = func(dir, name, method string) credentials.CredStatus {
		return credentials.NewChecker(newKeystore()).Check(dir, name, method)
	}
	// readAnswer reads one trimmed line from stdin for a confirmation prompt.
	readAnswer = func() string {
		input, _ := bufio.NewReader(os.Stdin).ReadString('\n')
		return strings.TrimSpace(input)
	}
)
