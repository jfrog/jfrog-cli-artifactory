package setup

import (
	"strings"

	"github.com/jfrog/jfrog-cli-core/v2/artifactory/utils/maven"
	"github.com/jfrog/jfrog-cli-core/v2/common/project"
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/jfrog/jfrog-client-go/utils/errorutils"
)

// mavenSettings is the part of settings.xml `jf setup maven` writes. Element names carry
// no namespace, so they match settings files with or without the Maven xmlns.
type mavenSettings struct {
	Mirrors []struct {
		ID   string `xml:"id"`
		Name string `xml:"name"`
		URL  string `xml:"url"`
	} `xml:"mirrors>mirror"`
	Servers []struct {
		ID       string `xml:"id"`
		Username string `xml:"username"`
		Password string `xml:"password"`
	} `xml:"servers>server"`
}

func inspectMaven(serverDetails *config.ServerDetails) (inspection, error) {
	result := newInspection(binaryFound("mvn"))
	path, err := userFile(".m2", "settings.xml")
	if err != nil {
		return inspection{}, err
	}
	result.status.Location = path
	content, exists, err := readOptionalFile(path)
	if err != nil || !exists {
		return result, err
	}
	var settings mavenSettings
	if err = unmarshalXML(content, &settings); err != nil {
		return inspection{}, errorutils.CheckErrorf("failed to parse %s: %s", path, err.Error())
	}
	for _, mirror := range settings.Mirrors {
		if strings.TrimSpace(mirror.ID) != maven.ArtifactoryMirrorID {
			continue
		}
		mirrorURL := strings.TrimSpace(mirror.URL)
		result.status.State, result.status.Host, result.status.RepoKey = classify(project.Maven, mirrorURL, serverDetails, firstSegment)
		if result.status.State == StateConfigured {
			// setup writes the repository key as the mirror's name.
			if name := strings.TrimSpace(mirror.Name); name != "" {
				result.status.RepoKey = name
			}
			applyMavenCredentials(&result, settings)
		}
		break
	}
	return result, nil
}

// applyMavenCredentials reads the server entry setup writes under the mirror's id.
// A password encrypted with Maven's master password ("{...}") cannot be used by the probe.
func applyMavenCredentials(result *inspection, settings mavenSettings) {
	result.status.Credentials = CredentialsAbsent
	for _, server := range settings.Servers {
		if strings.TrimSpace(server.ID) != maven.ArtifactoryMirrorID || strings.TrimSpace(server.Password) == "" {
			continue
		}
		result.status.Credentials = CredentialsPresent
		password := strings.TrimSpace(server.Password)
		if encrypted := strings.HasPrefix(password, "{") && strings.HasSuffix(password, "}"); !encrypted {
			result.credentials = storedCredentials{user: strings.TrimSpace(server.Username), password: password}
		}
		return
	}
}
