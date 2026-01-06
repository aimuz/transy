// Package config handles application configuration.
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"

	"github.com/google/uuid"
	"go.aimuz.me/transy/internal/types"
)

const (
	appName        = "transy"
	oldAppName     = "fanyihub"
	configFileName = "config.json"
)

// Config represents the application configuration.
type Config struct {
	// Legacy fields (deprecated, kept for migration)
	Providers   []types.Provider `json:"providers,omitempty"`
	STTProvider string           `json:"stt_provider,omitempty"`

	// New architecture
	Credentials         []types.APICredential      `json:"credentials,omitempty"`
	TranslationProfiles []types.TranslationProfile `json:"translation_profiles,omitempty"`
	SpeechConfig        *types.SpeechConfig        `json:"speech_config,omitempty"`

	// Shared settings
	DefaultLanguages map[string]string `json:"default_languages"`
}

// Load loads configuration from the config file.
// Returns default config if file doesn't exist.
func Load() (*Config, error) {
	// Ensure migration from old app name to new app name
	if err := migrateLegacyConfig(); err != nil {
		return nil, fmt.Errorf("migrate legacy config: %w", err)
	}

	path, err := configPath()
	if err != nil {
		return nil, fmt.Errorf("get config path: %w", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return defaultConfig(), nil
		}
		return nil, fmt.Errorf("read config: %w", err)
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("unmarshal config: %w", err)
	}

	// Ensure default languages exist
	if cfg.DefaultLanguages == nil {
		cfg.DefaultLanguages = defaultLanguages()
	}

	// Migrate from legacy Provider format to new Credential + Profile format
	if err := cfg.migrateToNewFormat(); err != nil {
		return nil, fmt.Errorf("migrate to new format: %w", err)
	}

	return &cfg, nil
}

// Save persists the configuration to disk.
func (c *Config) Save() error {
	path, err := configPath()
	if err != nil {
		return fmt.Errorf("get config path: %w", err)
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}

	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("write config: %w", err)
	}

	return nil
}

// AddProvider adds a new provider.
func (c *Config) AddProvider(p types.Provider) error {
	if err := validateProvider(p); err != nil {
		return err
	}
	applyDefaults(&p)

	// First provider or explicitly active: deactivate others
	if len(c.Providers) == 0 || p.Active {
		for i := range c.Providers {
			c.Providers[i].Active = false
		}
		p.Active = true
	}

	c.Providers = append(c.Providers, p)
	return c.Save()
}

// UpdateProvider updates an existing provider.
func (c *Config) UpdateProvider(name string, p types.Provider) error {
	if err := validateProvider(p); err != nil {
		return err
	}
	applyDefaults(&p)

	idx := slices.IndexFunc(c.Providers, func(x types.Provider) bool {
		return x.Name == name
	})
	if idx == -1 {
		return fmt.Errorf("provider not found: %s", name)
	}

	wasActive := c.Providers[idx].Active
	if p.Active && !wasActive {
		for i := range c.Providers {
			c.Providers[i].Active = false
		}
	} else {
		p.Active = wasActive
	}

	c.Providers[idx] = p
	return c.Save()
}

// RemoveProvider removes a provider.
func (c *Config) RemoveProvider(name string) error {
	idx := slices.IndexFunc(c.Providers, func(p types.Provider) bool {
		return p.Name == name
	})
	if idx == -1 {
		return fmt.Errorf("provider not found: %s", name)
	}

	wasActive := c.Providers[idx].Active
	c.Providers = slices.Delete(c.Providers, idx, idx+1)

	if wasActive && len(c.Providers) > 0 {
		c.Providers[0].Active = true
	}

	return c.Save()
}

// SetProviderActive checks if provider exists and sets it active.
func (c *Config) SetProviderActive(name string) error {
	found := false
	for i := range c.Providers {
		if c.Providers[i].Name == name {
			c.Providers[i].Active = true
			found = true
		} else {
			c.Providers[i].Active = false
		}
	}
	if !found {
		return fmt.Errorf("provider not found: %s", name)
	}
	return c.Save()
}

