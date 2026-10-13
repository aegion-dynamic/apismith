package environments

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"aegion-dynamic/apismith/auth/cognito"

	"gopkg.in/yaml.v3"
)

// DefaultListen is the console's fallback listen address. Loopback
// only: the process has no auth of its own. Overridable via
// CONSOLE_LISTEN or the listen field in environments.yaml.
const DefaultListen = "127.0.0.1:8090"

// File is the on-disk environments.yaml shape.
type File struct {
	Listen             string        `yaml:"listen"`
	OpenAPISpec        string        `yaml:"openapi_spec"`
	DefaultEnvironment string        `yaml:"default_environment"`
	Environments       []Environment `yaml:"environments"`
}

// Environment describes one target API (DEV / STAGING / PRODUCTION).
type Environment struct {
	ID         string        `yaml:"id"`
	Name       string        `yaml:"name"`
	BaseURL    string        `yaml:"base_url"`
	Production bool          `yaml:"production"`
	Cognito    CognitoConfig `yaml:"cognito"`
}

// CognitoConfig is the non-secret Cognito settings for an environment.
// Username, password, and client secret live in credentials or env vars.
type CognitoConfig struct {
	Region     string `yaml:"region"`
	UserPoolID string `yaml:"user_pool_id"`
	ClientID   string `yaml:"client_id"`
	Endpoint   string `yaml:"endpoint"`
	AuthFlow   string `yaml:"auth_flow"`
}

// Credentials are secrets used to mint JWTs. Never serialised to the UI.
type Credentials struct {
	Username     string
	Password     string
	ClientSecret string
}

// Config is the runtime configuration after file load + env overlays.
type Config struct {
	Path               string // environments file, used to place history and the library beside it
	Listen             string
	OpenAPISpec        string
	DefaultEnvironment string
	Environments       []Environment
	Credentials        map[string]Credentials // keyed by environment id, plus "" for global
}

// PublicEnvironment is the subset safe to send to the browser.
type PublicEnvironment struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	BaseURL    string `json:"base_url"`
	Production bool   `json:"production"`
	Region     string `json:"region"`
	UserPoolID string `json:"user_pool_id"`
	ClientID   string `json:"client_id"`
	AuthFlow   string `json:"auth_flow"`
	HasSecret  bool   `json:"has_client_secret"`
	HasUser    bool   `json:"has_stored_credentials"`
}

// Load reads environments.yaml, optional credentials, and env-var overlays.
func Load(path string) (*Config, error) {
	if path == "" {
		path = "config/environments.yaml"
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read environments file %s: %w", path, err)
	}
	var file File
	if err := yaml.Unmarshal(raw, &file); err != nil {
		return nil, fmt.Errorf("parse environments file: %w", err)
	}

	cfg := &Config{
		Listen:             firstNonEmpty(os.Getenv("CONSOLE_LISTEN"), file.Listen, DefaultListen),
		OpenAPISpec:        firstNonEmpty(os.Getenv("CONSOLE_OPENAPI_SPEC"), file.OpenAPISpec),
		DefaultEnvironment: firstNonEmpty(os.Getenv("CONSOLE_DEFAULT_ENV"), file.DefaultEnvironment, "dev"),
		Environments:       file.Environments,
		Credentials:        map[string]Credentials{},
	}

	if credPath := firstNonEmpty(os.Getenv("CONSOLE_CREDENTIALS"), sibling(path, "credentials.yaml")); fileExists(credPath) {
		c, err := cognito.LoadFile(credPath)
		if err != nil {
			return nil, err
		}
		cfg.Credentials[""] = Credentials{
			Username:     c.Username,
			Password:     c.Password,
			ClientSecret: c.ClientSecret,
		}
		// Fill empty Cognito fields on the default env from the credentials file.
		if env := cfg.Find(cfg.DefaultEnvironment); env != nil {
			if env.Cognito.UserPoolID == "" {
				env.Cognito.UserPoolID = c.UserPoolID
			}
			if env.Cognito.ClientID == "" {
				env.Cognito.ClientID = c.ClientID
			}
			if env.Cognito.Region == "" {
				env.Cognito.Region = c.Region
			}
			if env.Cognito.Endpoint == "" {
				env.Cognito.Endpoint = c.Endpoint
			}
			if env.Cognito.AuthFlow == "" && c.AuthFlow != "" {
				env.Cognito.AuthFlow = string(c.AuthFlow)
			}
		}
	}

	cfg.applyEnvOverlays()
	if err := cfg.resolveOpenAPISpec(filepath.Dir(path)); err != nil {
		return nil, err
	}
	if len(cfg.Environments) == 0 {
		return nil, fmt.Errorf("no environments defined in %s", path)
	}
	if cfg.Find(cfg.DefaultEnvironment) == nil {
		cfg.DefaultEnvironment = cfg.Environments[0].ID
	}
	return cfg, nil
}

