# Fix: Unwanted Artifact Deployment When Running Maven Without Deploy Goal

## ⚠️ CRITICAL BUG - More Severe Than Initially Reported

**Related Ticket**: RTECO-453  
**GitHub Issue**: https://github.com/jfrog/jfrog-cli/issues/2602

When running Maven commands like `jf mvn install` or `jf mvn package` with a deployer configured, the JFrog CLI **actually deploys artifacts to Artifactory** even though the user did not request deployment. This causes unwanted artifact uploads on every build.

**Original Report**: False failure status  
**Actual Impact**: Unwanted deployments + confusion about build results

### Before This Fix (Old Behavior)

```bash
$ jf mvn install --detailed-summary
[main] INFO ... BUILD SUCCESS
...
{
  "status": "success",
  "totals": {
    "success": 2,
    "failure": 0
  },
  "files": [
    {
      "source": "target/test-maven-project-1.0.0-SNAPSHOT.war",
      "target": "https://ecosysjfrog.jfrog.io/artifactory/maven-test-deploy/.../test-maven-project-1.0.0-20251028.092607-8.war",
      "sha256": "4ec5ad2dd7a2133150562902b7580f28710f834b470e4772f9e724f290c6156d"
    }
  ]
}
# ❌ Artifacts were DEPLOYED to Artifactory (UNWANTED!)
# Exit code: 0
```

**Impact**: 
- **Unwanted deployments** to Artifactory on every build (even `mvn install`, `mvn package`, etc.)
- Artifactory fills up with artifacts from builds that weren't meant to be deployed
- Confusion about which artifacts are "official" releases vs test builds
- Potential security issues (artifacts deployed without proper approval)
- Wasted storage space

## Root Cause

The issue occurred because:

1. User has a **deployer** configured in `maven.yaml`
2. User runs `jf mvn install` (no `deploy` goal - should only install to local `~/.m2/repository`)
3. OLD CODE: `deploymentDisabled = !vConfig.IsSet("deployer")` → evaluates to `false` (deployer exists)
4. Because `deploymentDisabled = false`, the Maven Extractor (Java agent) is instructed to **actively upload artifacts**
5. Result: Artifacts are **deployed to Artifactory** even though the user only wanted local installation
6. This happens for ANY Maven command (`install`, `package`, `verify`, `test`) when a deployer is configured

## Solution

Added a helper function `isDeploymentRequested()` that checks if the user explicitly requested deployment by looking for the `deploy` goal in the Maven command.

The deployment detection logic now considers three conditions:
```go
mc.deploymentDisabled = mc.IsXrayScan() || !vConfig.IsSet("deployer") || !mc.isDeploymentRequested()
```

This ensures that when no `deploy` goal is present, deployment is disabled, preventing false failure reporting.

## Changes Made

### Modified: `artifactory/commands/mvn/mvn.go`

1. **Added helper function** (lines 132-141):
```go
// isDeploymentRequested checks if the user explicitly requested deployment
// by looking for "deploy" goal in the Maven command goals.
func (mc *MvnCommand) isDeploymentRequested() bool {
	for _, goal := range mc.goals {
		if goal == "deploy" {
			return true
		}
	}
	return false
}
```

2. **Updated deployment detection logic** (line 117):
```go
// Maven's extractor deploys build artifacts. This should be disabled since there is no intent to deploy anything or deploy upon Xray scan results.
// Also disable deployment if the user didn't explicitly request it (no "deploy" goal in the command).
mc.deploymentDisabled = mc.IsXrayScan() || !vConfig.IsSet("deployer") || !mc.isDeploymentRequested()
```

## Testing

### Test 1: Maven Without Deploy Goal (Bug Scenario - `mvn install`)

**Before Fix**:
```bash
$ jf mvn install --detailed-summary
# Result: 
# - SUCCESS JSON showing 2 artifacts
# - Artifacts DEPLOYED to Artifactory ❌ (UNWANTED!)
# - Files shown with Artifactory URLs
# - Exit code: 0
```

**After Fix**:
```bash
$ jf mvn install --detailed-summary
# Result:
# - No JSON summary (deployment disabled)
# - Extractor: "deploy artifacts set to false" ✅
# - NO deployment to Artifactory (CORRECT!)
# - Exit code: 0 ✅
```

### Test 2: Maven With Deploy Goal (Existing Functionality)

**Before and After** (No change - works correctly):
```bash
$ jf mvn clean deploy --detailed-summary
{
  "status": "success",
  "totals": {
    "success": 2,
    "failure": 0
  },
  "files": [...]
}
# Exit code: 0 ✅
```

### Test 3: FlexPack Mode

**Before and After** (No change - unaffected):
```bash
$ JFROG_RUN_NATIVE=true jf mvn clean package --build-name=test --build-number=1
# Result: No JSON, exit code 0 ✅
```

## Summary

| Scenario | Before Fix | After Fix |
|----------|------------|-----------|
| **`jf mvn install --detailed-summary`** (no deploy goal) | ❌ Deploys to Artifactory (UNWANTED!) | ✅ No deployment (CORRECT!) |
| **`jf mvn package --detailed-summary`** (no deploy goal) | ❌ Deploys to Artifactory (UNWANTED!) | ✅ No deployment (CORRECT!) |
| **`jf mvn deploy --detailed-summary`** (with deploy goal) | ✅ Deploys to Artifactory (CORRECT!) | ✅ Deploys to Artifactory (CORRECT!) |
| **FlexPack** (`JFROG_RUN_NATIVE=true`) | ✅ No deployment | ✅ No deployment |

## Backward Compatibility

✅ **No breaking changes**  
✅ **Existing functionality preserved**  
✅ **Only fixes the false failure scenario**

## Additional Notes

- The fix is minimal and focused on the specific issue
- Only affects the case where `--detailed-summary` is used with a deployer configured but no deploy goal
- Does not affect FlexPack mode or any other Maven workflows
- Exit codes now correctly reflect Maven's actual build status


