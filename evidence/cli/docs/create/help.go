package create

import "github.com/jfrog/jfrog-cli-core/v2/plugins/components"

func GetDescription() string {
	return "Create a custom evidence and save it to a repository. Add a predicate, predicate-type, subject, key, and key-name."
}

func GetArguments() []components.Argument {
	return []components.Argument{
		{Name: "predicate", Optional: false, Description: "Path to the predicate, arbitrary JSON."},
		{Name: "predicate-type", Optional: false, Description: "Type of the predicate."},
		{Name: "subject", Optional: false, Description: "Full path to some subjects' location, an artifact."},
		{Name: "key", Optional: false, Description: "Path to a private key that will sign the DSSE. Supported keys: 'ecdsa','rsa' and 'ed25519'."},
		{Name: "key-name", Optional: true, Description: "Keyid."},
	}
}
