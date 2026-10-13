package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"aegion-dynamic/apismith/auth/cognito"
	"aegion-dynamic/apismith/openapi"
	"aegion-dynamic/apismith/request"

	"github.com/spf13/cobra"
)

func newCallCmd() *cobra.Command {
	var (
		envID      string
		body       string
		bodyFile   string
		noAuth     bool
		useIDToken bool
		expect     string
		quiet      bool
		confirm    bool
		pathKVs    []string
		queryKVs   []string
		headerKVs  []string
		clientID   string
		username   string
		password   string
		baseURL    string
	)
	cmd := &cobra.Command{
		Use:     "call [METHOD PATH | OPERATION_ID]",
		Short:   "Send one request from the OpenAPI spec",
		Long:    callHelp,
		Example: callExamples,
		Args:    cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			rt, err := loadRuntime()
			if err != nil {
				return err
			}
			ep, extracted, err := resolveCall(rt.catalog, args)
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

			pathParams, err := parseKV(pathKVs)
			if err != nil {
				return fmt.Errorf("--path: %w", err)
			}
			for k, v := range extracted {
				if _, set := pathParams[k]; !set {
					pathParams[k] = v
				}
			}
			query, err := parseKV(queryKVs)
			if err != nil {
				return fmt.Errorf("--query: %w", err)
			}
			headers, err := parseKV(headerKVs)
			if err != nil {
				return fmt.Errorf("--header: %w", err)
			}

			payload, err := readBody(body, bodyFile)
			if err != nil {
				return err
			}

			in := request.ExecuteInput{
				Environment:       envID,
				Method:            ep.Method,
				Path:              ep.Path,
				PathParams:        pathParams,
				Query:             query,
				Headers:           headers,
				Body:              payload,
				AuthMode:          request.AuthJWT,
				ConfirmProduction: confirm,
			}
			if noAuth || !ep.AuthRequired {
				in.AuthMode = request.AuthNone
			} else {
				cfg, err := rt.cfg.CognitoConfigFor(envID, clientID, username, password)
				if err != nil {
					return err
				}
				tokens, err := cognito.Generate(context.Background(), cfg)
				if err != nil {
					return fmt.Errorf("jwt: %w", err)
				}
				token, err := selectAuthToken(tokens, useIDToken)
				if err != nil {
					return err
				}
				in.JWT = token
			}

			target, err := request.ResolveBaseURL(env.BaseURL, baseURL)
			if err != nil {
				return err
			}
			out := request.NewExecutor().Execute(in, target, env.Production)
			if strings.Contains(strings.ToLower(out.ContentType), "json") {
				out.Body = request.PrettyJSON(out.Body)
			}

			if err := printCall(ep, out, quiet); err != nil {
				return err
			}
			if !statusMatches(out, expect) {
				return errSilent
			}
			return nil
		},
	}
	cmd.Flags().StringVarP(&envID, "env", "e", "", "environment id (default from config)")
	cmd.Flags().StringVar(&baseURL, "base-url", "", "override the environment base URL for this request")
	cmd.Flags().StringVar(&body, "body", "", "JSON request body")
	cmd.Flags().StringVar(&bodyFile, "body-file", "", "read request body from a file")
	cmd.Flags().BoolVar(&noAuth, "no-auth", false, "do not attach a Cognito JWT")
	cmd.Flags().BoolVar(&useIDToken, "id-token", false, "use Cognito id_token instead of access_token for Authorization")
	cmd.Flags().StringVar(&expect, "expect", "", "expected status (e.g. 200 or 2xx); default is any 2xx")
	cmd.Flags().BoolVarP(&quiet, "quiet", "q", false, "print only the status line")
	cmd.Flags().BoolVar(&confirm, "confirm-production", false, "allow sending to a production environment")
	cmd.Flags().StringArrayVar(&pathKVs, "path", nil, "path parameter key=value")
	cmd.Flags().StringArrayVar(&queryKVs, "query", nil, "query parameter key=value")
	cmd.Flags().StringArrayVar(&headerKVs, "header", nil, "extra header key=value")
	cmd.Flags().StringVar(&clientID, "client-id", "", "override Cognito client id")
	cmd.Flags().StringVar(&username, "username", "", "override username")
	cmd.Flags().StringVar(&password, "password", "", "override password")
	return cmd
}

func selectAuthToken(tokens *cognito.Tokens, useIDToken bool) (string, error) {
	if tokens == nil {
		return "", fmt.Errorf("missing cognito tokens")
	}
	if useIDToken {
		if tokens.IDToken == "" {
			return "", fmt.Errorf("Cognito returned empty id_token")
		}
		return tokens.IDToken, nil
	}
	if tokens.AccessToken == "" {
		return "", fmt.Errorf("Cognito returned empty access_token")
	}
	return tokens.AccessToken, nil
}

// errSilent is used when call should exit 1 without reprinting the error.
var errSilent = fmt.Errorf("request failed")

func resolveCall(cat *openapi.Catalog, args []string) (*openapi.Endpoint, map[string]string, error) {
	if len(args) == 2 {
		return cat.LookupMethodPath(args[0], args[1])
	}
	return cat.Lookup(args[0])
}

func readBody(body, bodyFile string) (string, error) {
	if body != "" && bodyFile != "" {
		return "", fmt.Errorf("use only one of --body and --body-file")
	}
	if bodyFile == "" {
		return body, nil
	}
	b, err := os.ReadFile(bodyFile)
	if err != nil {
		return "", fmt.Errorf("body-file: %w", err)
	}
	return string(b), nil
}

func parseKV(items []string) (map[string]string, error) {
	out := map[string]string{}
	for _, item := range items {
		k, v, ok := strings.Cut(item, "=")
		if !ok || strings.TrimSpace(k) == "" {
			return nil, fmt.Errorf("expected key=value, got %q", item)
		}
		out[strings.TrimSpace(k)] = v
	}
	return out, nil
}

func statusMatches(out request.ExecuteOutput, expect string) bool {
	return request.StatusOK(out.Status, out.Error, expect)
}

func printCall(ep *openapi.Endpoint, out request.ExecuteOutput, quiet bool) error {
	if jsonOut {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(map[string]any{
			"ok":        out.OK,
			"operation": ep.ID,
			"response":  out,
		})
	}
	status := out.StatusText
	if status == "" && out.Status != 0 {
		status = fmt.Sprintf("%d", out.Status)
	}
	if out.Error != "" && out.Status == 0 {
		status = "ERROR"
	}
	fmt.Fprintf(os.Stderr, "%s  %s  %s  %s  %dms\n", status, colorMethod(ep.Method), ep.Path, out.URL, out.DurationMS)
	if out.Error != "" {
		fmt.Fprintln(os.Stderr, out.Error)
	}
	if quiet {
		return nil
	}
	if out.Body != "" {
		fmt.Println(out.Body)
	}
	return nil
}
