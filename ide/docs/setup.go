package docs

import (
	"fmt"

	"github.com/jfrog/jfrog-cli-artifactory/ide/ideconsts"
	"github.com/jfrog/jfrog-cli-core/v2/plugins/components"
)

func GetDescription() string {
	return "Setup IDE integration with JFrog Artifactory."
}

func GetAIDescription() string {
	return `Configure a locally installed IDE (VS Code variants or JetBrains) to pull AI editor extensions or plugins from a JFrog Artifactory remote repo. Auto-detects the IDE installation when possible.

When to use:
- Onboarding a developer machine to a corporate Artifactory mirror of marketplace extensions.
- Pointing VS Code / Cursor / Windsurf / Kiro / JetBrains products at an internal extensions repo.

Prerequisites:
- The IDE must be installed locally; auto-detection looks at standard install paths.
- Authentication comes from one of:
    * a server saved with 'jf config add' (default server, or one selected via --server-id alone), OR
    * --url paired with --access-token, OR
    * --url paired with --user + --password.
- --repo-key identifies the Artifactory repository.
- For VS Code-based IDEs you may need write access to product.json (sudo on some installs).
- When the AI Editor Extensions gallery URL is passed as the positional argument
  (not via --repo-key), the CLI calls Artifactory's AIEditorExtensionGenerateToken
  endpoint using the resolved server credentials and appends the returned per-user
  referenceToken to the gallery URL written into product.json. This makes curated
  downloads attributable to the authenticated user. The flag flow (--repo-key
  with --url / --server-id / default server) does not tokenize the URL.

Common patterns:
  $ jf ide setup vscode --repo-key=vscode-remote
  $ jf ide setup cursor --repo-key=cursor-remote --server-id=my-prod
  $ jf ide setup vscode --url=https://acme.jfrog.io/artifactory --repo-key=vscode-remote --access-token=<token>
  $ jf ide setup jetbrains --repo-key=jetbrains-remote

Gotchas:
- Supported IDE names are case-sensitive; unknown names are rejected.
- --url on its own is not enough; pair it with --access-token or --user + --password.
- --url + --server-id is NOT a valid combination — --server-id's saved credentials are
  ignored when --url is present. To use a saved server, drop --url and pass --server-id alone.
- --update-mode only applies to VS Code-based IDEs: "default", "manual", or "none".
- product.json may require elevated privileges to edit on macOS/Linux system installs.
- If a positional service URL is passed and it already ends with /_apis/public/gallery/<token>, the CLI
  writes it verbatim (no token round-trip). If it does not, the CLI calls
  AIEditorExtensionGenerateToken to append a per-user token, using --server-id, --access-token,
  --user + --password, or the default 'jf config' server to authenticate. The URL must contain
  /api/aieditorextensions/<repo-key>/ so the CLI can identify the repo to request a token for.

Related: jf c add, jf rt repo-create`
}

func GetArguments() []components.Argument {
	return []components.Argument{
		{
			Name:        "ide-name",
			Description: fmt.Sprintf("IDE to setup. Supported: %s", ideconsts.GetSupportedIDEsString()),
		},
		{
			Name:        "url",
			Description: "[Optional] Direct repository/service URL. When provided, --repo-key and server config are not required.",
			Optional:    true,
		},
	}
}
