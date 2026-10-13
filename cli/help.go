package cli

const rootHelp = `apismith exercises backend APIs from the same OpenAPI contract
that generates the server. There is no second hand-written API list.

Actions:
  ui     start the local explorer in a browser
  jwt    generate a Cognito access token
  ls     list operations from the OpenAPI spec
  call   send one request
  run    send a saved sequence of requests

Notes:
  The OpenAPI spec is the source of truth. call and ls fail if the
  operation is not in the spec.

  Default config is config/environments.yaml (--config / -c).
  OpenAPI path, listen address, default env, and target URL overlay
  from .env (CONSOLE_OPENAPI_SPEC, CONSOLE_LISTEN, CONSOLE_DEFAULT_ENV,
  CONSOLE_BASE_URL).

  Secrets do not belong in environments.yaml. Put them in
  config/credentials.yaml (gitignored) or COGNITO_* / CONSOLE_USERNAME
  / CONSOLE_PASSWORD.

  The backend verifies Cognito access tokens (token_use=access), not
  ID tokens. jwt and call attach the access token.

  Production environments (production: true) require an explicit
  confirmation in the UI, or --confirm-production on call.

  --json is machine-readable output for jwt, ls, and call.

  Run apismith help <action> for flags and examples.`

const rootExamples = `  apismith ui
  apismith ls --search login
  apismith jwt --token-only
  apismith call GET /users/me --env staging
  apismith call GET /users/me --base-url https://staging.example.com/api/v1
  apismith run config/library/sequences/signup.yaml`

const uiHelp = `Start the local API explorer.

The browser talks only to this process. The process attaches
Authorization: Bearer <access token> and forwards to DEV or STAGING,
which avoids CORS and keeps Cognito secrets off the frontend.

Notes:
  Default listen address is 127.0.0.1:8090 — loopback only, since the
  console has no auth of its own. It does not collide with the backend
  on :8080. Override with CONSOLE_LISTEN or listen in environments.yaml
  (bind 0.0.0.0 only on a trusted network).

  The UI loads the same OpenAPI spec as ls and call. New endpoints
  appear automatically when the spec changes (restart ui to reload).

  Pick an environment or edit the Base URL field to send requests to a
  hosted server instead of localhost.

  Generate JWT in the auth panel, or reuse credentials from
  config/credentials.yaml / environment variables.`

const uiExamples = `  apismith ui
  apismith ui --config config/environments.yaml`

const jwtHelp = `Generate a Cognito JWT using the same SRP (or password) flow as
jwt-token-printer.

With no file argument, credentials come from the selected environment
plus config/credentials.yaml and environment variables.

Pass a JSON or YAML file to use a jwt-token-printer config.json
directly.

Notes:
  Prints the access token (what the API verifies). Human output also
  includes the ID token and expiry. The refresh token is never printed.

  --token-only writes only the access token to stdout, for:

    Authorization: Bearer $(apismith jwt --token-only)

  --json prints access_token, id_token, token_type, expires_in.

  Do not commit tokens or credentials. Do not log Authorization headers.`

const jwtExamples = `  apismith jwt
  apismith jwt --env staging --token-only
  apismith jwt --json
  apismith jwt config/credentials.yaml
  apismith jwt path/to/config.json`

const lsHelp = `List operations parsed from the OpenAPI spec.

Each line is METHOD, path, summary, and operationId.

Notes:
  --tag filters by OpenAPI tag (case-insensitive).
  --search matches method, path, operationId, summary, or tag.

  --json prints the full catalog entries (parameters, request body
  schema, responses).

  Use the operationId or METHOD PATH with apismith call.`

const lsExamples = `  apismith ls
  apismith ls --tag users
  apismith ls --search login
  apismith ls --json`

const callHelp = `Send one request. The operation must exist in the OpenAPI spec.

Identify the operation as:
  apismith call METHOD PATH
  apismith call OPERATION_ID

PATH may be the spec template (/users/{id}) or a concrete value
(/users/abc-123). Concrete values fill path parameters unless you
override them with --path.

Notes:
  If the operation requires auth, a Cognito access token is generated
  and attached automatically. Use --no-auth for public endpoints, or
  when the spec already marks the operation as unauthenticated.

  --path, --query, and --header are repeatable key=value flags.

  --body is a JSON string; --body-file reads the payload from disk.
  Do not pass both.

  Default success is any 2xx. --expect 404 or --expect 2xx overrides
  that. Exit 0 on match, 1 otherwise. Transport errors always fail.

  Status line goes to stderr; response body goes to stdout, so you can
  pipe the body. --quiet suppresses the body. --json prints one object.

  --base-url sends this request to a hosted (or any) server instead of
  the environment's configured URL. Auth still comes from --env.

  Production targets require --confirm-production.`

const runHelp = `Run a sequence of requests from a YAML file.

Each step is an operation: method, path, params, and body. {{name}}
is filled from --var and from earlier captures. {{uuid}} and {{now}}
are filled when the step is sent. The run stops on the first step
that misses its expected status or capture path.

Notes:
  A step with no auth_mode uses the command default (a Cognito access
  token, or none with --no-auth). A step can set auth_mode: none or
  auth_mode: jwt on its own.

  expect is a status code (201) or a class (2xx). Omit it to accept
  any 2xx or 3xx.

  capture maps a variable name to a JSON path ($.id, $.items[0].id).
  The value is available to later steps as {{id}}.

  Steps that only set "saved" load that request from the library next
  to the environments file. Steps with method and path are portable
  and do not need the library.

  Tokens are not written into the sequence file. --json prints the
  step results. Exit 0 when every step passes, 1 otherwise.

  Production targets require --confirm-production.`

const runExamples = `  apismith run sequences/signup.yaml --env dev
  apismith run sequences/signup.yaml --var email=a@b.com
  apismith run sequences/public.yaml --no-auth
  apismith run sequences/signup.yaml --json`

const callExamples = `  apismith call GET /users/me --env dev
  apismith call GET /users/me --base-url https://staging.example.com/api/v1
  apismith call GET /users/{id} --path id=123 --query page=1
  apismith call GET /users/123
  apismith call POST /users --body '{"email":"test@example.com"}'
  apismith call createUser --body-file user.json
  apismith call POST /auth/login --no-auth --body-file login.json
  apismith call GET /missing --expect 404
  apismith call GET /users/me --json`
