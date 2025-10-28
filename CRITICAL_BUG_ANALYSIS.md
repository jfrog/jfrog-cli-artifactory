# CRITICAL BUG: Unwanted Artifact Deployment in Maven Commands

## Executive Summary

**This bug is MORE CRITICAL than initially reported.** 

The original report focused on false failure status, but testing reveals that the CLI is **actually deploying artifacts to Artifactory when it shouldn't**, not just mis-reporting status.

---

## The Bug

### Scenario: `jf mvn install --detailed-summary`

**Expected Behavior:**
- `mvn install` should only install artifacts to local Maven repository (`~/.m2/repository`)
- **NO** deployment to Artifactory should occur
- Exit code 0 (success)

### WITHOUT Our Fix ❌

```bash
$ jf mvn install --detailed-summary
# Result:
# 1. Maven reports: BUILD SUCCESS
# 2. Artifacts are DEPLOYED to Artifactory (UNWANTED!)
# 3. JSON shows: {"status": "success", "success": 2}
# 4. Exit code: 0
```

**Proof - Artifacts were uploaded:**
```json
{
  "status": "success",
  "totals": {
    "success": 2,
    "failure": 0
  },
  "files": [
    {
      "source": "/Users/agrasthn/workspace/forked/test-maven-project/target/test-maven-project-1.0.0-SNAPSHOT.war",
      "target": "https://ecosysjfrog.jfrog.io/artifactory/maven-test-deploy/com/jfrog/test/test-maven-project/1.0.0-SNAPSHOT/test-maven-project-1.0.0-20251028.092607-8.war",
      "sha256": "4ec5ad2dd7a2133150562902b7580f28710f834b470e4772f9e724f290c6156d"
    },
    {
      "source": "/Users/agrasthn/workspace/forked/test-maven-project/pom.xml",
      "target": "https://ecosysjfrog.jfrog.io/artifactory/maven-test-deploy/com/jfrog/test/test-maven-project/1.0.0-SNAPSHOT/test-maven-project-1.0.0-20251028.092607-8.pom",
      "sha256": "e64e6ac2427f70236dc8300a4a0970863f85f2b2b8f64ee644233365045e6bcc"
    }
  ]
}
```

**This is WRONG!** `mvn install` should NOT deploy to Artifactory.

### WITH Our Fix ✅

```bash
$ jf mvn install --detailed-summary
# Result:
# 1. Maven reports: BUILD SUCCESS
# 2. NO deployment to Artifactory (CORRECT!)
# 3. Extractor log: "deploy artifacts set to false, artifacts will not be deployed..."
# 4. No JSON summary output (because deployment is disabled)
# 5. Exit code: 0
```

**This is CORRECT!** No unwanted deployment occurs.

---

## Root Cause Analysis

### The Code (Before Fix)

```go
// In mvn.go (line 114):
mc.deploymentDisabled = mc.IsXrayScan() || !vConfig.IsSet("deployer")
```

**Logic:**
- If deployer is configured: `deploymentDisabled = false`
- If no deployer: `deploymentDisabled = true`

**Problem:**
- When deployer is configured, `deploymentDisabled = false` for ALL Maven commands
- This tells the Maven Extractor to deploy artifacts regardless of the Maven goal
- Result: `mvn install`, `mvn package`, `mvn verify` all trigger deployments!

### What `deploymentDisabled` Controls

In `utils.go` (lines 206-229):

```go
func createMvnRunProps(..., disableDeploy bool) ... {
    if disableDeploy {
        setDeployFalse(vConfig)  // Sets "deployArtifacts=false" in extractor
    }
    ...
}

func setDeployFalse(vConfig *viper.Viper) {
    vConfig.Set(buildUtils.DeployerPrefix+buildUtils.DeployArtifacts, "false")
    ...
}
```

**Key Point:** When `disableDeploy = false`, the Maven Extractor (Java agent) is instructed to **actively upload artifacts** to Artifactory during the Maven build.

---

## The Fix

### Updated Code

```go
// In mvn.go (line 116):
mc.deploymentDisabled = mc.IsXrayScan() || !vConfig.IsSet("deployer") || !mc.isDeploymentRequested()

// New helper function (lines 132-141):
func (mc *MvnCommand) isDeploymentRequested() bool {
    for _, goal := range mc.goals {
        if goal == "deploy" {
            return true
        }
    }
    return false
}
```

**New Logic:**
- Deployment is disabled if ANY of these conditions is true:
  1. Xray scan mode
  2. No deployer configured
  3. **No "deploy" goal in the Maven command** (NEW!)

**Result:**
- `jf mvn install`: `deploymentDisabled = true` → No deployment ✅
- `jf mvn package`: `deploymentDisabled = true` → No deployment ✅
- `jf mvn deploy`: `deploymentDisabled = false` → Deployment occurs ✅

---

## Test Results

### Test 1: `jf mvn install --detailed-summary` (Non-Deploy Goal)

| Aspect | WITHOUT Fix ❌ | WITH Fix ✅ |
|--------|----------------|-------------|
| Maven Build | SUCCESS | SUCCESS |
| Deployment to Artifactory | **Yes (UNWANTED!)** | No (CORRECT!) |
| JSON Summary | Shown with success:2 | Not shown (deployment disabled) |
| Extractor Message | (No message) | "deploy artifacts set to false" |
| Exit Code | 0 | 0 |

### Test 2: `jf mvn clean deploy --detailed-summary` (Deploy Goal)

| Aspect | WITHOUT Fix | WITH Fix ✅ |
|--------|-------------|-------------|
| Maven Build | SUCCESS | SUCCESS |
| Deployment to Artifactory | Yes | Yes ✅ |
| JSON Summary | Shown with success:2 | Shown with success:2 ✅ |
| Exit Code | 0 | 0 ✅ |

---

## Impact Assessment

### Who is Affected?

**Any user with:**
1. Maven deployer configured in `.jfrog/projects/maven.yaml`
2. Running Maven commands WITHOUT the `deploy` goal (e.g., `install`, `package`, `verify`, `test`)
3. Using `--detailed-summary` flag (triggers the bug scenario)

### What Happens?

- **Unwanted deployments** to Artifactory on every build
- SNAPSHOT versions get deployed with every `mvn install` or `mvn package`
- Artifactory fills up with artifacts from builds that weren't meant to be deployed
- CI/CD pipelines may deploy to production repositories unintentionally

### Severity

**CRITICAL** - This causes:
1. Data pollution in Artifactory (unwanted artifact uploads)
2. Potential security issues (artifacts deployed without proper review/approval)
3. Confusion about which artifacts are "official" releases vs test builds
4. Wasted storage space

---

## Recommendations

### For the Fix
✅ **Approve and merge** - This fix is essential

### For Documentation
- Update Maven documentation to clarify when deployment occurs
- Document the relationship between Maven goals and JFrog CLI behavior

### For Future
- Consider adding a warning when deployer is configured but no deploy goal is present
- Add integration tests for non-deploy Maven goals with deployer configured

---

## Original Issue Reference

**GitHub Issue**: https://github.com/jfrog/jfrog-cli/issues/2602  
**Ticket**: RTECO-453

**Note:** The original issue description focused on "false failure" reporting, but the actual impact is unwanted deployments, which is more severe.

