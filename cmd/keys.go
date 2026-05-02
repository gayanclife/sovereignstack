// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package cmd

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/gayanclife/sovereignstack/core/keys"
	"github.com/spf13/cobra"
)

var keysCmd = &cobra.Command{
	Use:   "keys",
	Short: "Manage API keys and user profiles",
	Long: `Manage API keys and user profiles for the gateway.

Keys are stored in ~/.sovereignstack/keys.json and used by the gateway
to authenticate requests and enforce access policies.`,
}

var keysAddCmd = &cobra.Command{
	Use:   "add <user-id>",
	Short: "Add a new user with a generated API key",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		userID := args[0]
		keyPath := getKeyStorePath()

		ks, err := keys.LoadKeyStore(keyPath)
		if err != nil {
			return fmt.Errorf("failed to load keys: %w", err)
		}

		existing, _ := ks.GetByID(userID)
		if existing != nil {
			return fmt.Errorf("user %q already exists", userID)
		}

		dept, _ := cmd.Flags().GetString("department")
		team, _ := cmd.Flags().GetString("team")
		role, _ := cmd.Flags().GetString("role")
		rateLimit, _ := cmd.Flags().GetFloat64("rate-limit")

		apiKey := generateAPIKey(userID)

		profile := &keys.UserProfile{
			ID:              userID,
			Key:             apiKey,
			Department:      dept,
			Team:            team,
			Role:            role,
			RateLimitPerMin: rateLimit,
			AllowedModels:   []string{},
			CreatedAt:       time.Now(),
			LastUsedAt:      time.Now(),
		}

		if err := ks.AddUser(profile); err != nil {
			return fmt.Errorf("failed to add user: %w", err)
		}

		return emit(cmd, profile, func() {
			fmt.Printf("✓ Created user %q\n", userID)
			fmt.Printf("  API Key: %s\n", apiKey)
			fmt.Printf("  Rate Limit: %.0f requests/min\n", rateLimit)
		})
	},
}

var keysListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all users (without showing API keys)",
	RunE: func(cmd *cobra.Command, args []string) error {
		keyPath := getKeyStorePath()

		ks, err := keys.LoadKeyStore(keyPath)
		if err != nil {
			return fmt.Errorf("failed to load keys: %w", err)
		}

		users := ks.ListUsers()
		return emit(cmd, map[string]any{"users": users, "count": len(users)}, func() {
			if len(users) == 0 {
				fmt.Println("No users found.")
				return
			}

			w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			fmt.Fprintln(w, "USER ID\tDEPARTMENT\tROLE\tMODELS\tDAILY QUOTA\tMONTHLY QUOTA\tLAST USED")

			for _, user := range users {
				dailyQuota := fmt.Sprintf("%d", user.MaxTokensPerDay)
				monthlyQuota := fmt.Sprintf("%d", user.MaxTokensPerMonth)
				if user.MaxTokensPerDay == 0 {
					dailyQuota = "unlimited"
				}
				if user.MaxTokensPerMonth == 0 {
					monthlyQuota = "unlimited"
				}

				lastUsed := "never"
				if !user.LastUsedAt.IsZero() {
					lastUsed = time.Since(user.LastUsedAt).String() + " ago"
				}

				fmt.Fprintf(w, "%s\t%s\t%s\t%d\t%s\t%s\t%s\n",
					user.ID, user.Department, user.Role, len(user.AllowedModels),
					dailyQuota, monthlyQuota, lastUsed)
			}
			w.Flush()
		})
	},
}

var keysRemoveCmd = &cobra.Command{
	Use:   "remove <user-id>",
	Short: "Remove a user and revoke their API key",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		userID := args[0]
		keyPath := getKeyStorePath()

		ks, err := keys.LoadKeyStore(keyPath)
		if err != nil {
			return fmt.Errorf("failed to load keys: %w", err)
		}

		if err := ks.RemoveUser(userID); err != nil {
			return fmt.Errorf("failed to remove user: %w", err)
		}

		return emit(cmd, map[string]any{"user_id": userID, "removed": true}, func() {
			fmt.Printf("✓ Removed user %q\n", userID)
		})
	},
}