// GetActiveProvider returns the currently active provider.
func (c *Config) GetActiveProvider() *types.Provider {
	for i := range c.Providers {
		if c.Providers[i].Active {
			p := c.Providers[i]
			return &p
		}
	}
	// Auto-activate first if none active
	if len(c.Providers) > 0 {
		c.Providers[0].Active = true
		_ = c.Save()
		p := c.Providers[0]
		return &p
	}
	return nil
}

// Helper functions

func validateProvider(p types.Provider) error {
	if p.Name == "" {
		return fmt.Errorf("provider name required")
	}
	if p.APIKey == "" {
		return fmt.Errorf("api key required")
	}
	if p.Model == "" {
		return fmt.Errorf("model required")
	}
	if p.Type == "openai-compatible" && p.BaseURL == "" {
		return fmt.Errorf("base url required for openai-compatible")
	}
	return nil
}

func applyDefaults(p *types.Provider) {
	if p.MaxTokens == 0 {
		p.MaxTokens = types.DefaultMaxTokens
	}
	if p.Temperature == 0 {
		p.Temperature = types.DefaultTemperature
	}
}

func configPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("get user config dir: %w", err)
	}
	return filepath.Join(dir, appName, configFileName), nil
}

func defaultConfig() *Config {
	return &Config{
		Providers:        []types.Provider{},
		DefaultLanguages: defaultLanguages(),
	}
}

func defaultLanguages() map[string]string {
	return map[string]string{
		"zh": "en",
		"en": "zh",
	}
}

// migrateLegacyConfig migrates configuration from old app name to new app name.
// If the old directory exists and the new one doesn't, it creates a symlink.
func migrateLegacyConfig() error {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return fmt.Errorf("get user config dir: %w", err)
	}

	oldDir := filepath.Join(configDir, oldAppName)
	newDir := filepath.Join(configDir, appName)

	// Check if old directory exists
	oldInfo, err := os.Stat(oldDir)
	if err != nil {
		if os.IsNotExist(err) {
			// No old directory, nothing to migrate
			return nil
		}
		return fmt.Errorf("stat old config dir: %w", err)
	}

	if !oldInfo.IsDir() {
		// Old path exists but is not a directory
		return nil
	}

	// Check if new directory exists
	_, err = os.Stat(newDir)
	if err == nil {
		// New directory already exists, no migration needed
		return nil
	}
	if !os.IsNotExist(err) {
		return fmt.Errorf("stat new config dir: %w", err)
	}

	// New directory doesn't exist, create symlink from new to old
	if err := os.Symlink(oldDir, newDir); err != nil {
		return fmt.Errorf("create symlink: %w", err)
	}

	return nil
}

// ─────────────────────────────────────────────────────────────────────────────
// Migration from Legacy Format
// ─────────────────────────────────────────────────────────────────────────────

// migrateToNewFormat converts legacy Provider format to new Credential + Profile format.
func (c *Config) migrateToNewFormat() error {
	if len(c.Providers) == 0 {
		return nil // Nothing to migrate
	}

	// Already migrated if we have credentials
	if len(c.Credentials) > 0 {
		return nil
	}

	// Create a map to dedupe credentials by API key
	credByKey := make(map[string]*types.APICredential)

	for _, p := range c.Providers {
		// Check if credential already exists
		cred, exists := credByKey[p.APIKey]
		if !exists {
			// Create new credential
			cred = &types.APICredential{
				ID:      uuid.New().String(),
				Name:    p.Name + " API",
				Type:    p.Type,
				BaseURL: p.BaseURL,
				APIKey:  p.APIKey,
			}
			credByKey[p.APIKey] = cred
			c.Credentials = append(c.Credentials, *cred)
		}

		// Create translation profile
		profile := types.TranslationProfile{
			ID:              uuid.New().String(),
			Name:            p.Name,
			CredentialID:    cred.ID,
			Model:           p.Model,
			SystemPrompt:    p.SystemPrompt,
			MaxTokens:       p.MaxTokens,
			Temperature:     p.Temperature,
			Active:          p.Active,
			DisableThinking: p.DisableThinking,
		}
		c.TranslationProfiles = append(c.TranslationProfiles, profile)
	}

	// Clear legacy providers after migration
	c.Providers = nil

	// Save migrated config
	return c.Save()
}