func (c *Config) applyEnvOverlays() {
	global := c.Credentials[""]
	if u := os.Getenv("CONSOLE_USERNAME"); u != "" {
		global.Username = u
	}
	if p := os.Getenv("CONSOLE_PASSWORD"); p != "" {
		global.Password = p
	}
	if s := os.Getenv("COGNITO_CLIENT_SECRET"); s != "" {
		global.ClientSecret = s
	}
	c.Credentials[""] = global

	env := c.Find(c.DefaultEnvironment)
	if env == nil {
		return
	}
	if v := os.Getenv("CONSOLE_BASE_URL"); v != "" {
		env.BaseURL = v
	}
	if v := os.Getenv("AWS_REGION"); v != "" {
		env.Cognito.Region = v
	}
	if v := os.Getenv("COGNITO_USER_POOL_ID"); v != "" {
		env.Cognito.UserPoolID = v
	}
	if v := os.Getenv("COGNITO_CLIENT_ID"); v != "" {
		env.Cognito.ClientID = v
	}
	if v := os.Getenv("COGNITO_ENDPOINT"); v != "" {
		env.Cognito.Endpoint = v
	}
	if v := os.Getenv("CONSOLE_AUTH_FLOW"); v != "" {
		env.Cognito.AuthFlow = v
	}
}

func (c *Config) resolveOpenAPISpec(configDir string) error {
	candidates := dedupePaths([]string{
		c.OpenAPISpec,
		filepath.Join(configDir, "..", "openapi", "openapi.yaml"),
		"openapi/openapi.yaml",
		"../framework-backend/nimbus_openapi_spec.yaml",
		"../framework-backend/docs/content/docs/openapi.yaml",
	})
	for _, p := range candidates {
		if p == "" {
			continue
		}
		if !filepath.IsAbs(p) {
			// Try as given (cwd) and relative to the config file.
			if fileExists(p) {
				abs, err := filepath.Abs(p)
				if err == nil {
					c.OpenAPISpec = abs
					return nil
				}
				c.OpenAPISpec = p
				return nil
			}
			rel := filepath.Join(configDir, p)
			if fileExists(rel) {
				abs, err := filepath.Abs(rel)
				if err == nil {
					c.OpenAPISpec = abs
					return nil
				}
				c.OpenAPISpec = rel
				return nil
			}
			continue
		}
		if fileExists(p) {
			c.OpenAPISpec = p
			return nil
		}
	}
	return fmt.Errorf("OpenAPI spec not found; set CONSOLE_OPENAPI_SPEC or copy the spec to openapi/openapi.yaml (looked for %s)", strings.Join(candidates, ", "))
}

// Find returns the environment with the given id.
func (c *Config) Find(id string) *Environment {
	if c == nil {
		return nil
	}
	for i := range c.Environments {
		if c.Environments[i].ID == id {
			return &c.Environments[i]
		}
	}
	return nil
}

// Public returns browser-safe environment descriptors.
func (c *Config) Public() []PublicEnvironment {
	out := make([]PublicEnvironment, 0, len(c.Environments))
	for i := range c.Environments {
		e := c.Environments[i]
		creds := c.credsFor(e.ID)
		out = append(out, PublicEnvironment{
			ID:         e.ID,
			Name:       e.Name,
			BaseURL:    strings.TrimRight(e.BaseURL, "/"),
			Production: e.Production,
			Region:     e.Cognito.Region,
			UserPoolID: e.Cognito.UserPoolID,
			ClientID:   e.Cognito.ClientID,
			AuthFlow:   firstNonEmpty(e.Cognito.AuthFlow, "srp"),
			HasSecret:  creds.ClientSecret != "",
			HasUser:    creds.Username != "" && creds.Password != "",
		})
	}
	return out
}

// CognitoConfigFor builds a cognito.Config for minting a JWT.
func (c *Config) CognitoConfigFor(envID, clientID, username, password string) (cognito.Config, error) {
	env := c.Find(envID)
	if env == nil {
		return cognito.Config{}, fmt.Errorf("unknown environment %q", envID)
	}
	creds := c.credsFor(envID)
	cfg := cognito.Config{
		UserPoolID:   env.Cognito.UserPoolID,
		ClientID:     firstNonEmpty(clientID, env.Cognito.ClientID),
		ClientSecret: creds.ClientSecret,
		Username:     firstNonEmpty(username, creds.Username),
		Password:     firstNonEmpty(password, creds.Password),
		Region:       env.Cognito.Region,
		Endpoint:     env.Cognito.Endpoint,
		AuthFlow:     cognito.AuthFlow(firstNonEmpty(env.Cognito.AuthFlow, "srp")),
	}
	return cfg, nil
}

func (c *Config) credsFor(envID string) Credentials {
	if specific, ok := c.Credentials[envID]; ok {
		merged := c.Credentials[""]
		if specific.Username != "" {
			merged.Username = specific.Username
		}
		if specific.Password != "" {
			merged.Password = specific.Password
		}
		if specific.ClientSecret != "" {
			merged.ClientSecret = specific.ClientSecret
		}
		return merged
	}
	return c.Credentials[""]
}

// dedupePaths drops empty entries and repeats, preserving first-seen
// order. The candidate list mixes literal and filepath.Join forms that
// can resolve to the same path; the not-found error lists what was
// searched and should not show the same entry twice.
func dedupePaths(paths []string) []string {
	out := make([]string, 0, len(paths))
	for _, p := range paths {
		if p == "" || slices.Contains(out, p) {
			continue
		}
		out = append(out, p)
	}
	return out
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func fileExists(path string) bool {
	if path == "" {
		return false
	}
	st, err := os.Stat(path)
	return err == nil && !st.IsDir()
}

func sibling(path, name string) string {
	return filepath.Join(filepath.Dir(path), name)
}