var keysInfoCmd = &cobra.Command{
	Use:   "info <user-id>",
	Short: "Show detailed user profile",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		userID := args[0]
		keyPath := getKeyStorePath()

		ks, err := keys.LoadKeyStore(keyPath)
		if err != nil {
			return fmt.Errorf("failed to load keys: %w", err)
		}

		user, _ := ks.GetByID(userID)
		if user == nil {
			return fmt.Errorf("user %q not found", userID)
		}

		return emit(cmd, user, func() {
			fmt.Printf("User: %s\n", user.ID)
			fmt.Printf("  API Key: %s\n", user.Key)
			fmt.Printf("  Department: %s\n", user.Department)
			fmt.Printf("  Team: %s\n", user.Team)
			fmt.Printf("  Role: %s\n", user.Role)
			fmt.Printf("  Rate Limit: %.0f requests/min\n", user.RateLimitPerMin)
			fmt.Printf("  Daily Token Limit: ")
			if user.MaxTokensPerDay == 0 {
				fmt.Printf("unlimited\n")
			} else {
				fmt.Printf("%d tokens\n", user.MaxTokensPerDay)
			}
			fmt.Printf("  Monthly Token Limit: ")
			if user.MaxTokensPerMonth == 0 {
				fmt.Printf("unlimited\n")
			} else {
				fmt.Printf("%d tokens\n", user.MaxTokensPerMonth)
			}
			fmt.Printf("  Allowed Models: ")
			if len(user.AllowedModels) == 0 {
				fmt.Printf("(none)\n")
			} else {
				fmt.Printf("%s\n", strings.Join(user.AllowedModels, ", "))
			}
			fmt.Printf("  Created: %s\n", user.CreatedAt.Format("2006-01-02 15:04:05"))
			fmt.Printf("  Last Used: %s\n", user.LastUsedAt.Format("2006-01-02 15:04:05"))
		})
	},
}

var keysGrantModelCmd = &cobra.Command{
	Use:   "grant-model <user-id> <model>",
	Short: "Allow a user to access a model",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		userID := args[0]
		model := args[1]
		keyPath := getKeyStorePath()

		ks, err := keys.LoadKeyStore(keyPath)
		if err != nil {
			return fmt.Errorf("failed to load keys: %w", err)
		}

		user, _ := ks.GetByID(userID)
		if user == nil {
			return fmt.Errorf("user %q not found", userID)
		}

		for _, m := range user.AllowedModels {
			if m == model {
				fmt.Printf("✓ User %q already has access to %q\n", userID, model)
				return nil
			}
		}

		user.AllowedModels = append(user.AllowedModels, model)
		if err := ks.AddUser(user); err != nil {
			return fmt.Errorf("failed to update user: %w", err)
		}

		fmt.Printf("✓ Granted %q access to %q\n", userID, model)
		return nil
	},
}

var keysRevokeModelCmd = &cobra.Command{
	Use:   "revoke-model <user-id> <model>",
	Short: "Deny a user access to a model",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		userID := args[0]
		model := args[1]
		keyPath := getKeyStorePath()

		ks, err := keys.LoadKeyStore(keyPath)
		if err != nil {
			return fmt.Errorf("failed to load keys: %w", err)
		}

		user, _ := ks.GetByID(userID)
		if user == nil {
			return fmt.Errorf("user %q not found", userID)
		}

		newModels := make([]string, 0)
		found := false
		for _, m := range user.AllowedModels {
			if m == model {
				found = true
			} else {
				newModels = append(newModels, m)
			}
		}

		if !found {
			fmt.Printf("✓ User %q doesn't have access to %q\n", userID, model)
			return nil
		}

		user.AllowedModels = newModels
		if err := ks.AddUser(user); err != nil {
			return fmt.Errorf("failed to update user: %w", err)
		}

		fmt.Printf("✓ Revoked %q access to %q\n", userID, model)
		return nil
	},
}

