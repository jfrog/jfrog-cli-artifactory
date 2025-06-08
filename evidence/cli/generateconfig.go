package cli

import (
	"github.com/jfrog/jfrog-cli-artifactory/evidence/evidenceproviders"
	"github.com/jfrog/jfrog-cli-artifactory/evidence/evidenceproviders/sonarqube"
	"github.com/jfrog/jfrog-cli-core/v2/utils/ioutils"
	"github.com/jfrog/jfrog-client-go/utils/errorutils"
	"github.com/jfrog/jfrog-client-go/utils/io/fileutils"
	"github.com/jfrog/jfrog-client-go/utils/log"
	"gopkg.in/yaml.v3"
	neturl "net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// CreateSonarConfig creates sonar configuration based on existing config first if not available
// falls back on to default config values.
func CreateSonarConfig(sonarConfigNode *yaml.Node, evidenceConfig *evidenceproviders.EvidenceConfig) (err error) {
	var sonarConfig *evidenceproviders.SonarConfig
	if sonarConfigNode != nil {
		log.Debug("Using existing evidence.yaml file for Sonar configuration")
		if sonarConfig, err = sonarqube.CreateSonarConfiguration(sonarConfigNode); sonarConfig != nil {
			sonarConfig = sonarqube.NewSonarConfig(
				defaultIfEmpty(sonarConfig.URL, sonarqube.DefaultSonarHost),
				defaultIfEmpty(sonarConfig.ReportTaskFile, sonarqube.DefaultReportTaskFile),
				defaultIntIfEmpty(sonarConfig.MaxRetries, sonarqube.DefaultRetries),
				defaultIntIfEmpty(sonarConfig.RetryInterval, sonarqube.DefaultIntervalInSeconds),
				sonarConfig.Proxy,
			)
		} else if err != nil {
			return err
		}
	} else {
		sonarConfig = sonarqube.NewDefaultSonarConfig()
	}
	return interactiveSonarEvidenceConfiguration(sonarConfig, evidenceConfig)
}

// CreateBuildPublishConfig creates build publish configuration based on existing config first if not available
// falls back on to default config values.
func CreateBuildPublishConfig(buildPublishConfigNode *yaml.Node, evidenceConfig *evidenceproviders.EvidenceConfig) (err error) {
	buildPublishConfigData := &evidenceproviders.BuildPublishConfig{}
	if buildPublishConfigNode != nil {
		log.Debug("Using existing evidence.yaml file for build publish configuration")
		if buildPublishConfigData = buildPublishConfigData.CreateBuildPublishConfig(buildPublishConfigNode); buildPublishConfigData == nil {
			buildPublishConfigData = &evidenceproviders.BuildPublishConfig{
				Enable:   true,
				KeyPath:  "",
				KeyAlias: "",
			}
		}
	}
	return interactiveBuildPublishConfiguration(buildPublishConfigData, evidenceConfig)
}

func defaultIfEmpty(value, defaultValue string) string {
	if value == "" {
		return defaultValue
	}
	return value
}

func defaultIntIfEmpty(value *int, defaultValue int) string {
	if value == nil {
		return strconv.Itoa(defaultValue)
	}
	return strconv.Itoa(*value)
}

func interactiveSonarEvidenceConfiguration(sonarConfig *evidenceproviders.SonarConfig, evidenceConfig *evidenceproviders.EvidenceConfig) error {
	var sonarURL string
	for isURLValid := false; !isURLValid; {
		sonarURL = ioutils.AskStringWithDefault("Sonar Qube URL", "", sonarConfig.URL)
		isURLValid = validateHostOnlyURL(sonarURL)
	}
	reportTaskFile := ioutils.AskStringWithDefault("Report task file", "", sonarConfig.ReportTaskFile)
	maxRetries := ioutils.AskStringWithDefault("Max retries", "", strconv.Itoa(*sonarConfig.MaxRetries))
	retryInterval := ioutils.AskStringWithDefault("Retry interval in Seconds", "", strconv.Itoa(*sonarConfig.RetryInterval))
	var proxy string
	if sonarConfig.Proxy == "" {
		proxy = ioutils.AskString("Proxy", "", true, false)
	} else {
		proxy = ioutils.AskStringWithDefault("Proxy", "", sonarConfig.Proxy)
	}
	sc := sonarqube.NewSonarConfig(sonarURL, reportTaskFile, maxRetries, retryInterval, proxy)
	evidenceConfig.Sonar = sc
	return nil
}

func interactiveBuildPublishConfiguration(buildPublishConfig *evidenceproviders.BuildPublishConfig, evidenceConfig *evidenceproviders.EvidenceConfig) (err error) {
	if buildPublishConfig == nil {
		buildPublishConfig = &evidenceproviders.BuildPublishConfig{}
	}
	enableBuildPublish := ioutils.AskStringWithDefault("Enable Build Publish Evidence", "", "true")
	buildPublishConfig.Enable, err = strconv.ParseBool(enableBuildPublish)
	if err != nil {
		log.Warn("Invalid value for Enable Build Publish Evidence, defaulting to false")
		buildPublishConfig.Enable = false
	}
	if !buildPublishConfig.Enable {
		return nil
	}
	buildPublishConfig.EvidenceProvider = ioutils.AskStringWithDefault("Evidence Provider (Eg:- sonar)", "", buildPublishConfig.EvidenceProvider)
	buildPublishConfig.KeyAlias = ioutils.AskStringWithDefault("Key Alias", "", buildPublishConfig.KeyAlias)
	keyPath := ioutils.AskStringWithDefault("Private Key Path", "", buildPublishConfig.KeyPath)
	if exists, err := fileutils.IsFileExists(keyPath, false); err != nil || !exists {
		return errorutils.CheckErrorf("Private Key path %s does not exist or is not a valid file path", keyPath)
	}
	buildPublishConfig.KeyPath = keyPath
	evidenceConfig.BuildPublish = buildPublishConfig
	return nil
}

func validateHostOnlyURL(rawURL string) bool {
	u, err := neturl.Parse(rawURL)
	if err != nil {
		return false
	}
	switch strings.ToLower(u.Scheme) {
	case "http", "https":
	default:
		return false
	}
	if u.Hostname() == "" {
		return false
	}
	if u.User != nil || u.Path != "" || u.RawQuery != "" || u.Fragment != "" {
		return false
	}
	return true
}

func WriteConfigFile(global bool, ec *evidenceproviders.EvidenceConfig) error {
	evidenceDir, err := evidenceproviders.GetEvidenceDir(global)
	if err != nil {
		return err
	}
	if err = fileutils.CreateDirIfNotExist(evidenceDir); err != nil {
		return err
	}
	configFilePath := filepath.Join(evidenceDir, "evidence.yaml")
	resBytes, err := yaml.Marshal(ec)
	if err != nil {
		return errorutils.CheckError(err)
	}
	if err = os.WriteFile(configFilePath, resBytes, 0644); err != nil {
		return errorutils.CheckError(err)
	}
	log.Info("Evidence config successfully created.")
	return nil
}
