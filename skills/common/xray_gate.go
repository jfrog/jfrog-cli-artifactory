package common

import (
	"fmt"
	"os"
	"time"

	"github.com/jfrog/jfrog-cli-core/v2/artifactory/utils"
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	corelog "github.com/jfrog/jfrog-cli-core/v2/utils/log"
	"github.com/jfrog/jfrog-cli-core/v2/utils/progressbar"
	"github.com/jfrog/jfrog-client-go/artifactory"
	"github.com/jfrog/jfrog-client-go/artifactory/services"
	clientutils "github.com/jfrog/jfrog-client-go/utils"
	"github.com/jfrog/jfrog-client-go/utils/log"
)

const (
	defaultXrayGateTimeout = 5 * time.Minute
	xrayPollInterval       = 5 * time.Second
	envSkipSkillsScan      = "JFROG_CLI_SKIP_SKILLS_SCAN"
	envScanTimeout         = "JFROG_CLI_SKILLS_SCAN_TIMEOUT"
)

// XrayGateParams contains the parameters for the Xray scan gate check.
type XrayGateParams struct {
	ServerDetails       *config.ServerDetails
	RepoKey             string
	ArtifactPath        string // in-repo path: "slug/version/slug-version.zip"
	Slug                string
	Version             string
	SkipScan            bool
	AutoDeleteOnFailure bool
}

// CheckXrayGate calls the Artifactory Skills Xray gate endpoint after publish.
// It polls until a terminal status is reached, then acts based on the result.
// Returns an error only if the scan detects malicious content (BLOCKED).
func CheckXrayGate(params XrayGateParams) error {
	if params.SkipScan || envBool(envSkipSkillsScan) {
		log.Info("Xray scan check skipped.")
		return nil
	}

	sm, err := utils.CreateServiceManager(params.ServerDetails, 3, 0, false)
	if err != nil {
		log.Warn("Could not create service manager for Xray gate check:", err.Error())
		return nil
	}

	resp, err := sm.GetSkillXrayStatus(params.RepoKey, params.ArtifactPath)
	if err != nil {
		log.Warn("Xray gate check failed:", err.Error())
		return nil
	}

	switch resp.Status {
	case services.SkillXrayStatusNotInEntitlement:
		log.Debug("Xray entitlement not active. Skipping scan gate.")
		return nil
	case services.SkillXrayStatusDisabledForRepo:
		log.Info(fmt.Sprintf("Xray scanning is disabled for repository '%s'. Skipping scan.", params.RepoKey))
		return nil
	case services.SkillXrayStatusApproved:
		log.Info(fmt.Sprintf("[SUCCESS] Skill \"%s\" v%s passed security scan.", params.Slug, params.Version))
		return nil
	case services.SkillXrayStatusBlocked:
		return handleBlocked(sm, params)
	case services.SkillXrayStatusScanInProgress:
		return pollUntilDone(sm, params)
	default:
		log.Warn(fmt.Sprintf("Unknown Xray gate status: %s. Skipping scan gate.", resp.Status))
		return nil
	}
}