// ─────────────────────────────────────────────────────────────────────────────
// API Credential Management
// ─────────────────────────────────────────────────────────────────────────────

// GetCredentials returns all API credentials.
func (c *Config) GetCredentials() []types.APICredential {
	return c.Credentials
}

// GetCredential returns a credential by ID.
func (c *Config) GetCredential(id string) *types.APICredential {
	for i := range c.Credentials {
		if c.Credentials[i].ID == id {
			return &c.Credentials[i]
		}
	}
	return nil
}

// AddCredential adds a new API credential.
func (c *Config) AddCredential(cred types.APICredential) error {
	if cred.Name == "" {
		return fmt.Errorf("credential name required")
	}
	if cred.APIKey == "" {
		return fmt.Errorf("api key required")
	}
	if cred.Type == "openai-compatible" && cred.BaseURL == "" {
		return fmt.Errorf("base url required for openai-compatible")
	}

	if cred.ID == "" {
		cred.ID = uuid.New().String()
	}

	c.Credentials = append(c.Credentials, cred)
	return c.Save()
}

// UpdateCredential updates an existing credential.
func (c *Config) UpdateCredential(id string, cred types.APICredential) error {
	idx := slices.IndexFunc(c.Credentials, func(x types.APICredential) bool {
		return x.ID == id
	})
	if idx == -1 {
		return fmt.Errorf("credential not found: %s", id)
	}

	cred.ID = id // Preserve ID
	c.Credentials[idx] = cred
	return c.Save()
}

// RemoveCredential removes a credential by ID.
// Returns error if credential is in use by any profile or speech config.
func (c *Config) RemoveCredential(id string) error {
	// Check if in use by translation profiles
	for _, p := range c.TranslationProfiles {
		if p.CredentialID == id {
			return fmt.Errorf("credential in use by translation profile: %s", p.Name)
		}
	}
	// Check if in use by speech config
	if c.SpeechConfig != nil && c.SpeechConfig.CredentialID == id {
		return fmt.Errorf("credential in use by speech config")
	}

	idx := slices.IndexFunc(c.Credentials, func(x types.APICredential) bool {
		return x.ID == id
	})
	if idx == -1 {
		return fmt.Errorf("credential not found: %s", id)
	}

	c.Credentials = slices.Delete(c.Credentials, idx, idx+1)
	return c.Save()
}

// ─────────────────────────────────────────────────────────────────────────────
// Translation Profile Management
// ─────────────────────────────────────────────────────────────────────────────

// GetTranslationProfiles returns all translation profiles.
func (c *Config) GetTranslationProfiles() []types.TranslationProfile {
	return c.TranslationProfiles
}

// GetActiveTranslationProfile returns the currently active translation profile.
func (c *Config) GetActiveTranslationProfile() *types.TranslationProfile {
	for i := range c.TranslationProfiles {
		if c.TranslationProfiles[i].Active {
			return &c.TranslationProfiles[i]
		}
	}
	// Auto-activate first if none active
	if len(c.TranslationProfiles) > 0 {
		c.TranslationProfiles[0].Active = true
		_ = c.Save()
		return &c.TranslationProfiles[0]
	}
	return nil
}

// AddTranslationProfile adds a new translation profile.
func (c *Config) AddTranslationProfile(profile types.TranslationProfile) error {
	if profile.Name == "" {
		return fmt.Errorf("profile name required")
	}
	if profile.CredentialID == "" {
		return fmt.Errorf("credential id required")
	}
	if profile.Model == "" {
		return fmt.Errorf("model required")
	}

	// Validate credential exists
	if c.GetCredential(profile.CredentialID) == nil {
		return fmt.Errorf("credential not found: %s", profile.CredentialID)
	}

	if profile.ID == "" {
		profile.ID = uuid.New().String()
	}

	// Apply defaults
	if profile.MaxTokens == 0 {
		profile.MaxTokens = types.DefaultMaxTokens
	}
	if profile.Temperature == 0 {
		profile.Temperature = types.DefaultTemperature
	}

	// First profile or explicitly active: deactivate others
	if len(c.TranslationProfiles) == 0 || profile.Active {
		for i := range c.TranslationProfiles {
			c.TranslationProfiles[i].Active = false
		}
		profile.Active = true
	}

	c.TranslationProfiles = append(c.TranslationProfiles, profile)
	return c.Save()
}

