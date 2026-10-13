package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"aegion-dynamic/apismith/auth/cognito"
	"aegion-dynamic/apismith/request"

	"github.com/spf13/cobra"
)

func newRunCmd() *cobra.Command {
	var (
		envID      string
		baseURL    string
		noAuth     bool
		confirm    bool
		varKVs     []string
		clientID   string
		username   string
		password   string
		useIDToken bool
	)
	cmd := &cobra.Command{
		Use:     "run SEQUENCE.yaml",
		Short:   "Run a saved sequence of API calls",
		Long:    runHelp,
		Example: runExamples,
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			rt, err := loadRuntime()
			if err != nil {
				return err
			}
			seq, err := request.LoadSequenceFile(args[0])
			if err != nil {
				return err
			}
			if envID == "" {
				envID = rt.cfg.DefaultEnvironment
			}
			env := rt.cfg.Find(envID)
			if env == nil {
				return fmt.Errorf("unknown environment %q", envID)
			}
			vars, err := parseKV(varKVs)
			if err != nil {
				return fmt.Errorf("--var: %w", err)
			}
			target, err := request.ResolveBaseURL(env.BaseURL, baseURL)
			if err != nil {
				return err
			}

			authMode := request.AuthJWT
			if noAuth {
				authMode = request.AuthNone
			}
			var jwt string
			if sequenceNeedsJWT(seq, authMode) {
				cfg, err := rt.cfg.CognitoConfigFor(envID, clientID, username, password)
				if err != nil {
					return err
				}
				tokens, err := cognito.Generate(context.Background(), cfg)
				if err != nil {
					return fmt.Errorf("jwt: %w", err)
				}
				jwt, err = selectAuthToken(tokens, useIDToken)
				if err != nil {
					return err
				}
			}

			lib, err := request.OpenLibrary(libraryDir(rt))
			if err != nil {
				return err
			}
			result := request.RunSequence(request.NewExecutor(), lib, seq, request.RunOptions{
				BaseURL:    target,
				Production: env.Production,
				Confirm:    confirm,
				AuthMode:   authMode,
				JWT:        jwt,
				Vars:       vars,
			})
			if err := printRun(seq.Name, result); err != nil {
				return err
			}
			if !result.OK {
				return errSilent
			}
			return nil
		},
	}
	cmd.Flags().StringVarP(&envID, "env", "e", "", "environment id (default from config)")
	cmd.Flags().StringVar(&baseURL, "base-url", "", "override the environment base URL")
	cmd.Flags().BoolVar(&noAuth, "no-auth", false, "do not attach a Cognito JWT unless a step sets auth_mode: jwt")
	cmd.Flags().BoolVar(&confirm, "confirm-production", false, "allow sending to a production environment")
	cmd.Flags().StringArrayVar(&varKVs, "var", nil, "variable key=value available as {{key}}")
	cmd.Flags().StringVar(&clientID, "client-id", "", "override Cognito client id")
	cmd.Flags().StringVar(&username, "username", "", "override username")
	cmd.Flags().StringVar(&password, "password", "", "override password")
	cmd.Flags().BoolVar(&useIDToken, "id-token", false, "use Cognito id_token instead of access_token")
	return cmd
}

func libraryDir(rt *runtime) string {
	if rt != nil && rt.cfg != nil && rt.cfg.Path != "" {
		return filepath.Join(filepath.Dir(rt.cfg.Path), "library")
	}
	return filepath.Join("config", "library")
}

func sequenceNeedsJWT(seq request.Sequence, fallback request.AuthMode) bool {
	for _, step := range seq.Steps {
		mode := step.AuthMode
		if mode == "" {
			mode = fallback
		}
		if mode == request.AuthJWT {
			return true
		}
	}
	return false
}

func printRun(name string, result request.RunResult) error {
	if jsonOut {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(result)
	}
	fmt.Fprintf(os.Stderr, "%s\n", name)
	for _, step := range result.Steps {
		if step.Skipped {
			fmt.Fprintf(os.Stderr, "  -  %s  %s  skipped\n", colorMethod(step.Method), step.Path)
			continue
		}
		mark := "ok"
		if !step.OK {
			mark = "FAIL"
		}
		fmt.Fprintf(os.Stderr, "  %s  %d  %s  %s  %dms\n", mark, step.Status, colorMethod(step.Method), step.Path, step.DurationMS)
		if step.Error != "" {
			fmt.Fprintf(os.Stderr, "      %s\n", step.Error)
		}
		names := make([]string, 0, len(step.Captured))
		for k := range step.Captured {
			names = append(names, k)
		}
		sort.Strings(names)
		for _, k := range names {
			fmt.Fprintf(os.Stderr, "      %s = %s\n", k, step.Captured[k])
		}
	}
	return nil
}
