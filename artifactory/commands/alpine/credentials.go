package alpine

import (
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/jfrog/jfrog-client-go/auth"
)

// resolveCredentials resolves the effective username/password for direct Artifactory REST calls,
// preferring access token over stored password.
func resolveCredentials(serverDetails *config.ServerDetails, usernameOverride, passwordOverride string) (username, password string) {
	username = usernameOverride
	password = passwordOverride

	if username == "" && serverDetails != nil {
		username = serverDetails.GetUser()
	}

	if serverDetails != nil {
		if token := serverDetails.GetAccessToken(); token != "" {
			if username == "" {
				username = auth.ExtractUsernameFromAccessToken(token)
			}
			if password == "" {
				password = token
			}
		} else if password == "" {
			password = serverDetails.GetPassword()
		}
	}
	return
}

// resolveHTTPAuthCredentials resolves credentials for the HTTP_AUTH subprocess env var,
// preferring stored password over token to avoid short-lived token expiry mid-run.
func resolveHTTPAuthCredentials(serverDetails *config.ServerDetails, usernameOverride, passwordOverride string) (username, password string) {
	username = usernameOverride
	password = passwordOverride

	if serverDetails == nil {
		return
	}

	if username == "" {
		username = serverDetails.GetUser()
	}

	if password == "" {
		password = serverDetails.GetPassword()
	}

	if password == "" {
		if token := serverDetails.GetAccessToken(); token != "" {
			if username == "" {
				username = auth.ExtractUsernameFromAccessToken(token)
			}
			password = token
		}
	}
	return
}