var keysMigrateHashCmd = &cobra.Command{
	Use:   "migrate-hash",
	Short: "Convert plaintext API keys in keys.json to argon2id hashes (Phase C)",
	Long: `One-time migration that converts every plaintext API key stored in
keys.json into an argon2id hash. Idempotent — safe to re-run; rows that are
already hashed are skipped.

Existing keys continue to work for authentication after migration. The
plaintext is no longer recoverable; if a user has lost their key, issue a
new one with 'sovstack keys add'.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		keyPath := getKeyStorePath()
		ks, err := keys.LoadKeyStore(keyPath)
		if err != nil {
			return fmt.Errorf("failed to load keys: %w", err)
		}

		migrated, err := ks.MigrateHashes()
		if err != nil {
			return fmt.Errorf("migrate: %w", err)
		}

		return emit(cmd, map[string]any{
			"migrated":     migrated,
			"keys_file":    keyPath,
			"already_safe": migrated == 0,
		}, func() {
			if migrated == 0 {
				fmt.Println("✓ All keys already hashed — no migration needed.")
			} else {
				fmt.Printf("✓ Hashed %d plaintext key(s) in %s\n", migrated, keyPath)
			}
		})
	},
}

var keysSetQuotaCmd = &cobra.Command{
	Use:   "set-quota <user-id>",
	Short: "Set token quotas for a user",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		userID := args[0]
		keyPath := getKeyStorePath()

		daily, _ := cmd.Flags().GetInt64("daily")
		monthly, _ := cmd.Flags().GetInt64("monthly")

		ks, err := keys.LoadKeyStore(keyPath)
		if err != nil {
			return fmt.Errorf("failed to load keys: %w", err)
		}

		user, _ := ks.GetByID(userID)
		if user == nil {
			return fmt.Errorf("user %q not found", userID)
		}

		user.MaxTokensPerDay = daily
		user.MaxTokensPerMonth = monthly

		if err := ks.AddUser(user); err != nil {
			return fmt.Errorf("failed to update user: %w", err)
		}

		fmt.Printf("✓ Updated quotas for %q\n", userID)
		fmt.Printf("  Daily: ")
		if daily == 0 {
			fmt.Printf("unlimited\n")
		} else {
			fmt.Printf("%d tokens\n", daily)
		}
		fmt.Printf("  Monthly: ")
		if monthly == 0 {
			fmt.Printf("unlimited\n")
		} else {
			fmt.Printf("%d tokens\n", monthly)
		}
		return nil
	},
}

var keysUsageCmd = &cobra.Command{
	Use:   "usage <user-id>",
	Short: "Show token usage for a user",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		userID := args[0]
		keyPath := getKeyStorePath()

		ks, err := keys.LoadKeyStore(keyPath)
		if err != nil {
			return fmt.Errorf("failed to load keys: %w", err)
		}

		user, _ := ks.GetByID(userID)
		if user == nil {
			return fmt.Errorf("user %q not found", userID)
		}

		fmt.Printf("User: %s\n", userID)
		fmt.Printf("  Daily Limit: ")
		if user.MaxTokensPerDay == 0 {
			fmt.Printf("unlimited\n")
		} else {
			fmt.Printf("%d tokens\n", user.MaxTokensPerDay)
		}
		fmt.Printf("  Monthly Limit: ")
		if user.MaxTokensPerMonth == 0 {
			fmt.Printf("unlimited\n")
		} else {
			fmt.Printf("%d tokens\n", user.MaxTokensPerMonth)
		}
		fmt.Println()
		fmt.Println("Note: Usage tracking requires gateway with metrics endpoint running.")
		fmt.Println("      Run: sovstack gateway --keys ~/.sovereignstack/keys.json")
		return nil
	},
}

func init() {
	rootCmd.AddCommand(keysCmd)

	// All `sovstack keys ...` subcommands inherit this output-format flag.
	keysCmd.PersistentFlags().StringP("output", "o", "text", "Output format: text | json")

	keysCmd.AddCommand(keysAddCmd)
	keysAddCmd.Flags().StringP("department", "d", "", "Department name")
	keysAddCmd.Flags().StringP("team", "t", "", "Team name")
	keysAddCmd.Flags().StringP("role", "r", "user", "User role")
	keysAddCmd.Flags().Float64P("rate-limit", "l", 100, "Rate limit (requests per minute)")

	keysCmd.AddCommand(keysListCmd)
	keysCmd.AddCommand(keysRemoveCmd)
	keysCmd.AddCommand(keysInfoCmd)
	keysCmd.AddCommand(keysGrantModelCmd)
	keysCmd.AddCommand(keysRevokeModelCmd)

	keysCmd.AddCommand(keysMigrateHashCmd)
	keysCmd.AddCommand(keysSetQuotaCmd)
	keysSetQuotaCmd.Flags().Int64P("daily", "d", 0, "Daily token limit (0 = unlimited)")
	keysSetQuotaCmd.Flags().Int64P("monthly", "m", 0, "Monthly token limit (0 = unlimited)")

	keysCmd.AddCommand(keysUsageCmd)
}

func generateAPIKey(userID string) string {
	data := fmt.Sprintf("%s_%d", userID, time.Now().UnixNano())
	hash := sha256.Sum256([]byte(data))
	key := fmt.Sprintf("sk_%x", hash)
	if len(key) > 40 {
		key = key[:40]
	}
	return key
}

func getKeyStorePath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		home = "."
	}
	return filepath.Join(home, ".sovereignstack", "keys.json")
}