// UpdateTranslationProfile updates an existing translation profile.
func (c *Config) UpdateTranslationProfile(id string, profile types.TranslationProfile) error {
	idx := slices.IndexFunc(c.TranslationProfiles, func(x types.TranslationProfile) bool {
		return x.ID == id
	})
	if idx == -1 {
		return fmt.Errorf("profile not found: %s", id)
	}

	// Validate credential exists
	if c.GetCredential(profile.CredentialID) == nil {
		return fmt.Errorf("credential not found: %s", profile.CredentialID)
	}

	wasActive := c.TranslationProfiles[idx].Active
	if profile.Active && !wasActive {
		for i := range c.TranslationProfiles {
			c.TranslationProfiles[i].Active = false
		}
	} else {
		profile.Active = wasActive
	}

	profile.ID = id // Preserve ID
	c.TranslationProfiles[idx] = profile
	return c.Save()
}

// RemoveTranslationProfile removes a translation profile by ID.
func (c *Config) RemoveTranslationProfile(id string) error {
	idx := slices.IndexFunc(c.TranslationProfiles, func(x types.TranslationProfile) bool {
		return x.ID == id
	})
	if idx == -1 {
		return fmt.Errorf("profile not found: %s", id)
	}

	wasActive := c.TranslationProfiles[idx].Active
	c.TranslationProfiles = slices.Delete(c.TranslationProfiles, idx, idx+1)

	if wasActive && len(c.TranslationProfiles) > 0 {
		c.TranslationProfiles[0].Active = true
	}

	return c.Save()
}

// SetTranslationProfileActive sets a translation profile as active.
func (c *Config) SetTranslationProfileActive(id string) error {
	found := false
	for i := range c.TranslationProfiles {
		if c.TranslationProfiles[i].ID == id {
			c.TranslationProfiles[i].Active = true
			found = true
		} else {
			c.TranslationProfiles[i].Active = false
		}
	}
	if !found {
		return fmt.Errorf("profile not found: %s", id)
	}
	return c.Save()
}

// ─────────────────────────────────────────────────────────────────────────────
// Speech Configuration
// ─────────────────────────────────────────────────────────────────────────────

// GetSpeechConfig returns the speech configuration.
func (c *Config) GetSpeechConfig() *types.SpeechConfig {
	return c.SpeechConfig
}

// SetSpeechConfig sets the speech configuration.
func (c *Config) SetSpeechConfig(cfg types.SpeechConfig) error {
	// Validate credential exists if enabled
	if cfg.Enabled && cfg.CredentialID != "" {
		cred := c.GetCredential(cfg.CredentialID)
		if cred == nil {
			return fmt.Errorf("credential not found: %s", cfg.CredentialID)
		}
		// Validate it's OpenAI compatible
		if cred.Type != "openai" && cred.Type != "openai-compatible" {
			return fmt.Errorf("speech config requires OpenAI-compatible credential")
		}
	}

	// Default model
	if cfg.Model == "" {
		cfg.Model = "whisper-1"
	}

	c.SpeechConfig = &cfg
	return c.Save()
}

// ─────────────────────────────────────────────────────────────────────────────
// Compatibility: Build Provider from new format for existing code
// ─────────────────────────────────────────────────────────────────────────────

// GetActiveProviderCompat returns a legacy Provider struct for backward compatibility.
// This allows existing code to continue working during the transition.
func (c *Config) GetActiveProviderCompat() *types.Provider {
	profile := c.GetActiveTranslationProfile()
	if profile == nil {
		return nil
	}

	cred := c.GetCredential(profile.CredentialID)
	if cred == nil {
		return nil
	}

	return &types.Provider{
		Name:            profile.Name,
		Type:            cred.Type,
		BaseURL:         cred.BaseURL,
		APIKey:          cred.APIKey,
		Model:           profile.Model,
		SystemPrompt:    profile.SystemPrompt,
		MaxTokens:       profile.MaxTokens,
		Temperature:     profile.Temperature,
		Active:          profile.Active,
		DisableThinking: profile.DisableThinking,
	}
}