func pollUntilDone(sm artifactory.ArtifactoryServicesManager, params XrayGateParams) error {
	log.Info("Scanning for malicious content...")

	timeout := resolveTimeout()
	ticker := time.NewTicker(xrayPollInterval)
	defer ticker.Stop()
	deadline := time.After(timeout)
	pollCount := 0
	startTime := time.Now()

	// Use a spinner for interactive terminals, debug logs for CI/non-interactive.
	useSpinner := false
	mng, shouldInit, _ := progressbar.NewBarsMng()
	var spinner interface{ Abort(bool) }
	if shouldInit && mng != nil {
		useSpinner = true
		mng.GetBarsWg().Add(1)
		spinner = mng.NewUpdatableHeadlineBarWithSpinner(func() string {
			elapsed := time.Since(startTime).Truncate(time.Second)
			return fmt.Sprintf(" Scanning for malicious content... (%s elapsed, %d polls)", elapsed, pollCount)
		})
	}

	stopSpinner := func() {
		if !useSpinner {
			return
		}
		useSpinner = false
		spinner.Abort(true)
		mng.GetBarsWg().Done()
		time.Sleep(progressbar.ProgressRefreshRate)
		if logFile := mng.GetLogFile(); logFile != nil {
			_ = corelog.CloseLogFile(logFile)
		}
	}
	defer stopSpinner()

	for {
		select {
		case <-deadline:
			stopSpinner()
			log.Warn(fmt.Sprintf("Xray scan did not complete within %s after %d polls. The scan may still be in progress on the server.", timeout, pollCount))
			return nil
		case <-ticker.C:
			pollCount++
			if !useSpinner {
				log.Debug(fmt.Sprintf("Xray scan poll attempt %d...", pollCount))
			}
			resp, err := sm.GetSkillXrayStatus(params.RepoKey, params.ArtifactPath)
			if err != nil {
				log.Debug("Poll error (will retry):", err.Error())
				continue
			}
			switch resp.Status {
			case services.SkillXrayStatusApproved:
				stopSpinner()
				log.Info(fmt.Sprintf("[SUCCESS] Skill \"%s\" v%s passed security scan.", params.Slug, params.Version))
				return nil
			case services.SkillXrayStatusBlocked:
				stopSpinner()
				return handleBlocked(sm, params)
			case services.SkillXrayStatusScanInProgress:
				continue
			default:
				stopSpinner()
				log.Warn(fmt.Sprintf("Unexpected Xray gate status during polling: %s", resp.Status))
				return nil
			}
		}
	}
}

func handleBlocked(sm artifactory.ArtifactoryServicesManager, params XrayGateParams) error {
	log.Error(fmt.Sprintf("[VIOLATION] Skill \"%s\" v%s identified as malicious.", params.Slug, params.Version))
	if params.AutoDeleteOnFailure {
		deletePath := fmt.Sprintf("%s/%s/%s/", params.RepoKey, params.Slug, params.Version)
		if err := deleteSkillVersionWithManager(sm, params.RepoKey, params.Slug, params.Version); err != nil {
			log.Error(fmt.Sprintf("Failed to delete malicious skill artifact '%s' from '%s': %s", deletePath, params.RepoKey, err.Error()))
		} else {
			log.Info(fmt.Sprintf("Malicious artifact deleted: %s", deletePath))
		}
	}
	return fmt.Errorf("skill %q v%s was blocked by Xray security scan", params.Slug, params.Version)
}

// DeleteSkillVersion deletes the entire version directory for a skill.
// Creates its own service manager. For callers that already have one, use deleteSkillVersionWithManager.
func DeleteSkillVersion(serverDetails *config.ServerDetails, repoKey, slug, version string) error {
	sm, err := utils.CreateServiceManager(serverDetails, 3, 0, false)
	if err != nil {
		return fmt.Errorf("failed to create service manager for deletion: %w", err)
	}
	return deleteSkillVersionWithManager(sm, repoKey, slug, version)
}

func deleteSkillVersionWithManager(sm artifactory.ArtifactoryServicesManager, repoKey, slug, version string) error {
	if sm == nil {
		return fmt.Errorf("service manager is nil")
	}
	artURL := clientutils.AddTrailingSlashIfNeeded(sm.GetConfig().GetServiceDetails().GetUrl())
	deletePath := fmt.Sprintf("%s%s/%s/%s/", artURL, repoKey, slug, version)
	httpDetails := sm.GetConfig().GetServiceDetails().CreateHttpClientDetails()

	resp, body, err := sm.Client().SendDelete(deletePath, nil, &httpDetails)
	if err != nil {
		return fmt.Errorf("failed to delete %s/%s/%s: %w", repoKey, slug, version, err)
	}
	if resp.StatusCode >= 400 {
		return fmt.Errorf("failed to delete %s/%s/%s: HTTP %d — %s", repoKey, slug, version, resp.StatusCode, string(body))
	}
	return nil
}

func resolveTimeout() time.Duration {
	if v := os.Getenv(envScanTimeout); v != "" {
		d, err := time.ParseDuration(v)
		if err == nil && d > 0 {
			return d
		}
		log.Warn(fmt.Sprintf("Invalid %s value '%s', using default %s", envScanTimeout, v, defaultXrayGateTimeout))
	}
	return defaultXrayGateTimeout
}
